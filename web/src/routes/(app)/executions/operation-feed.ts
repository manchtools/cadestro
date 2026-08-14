// Movement F's "one card per operation" — DERIVED from execution rows, never read.
//
// `message ActionExecution` (contract/proto/cadestro/v1/control.proto) carries no
// shared dispatch, batch, manifest or operation id. Its complete field list is
// id, device_id, action_id, type, status, error, output, created_at,
// dispatched_at, completed_at, duration_ms, created_by, live_output,
// desired_state, changed, compliant, detection_output, action_name and
// scheduled_for; DispatchToMultiple answers with a flat
// `repeated ActionExecution`. A fan-out is therefore only recoverable by
// clustering the rows, so this module clusters — it never invents an id, and
// the cluster key below is a Svelte keyed-each key, never rendered as if the
// server had issued it.
//
// THE RULE, in full:
//
//   identity  action_id when the execution references a ManagedAction, else
//             `inline:<action_type>` — an inline action has no action row, so
//             the type is the only identity that survives the search document.
//
//   cluster   rows of one identity, ordered by (created_at, id), split wherever
//             the gap to the previous row exceeds OPERATION_WINDOW_SECONDS. One
//             Dispatch* call writes its executions inside a single server
//             transaction, so an operation's rows share a created_at second;
//             two separate operator gestures against the same action are
//             practically never that close together.
//
//   actor     is NOT part of the key. The executions search document
//             (server/internal/store/search_documents.go) indexes device_id,
//             action_id, device_hostname, action_name, status, action_type,
//             desired_state, changed, compliant, created_at, scheduled_for and
//             completed_at — `created_by` is not among them, so a list row has
//             no actor to group by. Rows that arrive from a typed read (the
//             DispatchToMultiple response a retry patches in) do carry it, and
//             the card names the actor only then.
//
// The clustering is order-independent: identical input rows in any order yield
// identical operations, so a re-sort of the underlying list never reshuffles
// the feed's grouping.

import type { Timestamp } from '@bufbuild/protobuf/wkt';
import { ExecutionStatus } from '$contract/cadestro/v1/common_pb';

/** Seconds of slack that still read as one dispatch gesture. */
export const OPERATION_WINDOW_SECONDS = 5;

/** DispatchToMultipleRequest.device_ids is validated `max=256`. */
export const RETRY_MAX_DEVICES = 256;

export type EffectBucket = 'failed' | 'queued' | 'ok' | 'skipped' | 'cancelled' | 'unknown';

/** Failed first — partial success is a shape to read, not an error to dismiss. */
export const BUCKET_ORDER: readonly EffectBucket[] = [
	'failed',
	'queued',
	'ok',
	'skipped',
	'cancelled',
	'unknown'
];

/**
 * Every ExecutionStatus lands in exactly one bucket, so the summary counts
 * always sum to the number of effect rows.
 *
 * INDETERMINATE joins `failed`: the run's outcome is unknown, and re-dispatch
 * is the remedy — the same remedy FAILED and TIMEOUT get.
 */
export function statusBucket(status: number): EffectBucket {
	switch (status) {
		case ExecutionStatus.FAILED:
		case ExecutionStatus.TIMEOUT:
		case ExecutionStatus.INDETERMINATE:
			return 'failed';
		case ExecutionStatus.PENDING:
		case ExecutionStatus.RUNNING:
		case ExecutionStatus.SCHEDULED:
			return 'queued';
		case ExecutionStatus.SUCCESS:
			return 'ok';
		case ExecutionStatus.SKIPPED:
		case ExecutionStatus.NOT_APPLICABLE:
			return 'skipped';
		case ExecutionStatus.CANCELLED:
			return 'cancelled';
		default:
			return 'unknown';
	}
}

/** The row shape the feed needs — structurally satisfied by ActionExecution
 *  plus the list page's indexed `deviceHostname`. */
export interface FeedRow {
	id: string;
	actionId: string;
	actionName: string;
	type: number;
	deviceId: string;
	deviceHostname: string;
	status: number;
	createdBy?: string;
	createdAt?: Timestamp;
	// Rendered by the effect row when the row carries them; a row rebuilt from a
	// SearchResult leaves them at the proto default.
	desiredState?: number;
	changed?: boolean;
	durationMs?: bigint;
}

export interface Operation<R extends FeedRow = FeedRow> {
	/** Keyed-each identity only — derived, never shown as a server id. */
	key: string;
	/** Empty for an inline action: there is no ManagedAction to re-dispatch. */
	actionId: string;
	actionName: string;
	type: number;
	/** Effect rows, failed first. */
	effects: R[];
	counts: Record<EffectBucket, number>;
	/** Distinct devices the operation touched, in effect order. */
	deviceIds: string[];
	startedAtSeconds: number;
	/** The anchor row's created_at, so the card formats time the same way every
	 *  other surface does instead of re-deriving it from the seconds. */
	startedAt?: Timestamp;
	/** Non-empty only when every row that names an actor names the same one. */
	actor: string;
	/** Exactly the devices whose effect failed — the retry subset. */
	retryDeviceIds: string[];
	retryable: boolean;
}

