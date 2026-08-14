// Eligibility for the carried selection — B2's three stage rows.
//
// Every count here is DERIVED from two real reads and nothing else: the
// device's server-computed DeviceStatus, and whether an ACTION_SET→DEVICE
// assignment for the chosen set already targets it. There is no heuristic and
// no placeholder: a device whose read failed lands in `unknown`, never in
// `queued`, because "offline" is a server fact and a failed read is not one.
//
// Bucket precedence — update > queued > ready — mirrors the caption's reading
// order: an already-assigned device is an update whatever its connectivity,
// which is why the same map feeds the stage rows, the pill caption and the
// commit count (one map, three renderings, no drift).

import { DeviceStatus } from '$contract/cadestro/v1/common_pb';

export type EligibilityBucket = 'ready' | 'update' | 'queued' | 'unknown';

export interface CarriedDevice {
	id: string;
	/** Empty when the device read failed. */
	hostname: string;
	/** `undefined` when the device read failed — unknown, NOT offline. */
	status?: DeviceStatus;
}

export interface Eligibility {
	/** Per-device bucket, so a row and a tile can never disagree. */
	bucket: Map<string, EligibilityBucket>;
	ready: number;
	update: number;
	queued: number;
	unknown: number;
}

export function computeEligibility(
	devices: readonly CarriedDevice[],
	assigned: ReadonlySet<string>
): Eligibility {
	const bucket = new Map<string, EligibilityBucket>();
	let ready = 0;
	let update = 0;
	let queued = 0;
	let unknown = 0;
	for (const d of devices) {
		let b: EligibilityBucket;
		if (assigned.has(d.id)) {
			b = 'update';
			update++;
		} else if (d.status === DeviceStatus.ONLINE) {
			b = 'ready';
			ready++;
		} else if (d.status === DeviceStatus.OFFLINE) {
			b = 'queued';
			queued++;
		} else {
			b = 'unknown';
			unknown++;
		}
		bucket.set(d.id, b);
	}
	return { bucket, ready, update, queued, unknown };
}

/** Tile tone for a device — plain connectivity, so the grid reads the fleet's
 *  own vocabulary. Eligibility lives in the rows, not in the tiles. */
export function statusTone(status: DeviceStatus | undefined): 'ok' | 'crit' | 'idle' {
	if (status === DeviceStatus.ONLINE) return 'ok';
	if (status === DeviceStatus.OFFLINE) return 'crit';
	return 'idle';
}

export interface HostnameGroup {
	name: string;
	count: number;
}

/** "api-prod-01…08 · 8" — the concept's breakdown lines. The group key is the
 *  hostname minus a trailing numeric segment, which is the only shape the
 *  fleet's own naming guarantees; anything else groups under itself rather
 *  than inventing a hierarchy. Unreadable devices carry no hostname and are
 *  left out — they are already accounted for by the `unknown` row. */
export function hostnameGroups(devices: readonly CarriedDevice[]): HostnameGroup[] {
	const counts = new Map<string, number>();
	for (const d of devices) {
		if (!d.hostname) continue;
		const parts = d.hostname.split('-');
		const name =
			parts.length > 1 && /^\d+$/.test(parts[parts.length - 1])
				? parts.slice(0, -1).join('-')
				: d.hostname;
		counts.set(name, (counts.get(name) ?? 0) + 1);
	}
	return [...counts.entries()]
		.map(([name, count]) => ({ name, count }))
		.sort((a, b) => b.count - a.count || a.name.localeCompare(b.name));
}
