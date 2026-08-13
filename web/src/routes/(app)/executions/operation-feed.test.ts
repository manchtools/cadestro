// The grouping rule, pinned.
//
// ActionExecution carries no dispatch/batch/operation id (see
// ./operation-feed.ts), so the feed CLUSTERS rows. These tests are what makes
// that rule a contract rather than a heuristic nobody can check: same action +
// same dispatch window group, a different action or a later gesture does not,
// every status lands in exactly one bucket, and the retry subset is exactly the
// failed devices.

import { describe, it, expect } from 'vitest';
import { create } from '@bufbuild/protobuf';
import { TimestampSchema } from '@bufbuild/protobuf/wkt';
import { ExecutionStatus } from '$sdk/powermanage/v1/common_pb';
import { ActionType } from '$sdk/powermanage/v1/actions_pb';
import {
	BUCKET_ORDER,
	OPERATION_WINDOW_SECONDS,
	RETRY_MAX_DEVICES,
	groupOperations,
	operationIdentity,
	statusBucket,
	type FeedRow
} from './operation-feed';

const ACTION_A = '01JQZZ6C9P5Q1S4T8V0W3X2Y7Z';
const ACTION_B = '01JQZZ7D0Q6R2T5V9W1X4Y3Z8A';

/** Rows carry real Timestamps, exactly as the search adapter builds them. */
function at(seconds: bigint) {
	return create(TimestampSchema, { seconds, nanos: 0 });
}

let seq = 0;
function row(over: Partial<FeedRow> = {}): FeedRow {
	seq += 1;
	const id = `01EXEC${String(seq).padStart(20, '0')}`;
	return {
		id,
		actionId: ACTION_A,
		actionName: 'Patch & reboot v7',
		type: ActionType.PACKAGE,
		deviceId: `01DEV${String(seq).padStart(21, '0')}`,
		deviceHostname: `host-${String(seq).padStart(2, '0')}`,
		status: ExecutionStatus.SUCCESS,
		createdAt: at(1_750_000_000n),
		...over
	};
}

describe('operation grouping — one card per dispatch gesture', () => {
	it('groups rows that share an action id and a dispatch second', () => {
		const rows = [row(), row(), row()];

		const operations = groupOperations(rows);

		expect(operations).toHaveLength(1);
		expect(operations[0].effects.map((e) => e.id).sort()).toEqual(rows.map((r) => r.id).sort());
		expect(operations[0].deviceIds).toHaveLength(3);
	});

	it('does not group two different actions dispatched at the same instant', () => {
		const operations = groupOperations([row(), row({ actionId: ACTION_B, actionName: 'Other' })]);

		expect(operations).toHaveLength(2);
		expect(new Set(operations.map((o) => o.actionId))).toEqual(new Set([ACTION_A, ACTION_B]));
	});

	it('splits the same action into two operations once the gap exceeds the window', () => {
		const first = 1_750_000_000n;
		const later = first + BigInt(OPERATION_WINDOW_SECONDS + 1);

		const operations = groupOperations([
			row({ createdAt: at(first) }),
			row({ createdAt: at(later) })
		]);

		expect(operations).toHaveLength(2);
		// Newest operation first.
		expect(operations[0].startedAtSeconds).toBe(Number(later));
	});

	it('keeps one operation while consecutive rows stay inside the window', () => {
		const base = 1_750_000_000n;
		const operations = groupOperations([
			row({ createdAt: at(base) }),
			row({ createdAt: at(base + BigInt(OPERATION_WINDOW_SECONDS)) }),
			row({ createdAt: at(base + BigInt(2 * OPERATION_WINDOW_SECONDS)) })
		]);

		expect(operations).toHaveLength(1);
		expect(operations[0].effects).toHaveLength(3);
	});

	it('gives inline executions an identity from the action type, never a fabricated id', () => {
		const inlineShell = row({ actionId: '', actionName: '', type: ActionType.SHELL });
		const inlinePackage = row({ actionId: '', actionName: '', type: ActionType.PACKAGE });

		expect(operationIdentity(inlineShell)).toBe(`inline:${ActionType.SHELL}`);
		expect(groupOperations([inlineShell, inlinePackage])).toHaveLength(2);
	});

	it('produces the same operations whatever order the server returned the rows in', () => {
		const rows = [
			row({ createdAt: at(1_750_000_000n) }),
			row({ createdAt: at(1_750_000_000n) }),
			row({ actionId: ACTION_B, createdAt: at(1_750_000_500n) })
		];

		const forward = groupOperations(rows).map((o) => o.key);
		const reversed = groupOperations([...rows].reverse()).map((o) => o.key);

		expect(reversed).toEqual(forward);
	});

	it('returns nothing for an empty or missing row set', () => {
		expect(groupOperations([])).toEqual([]);
		expect(groupOperations(null)).toEqual([]);
		expect(groupOperations(undefined)).toEqual([]);
	});
});

