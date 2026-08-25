

import { ComplianceStatus, DeviceStatus } from '$contract/cadestro/v1/common_pb';
import type { Device } from '$contract/cadestro/v1/control_pb';
import type { FleetTone } from '$lib/components/fleet/tone';

export const DEFAULT_SYNC_MINUTES = 30;

export const AGE_MULTIPLES = [1, 2, 8] as const;

export type AgeStep = 0 | 1 | 2 | 3;

export interface FleetDevice {
	id: string;
	hostname: string;

	tone: FleetTone;
	age: AgeStep;
	lastSeenSec: number;

	syncMinutes: number;
	device: Device;
}

function seconds(ts: { seconds: bigint } | undefined): number {
	return ts ? Number(ts.seconds) : 0;
}

export function deviceTone(d: Device): FleetTone {
	if (!d.lastSeenAt || seconds(d.lastSeenAt) <= 0) return 'idle';
	if (d.status !== DeviceStatus.ONLINE) return 'crit';
	if (
		d.complianceStatus === ComplianceStatus.NON_COMPLIANT ||
		d.complianceStatus === ComplianceStatus.IN_GRACE_PERIOD
	) {
		return 'warn';
	}
	return 'ok';
}

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

export function isDown(d: FleetDevice): boolean {
	return d.tone === 'crit' || d.tone === 'idle';
}

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
			if (!d) continue;
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
