

import { describe, it, expect } from 'vitest';
import { create } from '@bufbuild/protobuf';
import { TimestampSchema } from '@bufbuild/protobuf/wkt';
import { ComplianceStatus, DeviceStatus } from '$contract/cadestro/v1/common_pb';
import { DeviceSchema } from '$contract/cadestro/v1/control_pb';
import {
	DEFAULT_SYNC_MINUTES,
	ageBucket,
	buildBubbles,
	deviceTone,
	resolveSyncMinutes,
	selectionFacts,
	summarize,
	toFleetDevice,
	worstFirst,
	UNGROUPED_ID
} from './fleet-model';

const NOW = 1_800_000_000;

function device(over: {
	id: string;
	hostname?: string;
	status?: DeviceStatus;
	compliance?: ComplianceStatus;
	lastSeenSec?: number | null;
	syncIntervalMinutes?: number;
}) {
	return create(DeviceSchema, {
		id: { value: over.id },
		hostname: over.hostname ?? over.id,
		status: over.status ?? DeviceStatus.ONLINE,
		complianceStatus: over.compliance ?? ComplianceStatus.COMPLIANT,
		syncIntervalMinutes: over.syncIntervalMinutes ?? 0,
		lastSeenAt:
			over.lastSeenSec === null
				? undefined
				: create(TimestampSchema, { seconds: BigInt(over.lastSeenSec ?? NOW) })
	});
}

describe('deviceTone — the one classifier', () => {
	it('never-seen outranks every other signal: no heartbeat means no claim about health', () => {
		expect(
			deviceTone(device({ id: 'a', lastSeenSec: null, compliance: ComplianceStatus.NON_COMPLIANT }))
		).toBe('idle');

		expect(deviceTone(device({ id: 'b', lastSeenSec: 0 }))).toBe('idle');
	});

	it('unreachable is critical, drift is a warning, and grace period still counts as drift', () => {
		expect(deviceTone(device({ id: 'a', status: DeviceStatus.OFFLINE }))).toBe('crit');
		expect(deviceTone(device({ id: 'b', compliance: ComplianceStatus.NON_COMPLIANT }))).toBe('warn');
		expect(deviceTone(device({ id: 'c', compliance: ComplianceStatus.IN_GRACE_PERIOD }))).toBe('warn');
		expect(deviceTone(device({ id: 'd' }))).toBe('ok');
	});

	it('never returns the in-flight tone — no field on Device says an operation is landing', () => {
		const every = [
			device({ id: 'a' }),
			device({ id: 'b', status: DeviceStatus.OFFLINE }),
			device({ id: 'c', compliance: ComplianceStatus.NON_COMPLIANT }),
			device({ id: 'd', compliance: ComplianceStatus.UNKNOWN }),
			device({ id: 'e', lastSeenSec: null })
		];
		expect(every.map(deviceTone)).not.toContain('info');
	});
});

describe('summarize', () => {
	it('the strip always adds up to the fleet — a device can only be counted once', () => {
		const devices = [
			device({ id: 'a' }),
			device({ id: 'b', compliance: ComplianceStatus.NON_COMPLIANT }),
			device({ id: 'c', status: DeviceStatus.OFFLINE }),
			device({ id: 'd', lastSeenSec: null }),

			device({ id: 'e', status: DeviceStatus.OFFLINE, compliance: ComplianceStatus.NON_COMPLIANT })
		].map((d) => toFleetDevice(d, [], NOW));

		const s = summarize(devices);
		expect(s).toEqual({ total: 5, ok: 1, warn: 1, crit: 2, idle: 1 });
		expect(s.ok + s.warn + s.crit + s.idle).toBe(s.total);
	});
});

describe('sync-cadence decay', () => {
	it('a group interval outranks the device interval, and the smallest group wins', () => {
		expect(resolveSyncMinutes(60, [])).toBe(60);
		expect(resolveSyncMinutes(60, [10, 45])).toBe(10);
		expect(resolveSyncMinutes(0, [])).toBe(DEFAULT_SYNC_MINUTES);

		expect(resolveSyncMinutes(20, [0, 0])).toBe(20);
	});

	it('steps at 1x / 2x / 8x of the RESOLVED cadence, not at a hardcoded clock', () => {
		const at = (minutesAgo: number, cadence: number) =>
			ageBucket(NOW - minutesAgo * 60, NOW, cadence);

		expect(at(9, 10)).toBe(0);
		expect(at(10, 10)).toBe(0);
		expect(at(11, 10)).toBe(1);
		expect(at(20, 10)).toBe(1);
		expect(at(21, 10)).toBe(2);
		expect(at(80, 10)).toBe(2);
		expect(at(81, 10)).toBe(3);

		expect(at(45, 60)).toBe(0);
		expect(at(45, 10)).toBe(2);
	});

	it('never-seen devices get no decay step at all', () => {
		expect(ageBucket(0, NOW, 30)).toBe(0);
	});
});