describe('operation grouping — effect ordering and counts', () => {
	it('orders effect rows failed-first', () => {
		const operations = groupOperations([
			row({ status: ExecutionStatus.SUCCESS, deviceHostname: 'aaa-ok' }),
			row({ status: ExecutionStatus.PENDING, deviceHostname: 'bbb-queued' }),
			row({ status: ExecutionStatus.FAILED, deviceHostname: 'zzz-failed' })
		]);

		expect(operations[0].effects.map((e) => e.deviceHostname)).toEqual([
			'zzz-failed',
			'bbb-queued',
			'aaa-ok'
		]);
	});

	it('counts every effect exactly once, so the chips add up to the effect rows', () => {
		const statuses = [
			ExecutionStatus.SUCCESS,
			ExecutionStatus.FAILED,
			ExecutionStatus.TIMEOUT,
			ExecutionStatus.INDETERMINATE,
			ExecutionStatus.PENDING,
			ExecutionStatus.RUNNING,
			ExecutionStatus.SCHEDULED,
			ExecutionStatus.SKIPPED,
			ExecutionStatus.NOT_APPLICABLE,
			ExecutionStatus.CANCELLED,
			ExecutionStatus.UNSPECIFIED
		];
		const operations = groupOperations(statuses.map((status) => row({ status })));

		const counts = operations[0].counts;
		expect(Object.values(counts).reduce((a, b) => a + b, 0)).toBe(statuses.length);
		expect(counts).toEqual({
			ok: 1,
			failed: 3,
			queued: 3,
			skipped: 2,
			cancelled: 1,
			unknown: 1
		});
	});

	it('has a bucket for every ExecutionStatus the SDK defines', () => {
		// Matches-zero guard plus the parity check: a status added to the proto
		// without a bucket would silently fall into 'unknown' and be uncountable
		// as anything an operator can act on.
		const names = Object.keys(ExecutionStatus).filter((k) => isNaN(Number(k)));
		expect(names.length).toBeGreaterThan(3);
		for (const name of names) {
			if (name === 'UNSPECIFIED') continue;
			const value = ExecutionStatus[name as keyof typeof ExecutionStatus] as number;
			expect(BUCKET_ORDER, `${name} has no effect bucket`).toContain(statusBucket(value));
			expect(statusBucket(value), `${name} fell through to 'unknown'`).not.toBe('unknown');
		}
	});
});

describe('operation grouping — the retry subset', () => {
	it('offers exactly the failed devices, deduplicated, and nothing else', () => {
		const okDevice = '01DEVOK00000000000000000000';
		const failedDevice = '01DEVFAIL0000000000000000000';
		const operations = groupOperations([
			row({ status: ExecutionStatus.SUCCESS, deviceId: okDevice }),
			row({ status: ExecutionStatus.FAILED, deviceId: failedDevice }),
			// A device can carry two effects of one operation; retry sends it once.
			row({ status: ExecutionStatus.TIMEOUT, deviceId: failedDevice }),
			row({ status: ExecutionStatus.PENDING, deviceId: '01DEVQUEUED00000000000000000' })
		]);

		expect(operations[0].retryDeviceIds).toEqual([failedDevice]);
		expect(operations[0].retryable).toBe(true);
	});

	it('refuses to retry an inline action, which leaves no action reference', () => {
		const operations = groupOperations([
			row({ actionId: '', actionName: '', status: ExecutionStatus.FAILED })
		]);

		expect(operations[0].retryDeviceIds).toHaveLength(1);
		expect(operations[0].retryable).toBe(false);
	});

	it('refuses a subset larger than DispatchToMultiple accepts', () => {
		const rows = Array.from({ length: RETRY_MAX_DEVICES + 1 }, () =>
			row({ status: ExecutionStatus.FAILED })
		);

		const operations = groupOperations(rows);

		expect(operations[0].retryDeviceIds).toHaveLength(RETRY_MAX_DEVICES + 1);
		expect(operations[0].retryable).toBe(false);
	});

	it('names an actor only when every row that has one agrees', () => {
		expect(groupOperations([row({ createdBy: 'ada' }), row({ createdBy: 'ada' })])[0].actor).toBe(
			'ada'
		);
		expect(groupOperations([row({ createdBy: 'ada' }), row({ createdBy: 'bo' })])[0].actor).toBe('');
		// Search-index rows carry no created_by at all — the card then shows time only.
		expect(groupOperations([row(), row()])[0].actor).toBe('');
	});
});