function compare(a: string, b: string): number {
	return a < b ? -1 : a > b ? 1 : 0;
}

function secondsOf(row: FeedRow): number {
	const seconds = row.createdAt?.seconds;
	return seconds === undefined ? 0 : Number(seconds);
}

export function operationIdentity(row: FeedRow): string {
	return row.actionId ? `action:${row.actionId}` : `inline:${row.type}`;
}

function emptyCounts(): Record<EffectBucket, number> {
	return { failed: 0, queued: 0, ok: 0, skipped: 0, cancelled: 0, unknown: 0 };
}

/** Sort label for an effect row: hostname when the index has one, id otherwise. */
function effectName(row: FeedRow): string {
	return row.deviceHostname || row.deviceId;
}

/** The one actor the whole cluster agrees on, or '' when they disagree or none
 *  of them names one (which is every row that came from the search index). */
function singleActor(cluster: readonly FeedRow[]): string {
	let actor = '';
	for (const row of cluster) {
		const createdBy = row.createdBy ?? '';
		if (!createdBy) continue;
		if (!actor) actor = createdBy;
		else if (actor !== createdBy) return '';
	}
	return actor;
}

/** `cluster` arrives created_at-ascending, so its first row starts the operation. */
function makeOperation<R extends FeedRow>(identity: string, cluster: R[]): Operation<R> {
	const startedAtSeconds = secondsOf(cluster[0]);
	const effects = [...cluster].sort(
		(a, b) =>
			BUCKET_ORDER.indexOf(statusBucket(a.status)) -
				BUCKET_ORDER.indexOf(statusBucket(b.status)) ||
			compare(effectName(a), effectName(b)) ||
			compare(a.id, b.id)
	);

	const counts = emptyCounts();
	const deviceIds: string[] = [];
	const retryDeviceIds: string[] = [];
	const seenDevices = new Set<string>();
	const seenFailed = new Set<string>();
	for (const row of effects) {
		const bucket = statusBucket(row.status);
		counts[bucket]++;
		if (row.deviceId && !seenDevices.has(row.deviceId)) {
			seenDevices.add(row.deviceId);
			deviceIds.push(row.deviceId);
		}
		if (bucket === 'failed' && row.deviceId && !seenFailed.has(row.deviceId)) {
			seenFailed.add(row.deviceId);
			retryDeviceIds.push(row.deviceId);
		}
	}

	const actionId = cluster[0].actionId;
	return {
		key: `${identity}@${startedAtSeconds}`,
		actionId,
		actionName: cluster.find((row) => row.actionName)?.actionName ?? '',
		type: cluster[0].type,
		effects,
		counts,
		deviceIds,
		startedAtSeconds,
		startedAt: cluster[0].createdAt,
		actor: singleActor(cluster),
		retryDeviceIds,
		// An inline action leaves no ManagedAction reference behind, so
		// DispatchToMultiple has nothing to re-dispatch; the request also caps
		// device_ids at 256.
		retryable:
			actionId !== '' && retryDeviceIds.length > 0 && retryDeviceIds.length <= RETRY_MAX_DEVICES
	};
}

/** Cluster execution rows into operations, newest operation first. */
export function groupOperations<R extends FeedRow>(
	rows: readonly R[] | null | undefined,
	windowSeconds: number = OPERATION_WINDOW_SECONDS
): Operation<R>[] {
	if (!rows || rows.length === 0) return [];

	const byIdentity = new Map<string, R[]>();
	for (const row of rows) {
		const identity = operationIdentity(row);
		const existing = byIdentity.get(identity);
		if (existing) existing.push(row);
		else byIdentity.set(identity, [row]);
	}

	const operations: Operation<R>[] = [];
	for (const [identity, group] of byIdentity) {
		const ordered = [...group].sort(
			(a, b) => secondsOf(a) - secondsOf(b) || compare(a.id, b.id)
		);
		let cluster: R[] = [];
		let previousSeconds = 0;
		for (const row of ordered) {
			const seconds = secondsOf(row);
			if (cluster.length > 0 && seconds - previousSeconds > windowSeconds) {
				operations.push(makeOperation(identity, cluster));
				cluster = [];
			}
			cluster.push(row);
			previousSeconds = seconds;
		}
		if (cluster.length > 0) operations.push(makeOperation(identity, cluster));
	}

	// Newest first; the key breaks ties so the order never depends on Map
	// insertion order, i.e. on the order the server happened to return rows in.
	operations.sort((a, b) => b.startedAtSeconds - a.startedAtSeconds || compare(a.key, b.key));
	return operations;
}