describe('worstFirst', () => {
	it('unreachable, then never-seen, then drift, then healthy; stalest first inside a tone', () => {
		const rows = [
			toFleetDevice(device({ id: 'ok-1' }), [], NOW),
			toFleetDevice(device({ id: 'warn-1', compliance: ComplianceStatus.NON_COMPLIANT }), [], NOW),
			toFleetDevice(device({ id: 'never-1', lastSeenSec: null }), [], NOW),
			toFleetDevice(
				device({ id: 'crit-fresh', status: DeviceStatus.OFFLINE, lastSeenSec: NOW - 60 }),
				[],
				NOW
			),
			toFleetDevice(
				device({ id: 'crit-stale', status: DeviceStatus.OFFLINE, lastSeenSec: NOW - 60 * 60 * 24 }),
				[],
				NOW
			)
		];

		expect(worstFirst(rows).map((d) => d.id)).toEqual([
			'crit-stale',
			'crit-fresh',
			'never-1',
			'warn-1',
			'ok-1'
		]);
	});

	it('does not mutate its input — the far pane and near pane share the same rows', () => {
		const rows = [
			toFleetDevice(device({ id: 'ok-1' }), [], NOW),
			toFleetDevice(device({ id: 'crit-1', status: DeviceStatus.OFFLINE }), [], NOW)
		];
		worstFirst(rows);
		expect(rows.map((d) => d.id)).toEqual(['ok-1', 'crit-1']);
	});
});

describe('buildBubbles', () => {
	const devices = [
		device({ id: 'd1' }),
		device({ id: 'd2', status: DeviceStatus.OFFLINE }),
		device({ id: 'd3' }),
		device({ id: 'd4', lastSeenSec: null })
	].map((d) => toFleetDevice(d, [], NOW));

	it('gives every un-grouped device the trailing ungrouped bubble', () => {
		const bubbles = buildBubbles(
			devices,
			[{ id: 'g1', name: 'api' }],
			new Map([['g1', ['d1', 'd2']]]),
			'ungrouped'
		);

		expect(bubbles.map((b) => b.id)).toEqual(['g1', UNGROUPED_ID]);
		expect(bubbles[1].devices.map((d) => d.id)).toEqual(['d4', 'd3']);
		expect(bubbles[0].down).toBe(1);
		expect(bubbles[1].down).toBe(1);
	});

	it('keeps a device that is in two groups visible in both — membership is not deduplicated away', () => {
		const bubbles = buildBubbles(
			devices,
			[
				{ id: 'g1', name: 'api' },
				{ id: 'g2', name: 'berlin' }
			],
			new Map([
				['g1', ['d1']],
				['g2', ['d1']]
			]),
			'ungrouped'
		);

		expect(bubbles.find((b) => b.id === 'g1')!.devices.map((d) => d.id)).toEqual(['d1']);
		expect(bubbles.find((b) => b.id === 'g2')!.devices.map((d) => d.id)).toEqual(['d1']);

		expect(bubbles.find((b) => b.id === UNGROUPED_ID)!.devices.map((d) => d.id)).not.toContain('d1');
	});

	it('drops a group whose members are outside this surface, and sorts the rest worst-first', () => {
		const bubbles = buildBubbles(
			devices,
			[
				{ id: 'healthy', name: 'healthy' },
				{ id: 'broken', name: 'broken' },
				{ id: 'elsewhere', name: 'elsewhere' }
			],
			new Map([
				['healthy', ['d1', 'd3']],
				['broken', ['d2', 'd4']],
				['elsewhere', ['not-on-this-surface']]
			]),
			'ungrouped'
		);

		expect(bubbles.map((b) => b.id)).toEqual(['broken', 'healthy']);
		expect(bubbles[0].down).toBe(2);
	});
});

describe('selectionFacts', () => {
	it('counts the groups a selection actually spans and the members that cannot be reached now', () => {
		const devices = [
			device({ id: 'd1' }),
			device({ id: 'd2', status: DeviceStatus.OFFLINE }),
			device({ id: 'd3', lastSeenSec: null }),
			device({ id: 'd4' })
		].map((d) => toFleetDevice(d, [], NOW));
		const bubbles = buildBubbles(
			devices,
			[
				{ id: 'g1', name: 'a' },
				{ id: 'g2', name: 'b' }
			],
			new Map([
				['g1', ['d1', 'd2']],
				['g2', ['d3']]
			]),
			'ungrouped'
		);

		expect(selectionFacts(['d1'], bubbles)).toEqual({ groups: 1, offline: 0 });
		expect(selectionFacts(['d2', 'd3'], bubbles)).toEqual({ groups: 2, offline: 2 });

		expect(selectionFacts(['d1', 'd4'], bubbles)).toEqual({ groups: 2, offline: 0 });
	});

	it('counts a device in two groups once for offline, but in both group counts', () => {
		const devices = [toFleetDevice(device({ id: 'd1', status: DeviceStatus.OFFLINE }), [], NOW)];
		const bubbles = buildBubbles(
			devices,
			[
				{ id: 'g1', name: 'a' },
				{ id: 'g2', name: 'b' }
			],
			new Map([
				['g1', ['d1']],
				['g2', ['d1']]
			]),
			'ungrouped'
		);

		expect(selectionFacts(['d1'], bubbles)).toEqual({ groups: 2, offline: 1 });
	});
});
