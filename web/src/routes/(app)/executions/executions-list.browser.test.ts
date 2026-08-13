// Conversion contract for the executions list page.
//
// The page runs on the shared createSearchListState + DataTable machinery. Two
// of its needs are met by options rather than by page-local state:
//
//   * the created_at range travels through the `dateFilters` option into
//     SearchRequest.date_filters — the server's only interval channel, since tag
//     filters are matched by exact value;
//   * `defaultSortDir: 'desc'` keeps a bare /executions link newest-first, which
//     is what this page has always meant by "no ?sortDir".
//
// These tests are the wire contract: every filter and sort knob a deep link
// names must reach the server as the same Search RPC argument it did before the
// page was converted, the date range must still be transmitted, and the device
// column must still resolve its hostname from the raw search document
// (`device_hostname` is indexed for search but absent from the ActionExecution
// proto, so the row adapter carries it). They also cover the row-level cancel
// mutation.

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page as browser } from 'vitest/browser';
import { create } from '@bufbuild/protobuf';
import { SearchResultSchema, ActionExecutionSchema } from '$sdk/powermanage/v1/control_pb';
import {
	ExecutionStatus,
	SearchScope,
	SortField,
	SortDirection
} from '$sdk/powermanage/v1/common_pb';
import { ActionType } from '$sdk/powermanage/v1/actions_pb';
import * as m from '$lib/paraglide/messages';

const EXEC_A = '01JQZZ4A7K3M9P2Q6R8T1V0W5X';
const EXEC_B = '01JQZZ5B8N4P0R3S7T9V2W1X6Y';
const EXEC_C = '01JQZZ8E1R7S3V6W0X2Y5Z4A9B';
const ACTION_ID = '01JQZZ6C9P5Q1S4T8V0W3X2Y7Z';
// Sorts after ACTION_ID, so a same-second tie between the two operation cards
// resolves deterministically.
const OTHER_ACTION_ID = '01JQZZ9F2S8T4W7X1Y3Z6A5B0C';
const DEVICE_ID = '01JQZZ7D0Q6R2T5V9W1X4Y3Z8A';
const DEVICE_B = '01JR0000000000000000000002';
const DEVICE_C = '01JR0000000000000000000003';

const api = vi.hoisted(() => ({
	search: vi.fn(),
	cancelExecution: vi.fn(),
	dispatchToMultiple: vi.fn()
}));
// Mutable so each test can mount the page "at" a different deep link.
const nav = vi.hoisted(() => ({ url: new URL('https://control.test/executions') }));

// Only the client is faked; the generated protobuf re-exports stay real so the
// page's SearchScope / SortField / ExecutionStatus constants are the production
// ones.
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
		formatDuration: () => '—',
		fetchAllPages: vi.fn(async () => [])
	};
});

vi.mock('$app/state', () => ({
	page: {
		get url() {
			return nav.url;
		}
	}
}));

vi.mock('$app/paths', () => ({ base: '', assets: '' }));

// URL writes go through SvelteKit's shallow-routing API, which needs a live
// router; the page's history behaviour is url-state's contract, not this test's.
vi.mock('$app/navigation', () => ({
	pushState: vi.fn(),
	replaceState: vi.fn(),
	goto: vi.fn(),
	afterNavigate: vi.fn(),
	beforeNavigate: vi.fn()
}));

import ExecutionsPage from './+page.svelte';
// The real shell store, not a mock: "the menu calls openPanel" is only worth
// asserting as the panel it actually produces.
import { shell, resetShell } from '$lib/shell/shell.svelte';

function searchResult(id: string, status: ExecutionStatus, hostname: string) {
	return create(SearchResultSchema, {
		id,
		name: 'Rotate log files',
		fields: {
			action_id: ACTION_ID,
			action_name: 'Rotate log files',
			action_type: String(ActionType.SHELL),
			device_id: DEVICE_ID,
			device_hostname: hostname,
			status: String(status),
			created_at: '1750000000'
		}
	});
}

