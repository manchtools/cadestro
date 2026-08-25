// Pure fleet derivations for the semantic-zoom surface (concepts A4 + round-2
// movement A). Everything here is computed from fields the control contract
// actually returns on Device / DeviceGroup — no state is invented, and there is
// deliberately no 'info' / converging tone because nothing on a Device says an
// operation is currently landing on it.
import { ComplianceStatus, DeviceStatus } from '$contract/cadestro/v1/common_pb';
import type { Device } from '$contract/cadestro/v1/control_pb';
import type { FleetTone } from '$lib/components/fleet/tone';

/** Server default when neither the device nor any of its groups overrides it
 *  (control.proto: Device.sync_interval_minutes "0 = use default of 30"). */
export const DEFAULT_SYNC_MINUTES = 30;

/** Decay steps as MULTIPLES of the resolved cadence, not wall-clock constants —
 *  round 2's open call. Device carries no heartbeat field; sync_interval_minutes
 *  is the finest real cadence the contract exposes, so one interval is "fresh"
 *  and the stale steps are 2x and 8x of it. */
export const AGE_MULTIPLES = [1, 2, 8] as const;

export type AgeStep = 0 | 1 | 2 | 3;

export interface FleetDevice {
	id: string;
	hostname: string;
	/** ok | warn (drift) | crit (offline) | idle (never seen) — never 'info'. */
	tone: FleetTone;
	age: AgeStep;
	lastSeenSec: number;
	/** Resolved cadence the age step was measured against (minutes). */
	syncMinutes: number;
	device: Device;
}

function seconds(ts: { seconds: bigint } | undefined): number {
	return ts ? Number(ts.seconds) : 0;
}

/**
 * The one status classifier. The buckets are mutually exclusive by
 * construction, so the summary strip's counts always sum to the fleet size and
 * a tile's colour is exactly the bucket its device was counted in.
 */
export function deviceTone(d: Device): FleetTone {
	if (!d.lastSeenAt || seconds(d.lastSeenAt) <= 0) return 'idle'; // never seen — hollow
	if (d.status !== DeviceStatus.ONLINE) return 'crit'; // offline — corner notch
	if (
		d.complianceStatus === ComplianceStatus.NON_COMPLIANT ||
		d.complianceStatus === ComplianceStatus.IN_GRACE_PERIOD
	) {
		return 'warn'; // compliance drift — dot
	}
	return 'ok';
}

/** Resolved sync cadence in minutes. A group override wins over the device
 *  ("takes precedence over device-level setting"); with several groups the
 *  smallest non-zero one wins, mirroring the contract's documented MIN-across-
 *  groups resolution for the inventory interval. */
export function resolveSyncMinutes(deviceMinutes: number, groupMinutes: number[]): number {
	const overrides = groupMinutes.filter((n) => n > 0);
	if (overrides.length > 0) return Math.min(...overrides);
	return deviceMinutes > 0 ? deviceMinutes : DEFAULT_SYNC_MINUTES;
}

export function ageBucket(lastSeenSec: number, nowSec: number, syncMinutes: number): AgeStep {
	if (lastSeenSec <= 0) return 0;
	const ageMinutes = (nowSec - lastSeenSec) / 60;
	if (ageMinutes <= syncMinutes * AGE_MULTIPLES[0]) return 0;
	if (ageMinutes <= syncMinutes * AGE_MULTIPLES[1]) return 1;
	if (ageMinutes <= syncMinutes * AGE_MULTIPLES[2]) return 2;
	return 3;
}

export function toFleetDevice(d: Device, groupMinutes: number[], nowSec: number): FleetDevice {
	const syncMinutes = resolveSyncMinutes(d.syncIntervalMinutes, groupMinutes);
	const lastSeenSec = seconds(d.lastSeenAt);
	return {
		id: (d.id?.value ?? ''),
		hostname: d.hostname,
		tone: deviceTone(d),
		age: ageBucket(lastSeenSec, nowSec, syncMinutes),
		lastSeenSec,
		syncMinutes,
		device: d
	};
}

