// The assign surface's RPC seam. Every call here is one the control service
// really exposes (contract/ts/client.ts); nothing is composed out of a capability
// the server does not have.
//
// Reads
//   status of the carried devices  — GetDevice per id. There is no by-id batch
//     read on the contract (ListDevices filters by status/label only, Search is
//     full-text), so this is a BOUNDED fan-out: MAX_CARRIED ids, READ_CONCURRENCY
//     in flight. A failed read yields status `undefined` — unknown, not offline.
//   devices that already have the set — ONE keyset-paged ListAssignments
//     filtered server-side to (source_type=ACTION_SET, source_id=set,
//     target_type=DEVICE). That is the cheapest real call: GetDeviceAssignments
//     would be one round trip per device for the same answer. It sees DIRECT
//     device assignments — the exact edge CreateAssignment would write.
//
// Writes
//   CreateAssignment(ACTION_SET, set, DEVICE, device, REQUIRED) per device —
//     the server treats a repeated active tuple as idempotent and returns the
//     existing row, which is what makes "will update in place" honest.
//   DispatchActionSet(device, set) per device, only for the `now` schedule.
//     DispatchActionSetRequest carries neither run_at nor
//     respect_maintenance_window (unlike DispatchAction), so a timed or
//     maintenance-window dispatch of a SET is not on the contract and this
//     surface does not offer one.
//
// B3 — targeting by rule
//   The rule is not a target type of its own: an Assignment targets a
//   DEVICE_GROUP, so "assign this set to whatever matches" is exactly
//   CreateDeviceGroup(is_dynamic=true, dynamic_query=<compiled string>) followed
//   by CreateAssignment(ACTION_SET, set, DEVICE_GROUP, group, REQUIRED). Two
//   RPCs, in that order, because the second needs the first's id.
//   The count behind the rule comes from ValidateDynamicQuery — the only RPC
//   that answers for an ARBITRARY query. EvaluateDynamicGroup takes a group id
//   and MUTATES membership, so it is never used as a preview.
//   Nothing is dispatched for a rule target: DispatchActionSet is per DEVICE and
//   the matching devices of an unsaved rule are not listable (ListDevices
//   filters by status/labels only), so this surface refuses to guess a fan-out.

import { apiClient } from '$lib/sdk';
import { fetchAllPages } from '$lib/sdk/paginate';
import { getLocalizedError } from '$lib/errors';
import {
	AssignmentMode,
	AssignmentSourceType,
	AssignmentTargetType
} from '$contract/cadestro/v1/common_pb';
import type { ActionSet, ActionSetMember } from '$contract/cadestro/v1/control_pb';
import type { CarriedDevice } from './eligibility';

/** Fan-out bound. One request per device is the contract's only shape for
 *  both the status read and the assignment write, so the surface refuses a
 *  selection it cannot honestly service in one commit. */
export const MAX_CARRIED = 256;

const READ_CONCURRENCY = 8;
const WRITE_CONCURRENCY = 4;

/** The two schedules the contract actually supports for an action set. */
export type AssignSchedule = 'now' | 'on_schedule';

/** Order-preserving bounded-concurrency map. */
async function mapLimit<T, R>(
	items: readonly T[],
	limit: number,
	fn: (item: T) => Promise<R>
): Promise<R[]> {
	const out = new Array<R>(items.length);
	let next = 0;
	const workers = Array.from({ length: Math.min(limit, items.length) }, async () => {
		for (;;) {
			const i = next++;
			if (i >= items.length) return;
			out[i] = await fn(items[i]);
		}
	});
	await Promise.all(workers);
	return out;
}

export async function loadCarriedDevices(ids: readonly string[]): Promise<CarriedDevice[]> {
	return mapLimit(ids, READ_CONCURRENCY, async (id) => {
		try {
			const device = await apiClient.getDevice(id);
			return { id, hostname: device?.hostname ?? '', status: device?.status };
		} catch {
			// A device that cannot be read is unknown. Reporting it as offline
			// would invent a server fact.
			return { id, hostname: '', status: undefined };
		}
	});
}

export async function loadActionSets(): Promise<ActionSet[]> {
	return fetchAllPages(async (pageSize, pageToken) => {
		const page = await apiClient.listActionSets(pageSize, pageToken);
		return { items: page.sets ?? [], nextPageToken: page.nextPageToken ?? '' };
	});
}

export async function loadSetSteps(setId: string): Promise<ActionSetMember[]> {
	const response = await apiClient.getActionSet(setId);
	return [...(response.members ?? [])].sort((a, b) => a.sortOrder - b.sortOrder);
}

/** Device ids that already carry a direct assignment for this set. */
export async function loadAssignedDeviceIds(setId: string): Promise<Set<string>> {
	const assignments = await fetchAllPages(async (pageSize, pageToken) => {
		const page = await apiClient.listAssignments(
			pageSize,
			pageToken,
			AssignmentSourceType.ACTION_SET,
			setId,
			AssignmentTargetType.DEVICE,
			''
		);
		return { items: page.assignments ?? [], nextPageToken: page.nextPageToken ?? '' };
	});
	return new Set(assignments.map((a) => a.targetId));
}

/** The dynamic group a rule target is saved as. */
export interface RuleGroup {
	id: string;
	name: string;
}

/** Create the standing rule as a dynamic device group. `query` is passed through
 *  byte for byte — it is the editor's compiled string, and rewriting it here
 *  would make the rule differ from the one that was counted and acknowledged. */
export async function createRuleGroup(name: string, query: string): Promise<RuleGroup> {
	const group = await apiClient.createDeviceGroup(name, '', true, query);
	// A create that answers without a group is not a success we can build on:
	// the assignment needs the id, and inventing one would target nothing.
	if (!group?.id) throw new Error('CreateDeviceGroup returned no group');
	return { id: group.id, name: group.name || name };
}

/** Point the action set at the group. The set applies to whatever the rule
 *  matches, now and later — that is the standing part of a standing rule. */
export async function assignSetToGroup(setId: string, groupId: string): Promise<void> {
	await apiClient.createAssignment(
		AssignmentSourceType.ACTION_SET,
		setId,
		AssignmentTargetType.DEVICE_GROUP,
		groupId,
		AssignmentMode.REQUIRED
	);
}

export interface AssignOutcome {
	deviceId: string;
	ok: boolean;
	/** Localized failure text — present only when `ok` is false. */
	error?: string;
}

/** Assign the set to every carried device. Per-device outcomes are returned
 *  rather than thrown: a partial failure is a real result the operator has to
 *  see device by device, not an exception that hides which ones landed. */
export async function commitAssign(
	deviceIds: readonly string[],
	setId: string,
	schedule: AssignSchedule
): Promise<AssignOutcome[]> {
	return mapLimit(deviceIds, WRITE_CONCURRENCY, async (deviceId) => {
		try {
			await apiClient.createAssignment(
				AssignmentSourceType.ACTION_SET,
				setId,
				AssignmentTargetType.DEVICE,
				deviceId,
				AssignmentMode.REQUIRED
			);
			if (schedule === 'now') {
				await apiClient.dispatchActionSet(deviceId, setId);
			}
			return { deviceId, ok: true };
		} catch (error) {
			return { deviceId, ok: false, error: getLocalizedError(error) };
		}
	});
}