const results = [
	searchResult(EXEC_A, ExecutionStatus.SUCCESS, 'workstation-07'),
	searchResult(EXEC_B, ExecutionStatus.FAILED, 'laptop-12')
];

beforeEach(() => {
	document.body.innerHTML = '';
	resetShell();
	nav.url = new URL('https://control.test/executions');
	api.search.mockReset();
	api.cancelExecution.mockReset();
	api.dispatchToMultiple.mockReset();
	api.dispatchToMultiple.mockResolvedValue([]);
	api.search.mockResolvedValue({ results, totalCount: results.length });
});

/** Positional apiClient.search args — see list-logic's SearchArgs tuple. */
function lastSearchArgs() {
	const call = api.search.mock.calls.at(-1);
	if (!call) throw new Error('the page never issued a search');
	const [query, scope, pageSize, pageToken, dateFilters, tagFilters, sortField, sortDirection] =
		call;
	return { query, scope, pageSize, pageToken, dateFilters, tagFilters, sortField, sortDirection };
}

async function mountAt(query: string) {
	nav.url = new URL(`https://control.test/executions${query}`);
	render(ExecutionsPage);
	// The search is debounced; wait for the request the mount produced.
	await vi.waitFor(() => expect(api.search).toHaveBeenCalled(), { timeout: 3000 });
}

describe('executions list — sort keys reach the server as SortFields', () => {
	it('searches the executions scope newest-first by default', async () => {
		await mountAt('');

		const args = lastSearchArgs();
		expect(args.scope).toBe(SearchScope.EXECUTIONS);
		expect(args.sortField).toBe(SortField.CREATED_AT);
		expect(args.sortDirection).toBe(SortDirection.DESC);
		expect(args.query).toBe('');
		expect(args.tagFilters).toBeUndefined();
		expect(args.dateFilters).toBeUndefined();
	});

	it.each([
		['device', SortField.DEVICE_HOSTNAME],
		['status', SortField.STATUS],
		['created', SortField.CREATED_AT]
	])('maps the ?sort=%s deep link to its SortField', async (sortKey, expected) => {
		await mountAt(`?sort=${sortKey}&sortDir=asc`);

		expect(lastSearchArgs().sortField).toBe(expected);
		expect(lastSearchArgs().sortDirection).toBe(SortDirection.ASC);
	});

	it('falls back to the default sort for an unknown sort key', async () => {
		await mountAt('?sort=not-a-column');

		expect(lastSearchArgs().sortField).toBe(SortField.CREATED_AT);
	});

	it('switches to a sortable column through its header button', async () => {
		await mountAt('');
		await expect.element(browser.getByText('workstation-07')).toBeVisible();

		await browser.getByRole('button', { name: m.executions_table_device() }).click();

		await vi.waitFor(
			() => {
				expect(lastSearchArgs().sortField).toBe(SortField.DEVICE_HOSTNAME);
				// A non-timestamp column reads ascending when you switch to it.
				expect(lastSearchArgs().sortDirection).toBe(SortDirection.ASC);
			},
			{ timeout: 3000 }
		);
	});
});

describe('executions list — filters reach the server as tag filters', () => {
	it('sends one pipe-joined status tag for a multi-status selection', async () => {
		await mountAt(`?status=${ExecutionStatus.SUCCESS},${ExecutionStatus.FAILED}`);

		expect(lastSearchArgs().tagFilters).toEqual({
			status: `${ExecutionStatus.SUCCESS}|${ExecutionStatus.FAILED}`
		});
	});

	it('sends the action-type filter on its own tag field', async () => {
		await mountAt(`?types=${ActionType.SHELL},${ActionType.PACKAGE}`);

		expect(lastSearchArgs().tagFilters).toEqual({
			action_type: `${ActionType.SHELL}|${ActionType.PACKAGE}`
		});
	});

	it('keeps the device pre-filter as a device_id tag', async () => {
		await mountAt(`?device=${DEVICE_ID}`);

		expect(lastSearchArgs().tagFilters).toEqual({ device_id: DEVICE_ID });
	});

	it('combines every filter, the search term and the page offset in one request', async () => {
		await mountAt(
			`?query=rotate&types=${ActionType.SHELL}&status=${ExecutionStatus.FAILED}&device=${DEVICE_ID}&page=3&pageSize=10`
		);

		const args = lastSearchArgs();
		expect(args.query).toBe('rotate');
		expect(args.tagFilters).toEqual({
			action_type: String(ActionType.SHELL),
			status: String(ExecutionStatus.FAILED),
			device_id: DEVICE_ID
		});
		expect(args.pageSize).toBe(10);
		expect(args.pageToken).toBe('20'); // offset = (3 - 1) * 10
	});
});