export interface FleetSummary {
	total: number;
	ok: number;
	warn: number;
	crit: number;
	idle: number;
}

/** Counts per tone. Because deviceTone's buckets are exclusive,
 *  ok + warn + crit + idle === total is an invariant, not a coincidence. */
export function summarize(devices: FleetDevice[]): FleetSummary {
	const out: FleetSummary = { total: devices.length, ok: 0, warn: 0, crit: 0, idle: 0 };
	for (const d of devices) {
		if (d.tone === 'ok') out.ok++;
		else if (d.tone === 'warn') out.warn++;
		else if (d.tone === 'crit') out.crit++;
		else if (d.tone === 'idle') out.idle++;
	}
	return out;
}

/** A device is "down" when control cannot reach it: offline, or never seen. */
export function isDown(d: FleetDevice): boolean {
	return d.tone === 'crit' || d.tone === 'idle';
}

/** Worst first: unreachable before drift before healthy; within a tone the
 *  stalest first; ties broken by hostname so the order is deterministic. */
const TONE_RANK: Record<FleetTone, number> = { crit: 0, idle: 1, warn: 2, info: 3, ok: 4 };

export function worstFirst(devices: FleetDevice[]): FleetDevice[] {
	return [...devices].sort(
		(a, b) =>
			TONE_RANK[a.tone] - TONE_RANK[b.tone] ||
			b.age - a.age ||
			a.hostname.localeCompare(b.hostname)
	);
}

export const UNGROUPED_ID = '__ungrouped__';

export interface FleetBubble {
	id: string;
	name: string;
	devices: FleetDevice[];
	down: number;
}

/**
 * One bubble per device group plus a trailing "ungrouped" bubble. A device in
 * several groups appears in each of them — that is what the membership rows
 * say, and hiding the overlap would misreport group size. Groups sort
 * worst-first (most unreachable members first) so triage is the default view;
 * ungrouped always trails.
 */
export function buildBubbles(
	devices: FleetDevice[],
	groups: { id: string; name: string }[],
	membership: Map<string, string[]>,
	ungroupedName: string
): FleetBubble[] {
	const byId = new Map(devices.map((d) => [d.id, d]));
	const grouped = new Set<string>();
	const bubbles: FleetBubble[] = [];

	for (const g of groups) {
		const members: FleetDevice[] = [];
		for (const id of membership.get(g.id) ?? []) {
			const d = byId.get(id);
			if (!d) continue; // outside this surface's scope (e.g. not one of my devices)
			members.push(d);
			grouped.add(id);
		}
		if (members.length === 0) continue;
		bubbles.push({ id: g.id, name: g.name, devices: worstFirst(members), down: members.filter(isDown).length });
	}

	bubbles.sort((a, b) => b.down - a.down || a.name.localeCompare(b.name));

	const loose = devices.filter((d) => !grouped.has(d.id));
	if (loose.length > 0) {
		bubbles.push({
			id: UNGROUPED_ID,
			name: ungroupedName,
			devices: worstFirst(loose),
			down: loose.filter(isDown).length
		});
	}
	return bubbles;
}

/** Selection implications for the pill caption — both numbers are read off the
 *  same bubbles the operator clicked in, never estimated. */
export function selectionFacts(
	selectedIds: readonly string[],
	bubbles: FleetBubble[]
): { groups: number; offline: number } {
	const selected = new Set(selectedIds);
	let groups = 0;
	const counted = new Set<string>();
	let offline = 0;
	for (const b of bubbles) {
		let hit = false;
		for (const d of b.devices) {
			if (!selected.has(d.id)) continue;
			hit = true;
			if (!counted.has(d.id)) {
				counted.add(d.id);
				if (isDown(d)) offline++;
			}
		}
		if (hit) groups++;
	}
	return { groups, offline };
}
