// The execution watch window — a POLLING surface, so what is load-bearing is
// when it asks and when it stops asking:
//
//   1. it follows a live run: the status chip and the output tail track what
//      GetExecution answers;
//   2. it stops the moment the run reaches a terminal status — a settled run
//      can never change, and a window left open must not keep an RPC in flight
//      forever;
//   3. it stops when it is destroyed, even with a read in flight;
//   4. it is a BACKGROUND surface: a failing read writes an inline note and,
//      after three consecutive failures, gives up instead of hammering.
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { create } from '@bufbuild/protobuf';
import { ActionExecutionSchema } from '$sdk/powermanage/v1/control_pb';
import { ExecutionStatus } from '$sdk/powermanage/v1/common_pb';
import { ActionType } from '$sdk/powermanage/v1/actions_pb';
import * as m from '$lib/paraglide/messages';

const EXEC_ID = '01JQZZ4A7K3M9P2Q6R8T1V0W5X';
const DEVICE_ID = '01JQZZ7D0Q6R2T5V9W1X4Y3Z8A';

const api = vi.hoisted(() => ({ getExecution: vi.fn() }));

vi.mock('$app/paths', () => ({ base: '', assets: '' }));

// Only the client is faked; the generated protobuf re-exports stay real, so the
// component's ExecutionStatus constants are the production ones.
vi.mock('$lib/sdk', async () => {
	const common = await import('$sdk/powermanage/v1/common_pb');
	const control = await import('$sdk/powermanage/v1/control_pb');
	const actions = await import('$sdk/powermanage/v1/actions_pb');
	return {
		...actions,
		...control,
		...common,
		apiClient: api,
		formatTimestamp: () => '—',
		formatTimestampDateTime: () => '2026-08-01 09:00',
		formatDuration: () => '1.2s'
	};
});

import ExecutionWindow from './execution-window.svelte';

/** Poll interval for the tests: short enough to watch several rounds, long
 *  enough that each round is observable before the next one replaces it. */
const POLL_MS = 120;
/** A "did it stop?" window, comfortably wider than several poll intervals. */
const SETTLE_MS = 600;

function execution(status: ExecutionStatus, stdout = '', stderr = '') {
	return create(ActionExecutionSchema, {
		id: EXEC_ID,
		deviceId: DEVICE_ID,
		type: ActionType.SHELL,
		actionName: 'Rotate log files',
		status,
		liveOutput: stdout || stderr ? { stdout, stderr, exitCode: 0 } : undefined
	});
}

const statusChip = () =>
	document.querySelector('[data-testid="execution-window-status"]')?.textContent?.trim() ?? '';
const output = () => document.querySelector<HTMLElement>('[data-testid="execution-window-output"]');
const errorNote = () => document.querySelector<HTMLElement>('[data-testid="execution-window-error"]');

// render() is async in vitest-browser-svelte, and only the RESOLVED result
// carries unmount() — awaiting it is what makes "destroy stops polling" a real
// destroy rather than a silently skipped call.
let mounted: { unmount: () => Promise<void> } | null = null;

async function mount() {
	mounted = await render(ExecutionWindow, { executionId: EXEC_ID, pollMs: POLL_MS });
}

/** Assert the client is no longer being called: the count must be identical
 *  after a window several poll intervals wide. */
async function pollingSettled() {
	const at = api.getExecution.mock.calls.length;
	await new Promise((r) => setTimeout(r, SETTLE_MS));
	expect(api.getExecution.mock.calls.length, 'polling kept running').toBe(at);
	return at;
}

beforeEach(() => {
	document.body.innerHTML = '';
	api.getExecution.mockReset();
});

afterEach(async () => {
	await mounted?.unmount();
	mounted = null;
});

describe('the execution watch window follows one run', () => {
	it('tracks the status and shows the live tail, then stops polling at a terminal status', async () => {
		const lines = Array.from({ length: 14 }, (_, i) => `line ${i + 1}`).join('\n');
		api.getExecution
			.mockResolvedValueOnce(execution(ExecutionStatus.RUNNING, lines))
			.mockResolvedValueOnce(execution(ExecutionStatus.RUNNING, `${lines}\nline 15`))
			.mockResolvedValue(execution(ExecutionStatus.SUCCESS, `${lines}\nline 15`));

		await mount();

		await vi.waitFor(() => expect(statusChip()).toBe(m.executions_status_running()));
		// The tail is a tail: the last ten lines, not the whole stream — and it
		// SAYS it is a tail, so the missing earlier lines are not read as gone.
		await vi.waitFor(() => expect(output()).not.toBeNull());
		expect(output()!.textContent).toContain('line 14');
		expect(output()!.textContent).not.toContain('line 4');
		expect(
			document.querySelector('[data-testid="execution-window-tail-note"]')?.textContent?.trim()
		).toBe(m.executions_watch_tail({ count: 10 }));

		await vi.waitFor(() => expect(statusChip()).toBe(m.executions_status_success()), {
			timeout: 3000
		});

		// A settled run cannot change again, so the window stops asking …
		const calls = await pollingSettled();
		expect(calls).toBeGreaterThanOrEqual(3);
		// … and the live tail belongs to a RUNNING run only.
		expect(output()).toBeNull();
		expect(errorNote()).toBeNull();
	});

	it('stops polling when the window is destroyed', async () => {
		api.getExecution.mockResolvedValue(execution(ExecutionStatus.RUNNING, 'still going'));
		await mount();

		await vi.waitFor(() => expect(api.getExecution.mock.calls.length).toBeGreaterThanOrEqual(2));

		await mounted!.unmount();
		mounted = null;
		await pollingSettled();
	});

	it('writes an inline note for a failed read and gives up after three of them', async () => {
		vi.spyOn(console, 'error').mockImplementation(() => {});
		api.getExecution.mockRejectedValue(new Error('control plane unreachable'));

		await mount();

		await vi.waitFor(() => expect(errorNote()).not.toBeNull());
		expect(errorNote()!.textContent).toContain(m.execution_detail_load_failed());

		// Three consecutive failures end the watch; the note stays, SAYS the watch
		// stopped and why, and asking again becomes a manual control.
		await vi.waitFor(() =>
			expect(document.querySelector('[data-testid="execution-window-stopped"]')).not.toBeNull()
		);
		const stopped = document.querySelector('[data-testid="execution-window-stopped"]')!;
		expect(stopped.textContent).toContain(m.executions_watch_stopped({ count: 3 }));
		expect(stopped.querySelector('button')?.textContent).toContain(m.common_refresh());
		const calls = await pollingSettled();
		expect(calls).toBe(3);

		// No toast from a background window — the note is the whole report.
		expect(document.querySelector('[data-sonner-toast]')).toBeNull();
		vi.restoreAllMocks();
	});
});