// The regression this page's deviation from createSearchListState exists to
// prevent: date_filters are a separate SearchRequest field with no tag-filter
// equivalent, so if the list ever moves onto buildSearchArgs unchanged, the
// created_at range silently stops being sent and these assertions fail.
describe('executions list — the created_at range still reaches the server', () => {
	it('sends a created_at DateRange spanning local midnight to the inclusive end of day', async () => {
		await mountAt('?createdStart=2026-07-01&createdEnd=2026-07-31');

		const filters = lastSearchArgs().dateFilters;
		expect(filters).toHaveLength(1);
		expect(filters[0].field).toBe('created_at');

		const start = new Date(Number(filters[0].start) * 1000);
		expect([start.getFullYear(), start.getMonth(), start.getDate(), start.getHours()]).toEqual([
			2026, 6, 1, 0
		]);
		// End bound is the selected day plus one, so the last day is included.
		expect(filters[0].end - filters[0].start).toBe(BigInt(31 * 86400));
	});

	it('sends an open-ended range when only one bound is picked', async () => {
		await mountAt('?createdStart=2026-07-01');

		const filters = lastSearchArgs().dateFilters;
		expect(filters).toHaveLength(1);
		expect(filters[0].field).toBe('created_at');
		expect(filters[0].end).toBe(0n);
		expect(filters[0].start).toBeGreaterThan(0n);
	});

	it('ignores an unparseable date instead of sending a bogus range', async () => {
		await mountAt('?createdStart=not-a-date');

		expect(lastSearchArgs().dateFilters).toBeUndefined();
	});
});

describe('executions list — the rendered rows are the server page', () => {
	it('renders the action, the execution ULID and the hostname from the raw search fields', async () => {
		await mountAt('');

		// device_hostname is not on the ActionExecution proto — it only survives
		// because the page reads the raw SearchResult.fields.
		await expect.element(browser.getByText('workstation-07')).toBeVisible();
		await expect.element(browser.getByText('laptop-12')).toBeVisible();
		for (const id of [EXEC_A, EXEC_B]) {
			await expect.element(browser.getByText(id)).toBeVisible();
		}
		await expect
			.element(browser.getByText(m.executions_status_success(), { exact: true }))
			.toBeVisible();
		await expect
			.element(browser.getByText(m.executions_status_failed(), { exact: true }))
			.toBeVisible();
	});

	it('falls back to a short device id when the index has no hostname', async () => {
		const bare = create(SearchResultSchema, {
			id: EXEC_A,
			fields: { device_id: DEVICE_ID, status: String(ExecutionStatus.PENDING) }
		});
		api.search.mockResolvedValue({ results: [bare], totalCount: 1 });
		await mountAt('');

		await expect.element(browser.getByText(DEVICE_ID.slice(0, 8) + '...')).toBeVisible();
	});

	it('distinguishes an empty result set from a filtered one', async () => {
		api.search.mockResolvedValue({ results: [], totalCount: 0 });
		await mountAt('');

		await expect.element(browser.getByText(m.executions_empty())).toBeVisible();
		await expect.element(browser.getByText(m.executions_empty_hint())).toBeVisible();
	});

	// The feed is one idiom end to end: loading, empty and populated states are
	// all the same plate. An empty feed dressed as a Card was the odd one out.
	it('renders the empty feed on the same plate the feed uses everywhere else', async () => {
		api.search.mockResolvedValue({ results: [], totalCount: 0 });
		await mountAt('');
		await expect.element(browser.getByText(m.executions_empty())).toBeVisible();

		const feed = document.querySelector('[data-testid="ops-feed"]');
		expect(feed).not.toBeNull();
		const plate = feed!.firstElementChild as HTMLElement;
		expect(plate.className).toContain('shadow-plate');
		expect(plate.className).toContain('border-hair');
		// bits-ui's Card carries data-slot="card"; the feed never renders one.
		expect(feed!.querySelector('[data-slot="card"]')).toBeNull();
	});

	it('offers a different search term when a filter is what emptied the page', async () => {
		api.search.mockResolvedValue({ results: [], totalCount: 0 });
		await mountAt(`?status=${ExecutionStatus.FAILED}`);

		await expect.element(browser.getByText(m.common_try_different_search())).toBeVisible();
	});
});

