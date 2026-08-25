// Conversion guard for the two container list pages (action-sets, definitions)
// after they moved onto createSearchListState + the shared RowList grammar. Both
// are thin wrappers over one scoped Search RPC, so what matters is that the URL a
// user deep-links reaches the server unchanged — scope, sort field, direction,
// offset, and the "empty container" tag filter — that the member-management entry
// point the inline expansion used to provide still works from the row, and that
// the surface stays row-shaped: a detail link per row and no <table> anywhere.
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page as browser } from 'vitest/browser';
import { create } from '@bufbuild/protobuf';
import { SearchResultSchema, type SearchResult } from '$contract/cadestro/v1/control_pb';
import { SearchScope, SortField, SortDirection } from '$contract/cadestro/v1/common_pb';
import * as m from '$lib/paraglide/messages';

const SET_ID = '01JQZZ7D0Q6R2T5V9W1X4Y3Z8A';
const DEF_ID = '01JQZZ8E1R7S3V6W0X2Y5Z4A9B';
const ACTION_ID = '01JQZZ9F2S8T4W7X1Y3Z6A5B0C';

const api = vi.hoisted(() => ({
	search: vi.fn(),
	deleteActionSet: vi.fn(),
	deleteDefinition: vi.fn(),
	getActionSet: vi.fn(),
	listActions: vi.fn(),
	addActionToSet: vi.fn(),
	getDefinition: vi.fn(),
	listActionSets: vi.fn(),
	addActionSetToDefinition: vi.fn(),
	createDefinition: vi.fn()
}));
const nav = vi.hoisted(() => ({ url: new URL('https://control.test/action-sets') }));

