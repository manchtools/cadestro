

import { DeviceStatus } from '$contract/cadestro/v1/common_pb';

export type EligibilityBucket = 'ready' | 'update' | 'queued' | 'unknown';

export interface CarriedDevice {
	id: string;

	hostname: string;

	status?: DeviceStatus;
}

export interface Eligibility {

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

export function statusTone(status: DeviceStatus | undefined): 'ok' | 'crit' | 'idle' {
	if (status === DeviceStatus.ONLINE) return 'ok';
	if (status === DeviceStatus.OFFLINE) return 'crit';
	return 'idle';
}

export interface HostnameGroup {
	name: string;
	count: number;
}

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