// The row's action trigger is icon-only; bits-ui marks it with aria-haspopup.
async function openRowMenu() {
	const trigger = document.querySelector('[aria-haspopup="menu"]');
	expect(trigger).not.toBeNull();
	await browser.elementLocator(trigger!).click();
}

describe('executions list — the scheduled-run cancel path', () => {
	it('cancels through a confirmation dialog and patches the row in place', async () => {
		api.search.mockResolvedValue({
			results: [searchResult(EXEC_A, ExecutionStatus.SCHEDULED, 'workstation-07')],
			totalCount: 1
		});
		api.cancelExecution.mockResolvedValue(
			create(ActionExecutionSchema, { id: EXEC_A, status: ExecutionStatus.CANCELLED })
		);
		await mountAt('');
		await expect
			.element(browser.getByText(m.execution_status_scheduled(), { exact: true }))
			.toBeVisible();

		await openRowMenu();
		await browser.getByRole('menuitem', { name: m.execution_cancel() }).click();
		// Destructive mutations confirm first — no cancel is issued yet.
		const dialog = browser.getByRole('alertdialog');
		await expect.element(dialog.getByText(m.execution_cancel_confirm())).toBeVisible();
		expect(api.cancelExecution).not.toHaveBeenCalled();

		await dialog.getByRole('button', { name: m.execution_cancel() }).click();

		await vi.waitFor(() => expect(api.cancelExecution).toHaveBeenCalledWith(EXEC_A), {
			timeout: 3000
		});
		await expect
			.element(browser.getByText(m.execution_status_cancelled(), { exact: true }))
			.toBeVisible();
	});

	it('opens a watch window on the effect row, not on the derived operation', async () => {
		api.search.mockResolvedValue({
			results: [searchResult(EXEC_A, ExecutionStatus.RUNNING, 'workstation-07')],
			totalCount: 1
		});
		await mountAt('');

		await openRowMenu();
		await browser.getByRole('menuitem', { name: m.executions_watch_window() }).click();

		// One execution, one panel: the refId is the effect's own id — an
		// operation is a client-side cluster and has no id to watch.
		await vi.waitFor(() => expect(shell.panels).toHaveLength(1), { timeout: 3000 });
		expect(shell.panels[0]).toMatchObject({ kind: 'execution', refId: EXEC_A });
		expect(shell.panels[0].title).toContain('workstation-07');
	});

	it('offers no cancel action for an execution that is not scheduled', async () => {
		api.search.mockResolvedValue({
			results: [searchResult(EXEC_A, ExecutionStatus.SUCCESS, 'workstation-07')],
			totalCount: 1
		});
		await mountAt('');

		await openRowMenu();
		await expect
			.element(browser.getByRole('menuitem', { name: m.executions_open_details() }))
			.toBeVisible();
		expect(browser.getByRole('menuitem', { name: m.execution_cancel() }).elements()).toHaveLength(
			0
		);
	});
});