// Only the client is faked; the generated protobuf re-exports stay real so the
// pages' SearchScope / SortField constants are the production ones.
vi.mock('$lib/sdk', async () => {
	const common = await import('$contract/cadestro/v1/common_pb');
	const control = await import('$contract/cadestro/v1/control_pb');
	const actions = await import('$contract/cadestro/v1/actions_pb');
	return {
		...actions,
		...control,
		...common,
		apiClient: api,
		formatTimestamp: () => '—',
		formatTimestampDateTime: () => '2026-08-01 09:00',
		fetchAllPages: vi.fn()
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

vi.mock('$app/navigation', () => ({
	goto: vi.fn(),
	pushState: vi.fn(),
	replaceState: vi.fn(),
	afterNavigate: vi.fn(),
	beforeNavigate: vi.fn()
}));

import ActionSetsPage from './+page.svelte';
import DefinitionsPage from '../definitions/+page.svelte';

/** Positional apiClient.search args — see data-table/list-logic.ts SearchArgs. */
const ARG = {
	query: 0,
	scope: 1,
	pageSize: 2,
	pageToken: 3,
	dates: 4,
	tags: 5,
	sortField: 6,
	sortDir: 7
} as const;

/** Local midnight of a Y-M-D, as the epoch seconds the server compares against. */
function localMidnight(year: number, month: number, day: number): bigint {
	return BigInt(Math.floor(new Date(year, month - 1, day, 0, 0, 0, 0).getTime() / 1000));
}

function lastCall() {
	const call = api.search.mock.calls.at(-1);
	if (!call) throw new Error('the page never issued a search');
	return call;
}

function containerResult(id: string, name: string, memberCount: string): SearchResult {
	return create(SearchResultSchema, {
		id,
		name,
		description: 'Baseline',
		fields: { name, member_count: memberCount, created_at: '1750000000', updated_at: '1750000900' }
	});
}

/** Sort controls live in the row list's sort bar; addressing them by text alone
 *  would also match the row overflow trigger, which is labelled "Actions" too. */
async function clickSort(label: string) {
	const button = [
		...document.querySelectorAll<HTMLButtonElement>('[data-testid="row-list-sort"] button')
	].find((b) => b.textContent?.trim().startsWith(label));
	if (!button) throw new Error(`no sort control named ${label}`);
	button.click();
}

async function mountAt(Component: typeof ActionSetsPage, url: string, results: SearchResult[]) {
	// The overview is the landing level now; these tests pin the LIST level, one
	// zoom in, addressed explicitly the way a list deep link is.
	const target = new URL(url);
	target.searchParams.set('zoom', 'list');
	nav.url = target;
	api.search.mockResolvedValue({ results, totalCount: results.length });
	// Count first: the assertion must wait for THIS mount's request, not for one
	// a previous mount left in flight.
	const before = api.search.mock.calls.length;
	render(Component);
	await vi.waitFor(() => expect(api.search.mock.calls.length).toBeGreaterThan(before), {
		timeout: 3000
	});
}

beforeEach(() => {
	vi.clearAllMocks();
});

describe('action-sets list page', () => {
	const SETS = [containerResult(SET_ID, 'Base System Setup', '2')];

	it('queries the ACTION_SETS scope newest-first by default', async () => {
		await mountAt(ActionSetsPage, 'https://control.test/action-sets', SETS);

		const call = lastCall();
		expect(call[ARG.scope]).toBe(SearchScope.ACTION_SETS);
		expect(call[ARG.sortField]).toBe(SortField.CREATED_AT);
		expect(call[ARG.sortDir]).toBe(SortDirection.DESC);
		expect(call[ARG.tags]).toBeUndefined();
		expect(call[ARG.dates]).toBeUndefined();
	});

	it('maps the member-count column to SortField.MEMBER_COUNT', async () => {
		await mountAt(ActionSetsPage, 'https://control.test/action-sets', SETS);
		const before = api.search.mock.calls.length;

		await clickSort(m.action_sets_table_actions());

		await vi.waitFor(() => expect(api.search.mock.calls.length).toBeGreaterThan(before));
		expect(lastCall()[ARG.sortField]).toBe(SortField.MEMBER_COUNT);
		expect(lastCall()[ARG.sortDir]).toBe(SortDirection.ASC);
	});

	it('switches to another timestamp column newest-first', async () => {
		await mountAt(ActionSetsPage, 'https://control.test/action-sets', SETS);
		const before = api.search.mock.calls.length;

		await clickSort(m.actions_table_updated());

		await vi.waitFor(() => expect(api.search.mock.calls.length).toBeGreaterThan(before));
		expect(lastCall()[ARG.sortField]).toBe(SortField.UPDATED_AT);
		expect(lastCall()[ARG.sortDir]).toBe(SortDirection.DESC);
	});

	it('round-trips a deep-linked sort / direction / page / page-size', async () => {
		await mountAt(
			ActionSetsPage,
			'https://control.test/action-sets?sort=name&sortDir=asc&page=3&pageSize=10',
			SETS
		);

		const call = lastCall();
		expect(call[ARG.sortField]).toBe(SortField.NAME);
		expect(call[ARG.sortDir]).toBe(SortDirection.ASC);
		expect(call[ARG.pageSize]).toBe(10);
		expect(call[ARG.pageToken]).toBe('20');
	});

	it('keeps "no assigned actions" as a server-side member_count=0 filter', async () => {
		await mountAt(ActionSetsPage, 'https://control.test/action-sets?noActions=1', SETS);

		expect(lastCall()[ARG.tags]).toEqual({ member_count: '0' });
	});

	it('keeps the created / updated ranges as server-side date filters', async () => {
		await mountAt(
			ActionSetsPage,
			'https://control.test/action-sets?createdStart=2026-01-05&createdEnd=2026-01-31&updatedStart=2026-02-01',
			SETS
		);

		expect(lastCall()[ARG.dates]).toEqual([
			// Inclusive end of 31 Jan == local midnight of 1 Feb.
			{ field: 'created_at', start: localMidnight(2026, 1, 5), end: localMidnight(2026, 2, 1) },
			{ field: 'updated_at', start: localMidnight(2026, 2, 1), end: 0n }
		]);
	});

	it('renders the set with its ULID and member count', async () => {
		await mountAt(ActionSetsPage, 'https://control.test/action-sets', SETS);

		await expect.element(browser.getByText('Base System Setup')).toBeVisible();
		await expect.element(browser.getByText(SET_ID)).toBeVisible();
		await expect.element(browser.getByText(m.action_sets_count({ count: 2 }))).toBeVisible();
	});

	it('makes each row a link to its set detail, in the row grammar', async () => {
		await mountAt(ActionSetsPage, 'https://control.test/action-sets', SETS);
		await expect.element(browser.getByText('Base System Setup')).toBeVisible();

		const link = document.querySelector<HTMLAnchorElement>('[data-testid="row-list-link"]');
		expect(link?.getAttribute('href')).toBe(`/action-sets/${SET_ID}`);
		expect(document.querySelectorAll('table').length).toBe(0);
	});

	it('still opens member management from the row, offering only non-members', async () => {
		api.getActionSet.mockResolvedValue({
			set: { id: SET_ID, name: 'Base System Setup' },
			members: [{ actionId: { value: ACTION_ID }, sortOrder: 0, actionName: 'Install Firefox', actionType: 1 }]
		});
		api.listActions.mockResolvedValue({
			actions: [
				{ id: ACTION_ID, name: 'Install Firefox', type: 1 },
				{ id: '01JQZZAG3T9V5X8Y2Z4A7B6C1D', name: 'Rotate logs', type: 6 }
			]
		});
		await mountAt(ActionSetsPage, 'https://control.test/action-sets', SETS);

		const trigger = document.querySelector<HTMLButtonElement>(
			`button[aria-label="${m.common_actions()}"]`
		);
		if (!trigger) throw new Error('no row overflow trigger rendered');
		trigger.click();

		const item = await vi.waitFor(() => {
			const el = document.querySelector<HTMLElement>('[role="menuitem"]');
			if (!el) throw new Error('row menu did not open');
			return el;
		});
		expect(item.textContent).toContain(m.action_picker_title());
		item.click();

		await vi.waitFor(() => expect(api.getActionSet).toHaveBeenCalledWith(SET_ID));
		expect(api.listActions).toHaveBeenCalled();
		// The set's existing member must not be offered again.
		await expect.element(browser.getByText('Rotate logs')).toBeVisible();
		expect(document.body.textContent).not.toContain('Install Firefox');
	});
});

describe('definitions list page', () => {
	const DEFS = [containerResult(DEF_ID, 'Workstation Setup', '3')];

	it('queries the DEFINITIONS scope newest-first by default', async () => {
		await mountAt(DefinitionsPage, 'https://control.test/definitions', DEFS);

		const call = lastCall();
		expect(call[ARG.scope]).toBe(SearchScope.DEFINITIONS);
		expect(call[ARG.sortField]).toBe(SortField.CREATED_AT);
		expect(call[ARG.sortDir]).toBe(SortDirection.DESC);
	});

	it('maps the action-set-count column to SortField.MEMBER_COUNT', async () => {
		await mountAt(DefinitionsPage, 'https://control.test/definitions', DEFS);
		const before = api.search.mock.calls.length;

		await clickSort(m.definitions_table_sets());

		await vi.waitFor(() => expect(api.search.mock.calls.length).toBeGreaterThan(before));
		expect(lastCall()[ARG.sortField]).toBe(SortField.MEMBER_COUNT);
	});

	it('keeps "no assigned action sets" as a server-side member_count=0 filter', async () => {
		await mountAt(DefinitionsPage, 'https://control.test/definitions?noSets=1', DEFS);

		expect(lastCall()[ARG.tags]).toEqual({ member_count: '0' });
	});

	it('round-trips a deep-linked name sort with its offset', async () => {
		await mountAt(
			DefinitionsPage,
			'https://control.test/definitions?sort=name&sortDir=asc&page=2&pageSize=25',
			DEFS
		);

		const call = lastCall();
		expect(call[ARG.sortField]).toBe(SortField.NAME);
		expect(call[ARG.sortDir]).toBe(SortDirection.ASC);
		expect(call[ARG.pageToken]).toBe('25');
	});

	it('keeps the created range as a server-side date filter', async () => {
		await mountAt(
			DefinitionsPage,
			'https://control.test/definitions?createdStart=2026-04-01&createdEnd=2026-04-30',
			DEFS
		);

		expect(lastCall()[ARG.dates]).toEqual([
			{ field: 'created_at', start: localMidnight(2026, 4, 1), end: localMidnight(2026, 5, 1) }
		]);
	});

	it('renders the definition with its ULID and action-set count', async () => {
		await mountAt(DefinitionsPage, 'https://control.test/definitions', DEFS);

		await expect.element(browser.getByText('Workstation Setup')).toBeVisible();
		await expect.element(browser.getByText(DEF_ID)).toBeVisible();
		await expect.element(browser.getByText(m.definitions_count({ count: 3 }))).toBeVisible();
	});

	it('makes each row a link to its definition detail, in the row grammar', async () => {
		await mountAt(DefinitionsPage, 'https://control.test/definitions', DEFS);
		await expect.element(browser.getByText('Workstation Setup')).toBeVisible();

		const link = document.querySelector<HTMLAnchorElement>('[data-testid="row-list-link"]');
		expect(link?.getAttribute('href')).toBe(`/definitions/${DEF_ID}`);
		expect(document.querySelectorAll('table').length).toBe(0);
	});

	it('offers the create hint when the catalogue itself is empty', async () => {
		await mountAt(DefinitionsPage, 'https://control.test/definitions', []);

		await expect.element(browser.getByText(m.definitions_empty_hint())).toBeVisible();
	});

	it('offers a different search term when a filter is what emptied the page', async () => {
		await mountAt(DefinitionsPage, 'https://control.test/definitions?noSets=1', []);

		await expect.element(browser.getByText(m.common_try_different_search())).toBeVisible();
	});
});
