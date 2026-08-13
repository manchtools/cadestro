// The run-script dialog is a clipped box (`max-h-[90vh] overflow-hidden`) whose
// script list is the only part that may grow. These tests pin that the growth
// has exactly ONE place to go: the dialog's own scroll region. A second scroll
// area nested inside it traps the wheel and leaves rows the operator can reach
// only by scrolling two containers in the right order — which is how the last
// script in a long library became unreachable.
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { create } from '@bufbuild/protobuf';
import { ManagedActionSchema } from '$sdk/powermanage/v1/control_pb';
import { ActionType } from '$sdk/powermanage/v1/actions_pb';

const api = vi.hoisted(() => ({
	listActions: vi.fn(),
	dispatchAction: vi.fn()
}));

// Only the client is faked; the generated protobuf re-exports stay real, and
// `fetchAllPages` keeps its real paging contract so the dialog sweeps as it does
// in production.
vi.mock('$lib/sdk', async () => {
	const control = await import('$sdk/powermanage/v1/control_pb');
	return {
		...control,
		apiClient: api,
		fetchAllPages: async <T>(
			fetchPage: (size: number, token: string) => Promise<{ items: T[]; nextPageToken: string }>
		) => {
			const out: T[] = [];
			let token = '';
			for (;;) {
				const page = await fetchPage(100, token);
				out.push(...page.items);
				if (!page.nextPageToken) break;
				token = page.nextPageToken;
			}
			return out;
		},
		formatTimestamp: () => '—',
		formatTimestampDateTime: () => '2026-08-01 09:00'
	};
});

import RunScriptDialog from './run-script-dialog.svelte';

const DEVICE_ID = '01JQZZDEVICE00000000000000';

// More scripts than any single viewport can show — the condition the dialog has
// to survive, not a convenient handful.
const SCRIPTS = Array.from({ length: 40 }, (_, i) =>
	create(ManagedActionSchema, {
		id: `01JQZZSCRIPT${String(i).padStart(14, '0')}`,
		name: `Rotate logs on tier ${i}`,
		description: `Trims and ships the tier ${i} journal before it fills the disk`,
		type: ActionType.SHELL
	})
);

const region = () => document.querySelector<HTMLElement>('[data-testid="run-script-list"]')!;
const rows = () => Array.from(document.querySelectorAll<HTMLElement>('tbody tr'));

/** Every element under `root` that really scrolls vertically. */
function verticalScrollers(root: HTMLElement): HTMLElement[] {
	return Array.from(root.querySelectorAll<HTMLElement>('*')).filter((el) => {
		const overflow = getComputedStyle(el).overflowY;
		return (overflow === 'auto' || overflow === 'scroll') && el.scrollHeight > el.clientHeight;
	});
}

beforeEach(() => {
	document.body.innerHTML = '';
	api.listActions.mockReset();
	api.listActions.mockResolvedValue({
		actions: SCRIPTS,
		nextPageToken: '',
		totalCount: SCRIPTS.length
	});
	api.dispatchAction.mockReset();
	api.dispatchAction.mockResolvedValue({});
});

describe('the script list has exactly one scroll region', () => {
	it('scrolls the dialog’s own region and nests no second scroller inside it', async () => {
		render(RunScriptDialog, { open: true, deviceId: DEVICE_ID });
		await vi.waitFor(() => expect(rows().length).toBe(SCRIPTS.length), { timeout: 4000 });

		// the premise: forty scripts genuinely overflow the space the dialog gives
		// the list, so the region is doing real work
		expect(region().scrollHeight).toBeGreaterThan(region().clientHeight);

		// …and nothing inside it scrolls on the same axis
		expect(verticalScrollers(region())).toEqual([]);
	});

	it('brings the last script into view by scrolling that one region', async () => {
		render(RunScriptDialog, { open: true, deviceId: DEVICE_ID });
		await vi.waitFor(() => expect(rows().length).toBe(SCRIPTS.length), { timeout: 4000 });

		const last = rows().at(-1)!;
		expect(last.textContent).toContain(`Rotate logs on tier ${SCRIPTS.length - 1}`);

		region().scrollTop = region().scrollHeight;

		await vi.waitFor(() => {
			const box = region().getBoundingClientRect();
			const r = last.getBoundingClientRect();
			expect(r.bottom).toBeLessThanOrEqual(box.bottom + 1);
			expect(r.top).toBeGreaterThanOrEqual(box.top - 1);
		});

		// and it is a real target once there, not just geometry
		last.click();
		await vi.waitFor(() => expect(last.getAttribute('data-state')).toBe('selected'));
	});

	it('keeps the schedule box and the footer on screen while the list scrolls', async () => {
		render(RunScriptDialog, { open: true, deviceId: DEVICE_ID });
		await vi.waitFor(() => expect(rows().length).toBe(SCRIPTS.length), { timeout: 4000 });

		const content = document.querySelector<HTMLElement>('[data-slot="dialog-content"]')!;
		const box = content.getBoundingClientRect();
		const footerButtons = Array.from(
			document.querySelectorAll<HTMLElement>('[data-slot="dialog-footer"] button')
		);

		expect(footerButtons.length).toBe(2);
		for (const button of footerButtons) {
			const r = button.getBoundingClientRect();
			expect(r.bottom).toBeLessThanOrEqual(box.bottom + 1);
			expect(r.top).toBeGreaterThanOrEqual(box.top - 1);
		}
		// the dialog itself never becomes the scroller — it clips
		expect(content.scrollHeight).toBeLessThanOrEqual(content.clientHeight + 1);
	});
});