// ── Movement F: the feed groups executions into operations ──────────────────
//
// ActionExecution carries no dispatch id (see ./operation-feed.ts), so the
// cards are clustered by action identity + dispatch window. These tests are the
// rendered half of that rule: what groups, what does not, what order effects
// read in, that the chips add up, and that "retry failed N" re-dispatches to
// exactly the failed devices and to nobody else.

type ResultSpec = {
	id: string;
	status: ExecutionStatus;
	hostname: string;
	deviceId?: string;
	actionId?: string;
	actionName?: string;
	createdAt?: string;
};

function execResult(spec: ResultSpec) {
	const actionName = spec.actionName ?? 'Rotate log files';
	return create(SearchResultSchema, {
		id: spec.id,
		name: actionName,
		fields: {
			action_id: spec.actionId ?? ACTION_ID,
			action_name: actionName,
			action_type: String(ActionType.SHELL),
			device_id: spec.deviceId ?? DEVICE_ID,
			device_hostname: spec.hostname,
			status: String(spec.status),
			created_at: spec.createdAt ?? '1750000000'
		}
	});
}

function cards(): HTMLElement[] {
	return [...document.querySelectorAll<HTMLElement>('[data-testid="operation-card"]')];
}

async function mountResults(results: ReturnType<typeof execResult>[]) {
	api.search.mockResolvedValue({ results, totalCount: results.length });
	await mountAt('');
	await vi.waitFor(() => expect(cards().length).toBeGreaterThan(0), { timeout: 3000 });
}

describe('executions feed — what groups into one operation', () => {
	it('groups one action dispatched to several devices into a single card', async () => {
		await mountResults([
			execResult({ id: EXEC_A, status: ExecutionStatus.SUCCESS, hostname: 'workstation-07' }),
			execResult({
				id: EXEC_B,
				status: ExecutionStatus.FAILED,
				hostname: 'laptop-12',
				deviceId: DEVICE_B
			})
		]);

		expect(cards()).toHaveLength(1);
		await expect
			.element(
				browser.getByText(
					m.executions_op_headline({ action: 'Rotate log files', count: 2 }),
					{ exact: true }
				)
			)
			.toBeVisible();
	});

	it('keeps a different action dispatched at the same instant in its own card', async () => {
		await mountResults([
			execResult({ id: EXEC_A, status: ExecutionStatus.SUCCESS, hostname: 'workstation-07' }),
			execResult({
				id: EXEC_C,
				status: ExecutionStatus.SUCCESS,
				hostname: 'kiosk-03',
				deviceId: DEVICE_C,
				actionId: OTHER_ACTION_ID,
				actionName: 'Harden SSH baseline'
			})
		]);

		expect(cards()).toHaveLength(2);
		await expect
			.element(
				browser.getByText(m.executions_op_headline({ action: 'Harden SSH baseline', count: 1 }), {
					exact: true
				})
			)
			.toBeVisible();
	});

	it('splits the same action into two cards once the dispatch gap exceeds the window', async () => {
		await mountResults([
			execResult({ id: EXEC_A, status: ExecutionStatus.SUCCESS, hostname: 'workstation-07' }),
			execResult({
				id: EXEC_B,
				status: ExecutionStatus.SUCCESS,
				hostname: 'laptop-12',
				deviceId: DEVICE_B,
				// The window is 5 s; a minute later is a second operator gesture.
				createdAt: '1750000060'
			})
		]);

		expect(cards()).toHaveLength(2);
	});
});

describe('executions feed — effect rows and counts', () => {
	it('puts the failed effects first inside a card', async () => {
		await mountResults([
			execResult({ id: EXEC_A, status: ExecutionStatus.SUCCESS, hostname: 'aaa-succeeded' }),
			execResult({
				id: EXEC_B,
				status: ExecutionStatus.FAILED,
				hostname: 'zzz-failed',
				deviceId: DEVICE_B
			})
		]);

		const devices = [
			...cards()[0].querySelectorAll<HTMLElement>('[data-testid="effect-device"]')
		].map((el) => el.textContent?.trim());
		// Alphabetically 'aaa-succeeded' comes first — order is by outcome, not name.
		expect(devices).toEqual(['zzz-failed', 'aaa-succeeded']);
	});

	it('summarises the card with one chip per status bucket, adding up to the effects', async () => {
		await mountResults([
			execResult({ id: EXEC_A, status: ExecutionStatus.SUCCESS, hostname: 'workstation-07' }),
			execResult({
				id: EXEC_B,
				status: ExecutionStatus.FAILED,
				hostname: 'laptop-12',
				deviceId: DEVICE_B
			}),
			execResult({
				id: EXEC_C,
				status: ExecutionStatus.PENDING,
				hostname: 'kiosk-03',
				deviceId: DEVICE_C
			})
		]);

		const counts = cards()[0].querySelector('[data-testid="operation-counts"]');
		expect(counts?.textContent).toContain(m.executions_op_count_failed({ count: 1 }));
		expect(counts?.textContent).toContain(m.executions_op_count_queued({ count: 1 }));
		expect(counts?.textContent).toContain(m.executions_op_count_ok({ count: 1 }));
		// Three effects, three chips — no status is counted twice or dropped.
		expect(counts?.querySelectorAll('[data-testid="fleet-chip"]')).toHaveLength(3);
		expect(
			cards()[0].querySelectorAll('[data-testid="operation-effects"] > li')
		).toHaveLength(3);
	});
});

describe('executions feed — retry re-dispatches exactly the failed subset', () => {
	async function mountPartialFailure() {
		await mountResults([
			execResult({ id: EXEC_A, status: ExecutionStatus.SUCCESS, hostname: 'aaa-succeeded' }),
			execResult({
				id: EXEC_B,
				status: ExecutionStatus.FAILED,
				hostname: 'mmm-failed',
				deviceId: DEVICE_B
			}),
			execResult({
				id: EXEC_C,
				status: ExecutionStatus.TIMEOUT,
				hostname: 'zzz-timed-out',
				deviceId: DEVICE_C
			})
		]);
	}

	it('confirms first, then sends only the failed device ids with the same action', async () => {
		await mountPartialFailure();

		const retry = cards()[0].querySelector<HTMLElement>('[data-testid="operation-retry"]');
		expect(retry?.textContent).toContain(m.executions_op_retry_failed({ count: 2 }));

		await browser.elementLocator(retry!).click();
		const dialog = browser.getByRole('alertdialog');
		await expect
			.element(dialog.getByText(m.executions_op_retry_confirm({ count: 2 })))
			.toBeVisible();
		// Destructive-ish mutations confirm first — nothing is dispatched yet.
		expect(api.dispatchToMultiple).not.toHaveBeenCalled();

		await dialog.getByRole('button', { name: m.executions_op_retry_action() }).click();

		await vi.waitFor(
			() =>
				expect(api.dispatchToMultiple).toHaveBeenCalledWith([DEVICE_B, DEVICE_C], ACTION_ID),
			{ timeout: 3000 }
		);
		// The device that succeeded is never in the request.
		expect(api.dispatchToMultiple.mock.calls[0][0]).not.toContain(DEVICE_ID);
	});

	it('offers no retry for an operation whose effects all succeeded', async () => {
		await mountResults([
			execResult({ id: EXEC_A, status: ExecutionStatus.SUCCESS, hostname: 'workstation-07' })
		]);

		expect(cards()[0].querySelector('[data-testid="operation-retry"]')).toBeNull();
	});

	it('explains why an inline action cannot be re-dispatched instead of offering a broken button', async () => {
		const inline = create(SearchResultSchema, {
			id: EXEC_A,
			fields: {
				action_id: '',
				action_name: '',
				action_type: String(ActionType.SHELL),
				device_id: DEVICE_ID,
				device_hostname: 'workstation-07',
				status: String(ExecutionStatus.FAILED),
				created_at: '1750000000'
			}
		});
		await mountResults([inline]);

		expect(cards()[0].querySelector('[data-testid="operation-retry"]')).toBeNull();
		await expect.element(browser.getByText(m.executions_op_retry_inline())).toBeVisible();
	});
});
