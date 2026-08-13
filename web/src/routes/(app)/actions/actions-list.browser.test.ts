// Conversion contract for the actions list page.
//
// The page hands sorting, filtering and paging to the server through one
// scoped Search RPC, and every one of those knobs round-trips in the URL. These
// tests pin what a deep link is worth: the sort key a URL names must reach the
// server as the right SortField, the type / compliance / unassigned filters must
// arrive as the same tag filters the hand-rolled page sent, and the rendered row
// set must be exactly what the server returned (no client-side re-filtering
// silently narrowing a paginated page).
//
// The list renders in the shared row grammar (RowList), so two of its assertions
// are about the surface itself: the row must carry the detail link, and no
// <table> may come back — a table here is the regression the redesign removes.

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page as browser } from 'vitest/browser';
import { create } from '@bufbuild/protobuf';
import { SearchResultSchema } from '$sdk/powermanage/v1/control_pb';
import { SearchScope, SortField, SortDirection } from '$sdk/powermanage/v1/common_pb';
import { ActionType } from '$sdk/powermanage/v1/actions_pb';
import * as m from '$lib/paraglide/messages';

const SHELL_ID = '01JQZZ4A7K3M9P2Q6R8T1V0W5X';
const COMPLIANCE_ID = '01JQZZ5B8N4P0R3S7T9V2W1X6Y';
const PACKAGE_ID = '01JQZZ6C9P5Q1S4T8V0W3X2Y7Z';

const api = vi.hoisted(() => ({ search: vi.fn(), deleteAction: vi.fn() }));
// Mutable so each test can mount the page "at" a different deep link.
const nav = vi.hoisted(() => ({ url: new URL('https://control.test/actions') }));

// Only the client is faked; the generated protobuf re-exports stay real so the
// page's SearchScope / SortField / ActionType constants are the production ones.
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

// URL writes go through SvelteKit's shallow-routing API, which needs a live
// router; the page's history behaviour is url-state's contract, not this test's.
vi.mock('$app/navigation', () => ({
	pushState: vi.fn(),
	replaceState: vi.fn(),
	goto: vi.fn(),
	afterNavigate: vi.fn(),
	beforeNavigate: vi.fn()
}));

import ActionsPage from './+page.svelte';

const results = [
	create(SearchResultSchema, {
		id: SHELL_ID,
		name: 'Rotate log files',
		description: 'Nightly cleanup',
		fields: { name: 'Rotate log files', type: String(ActionType.SHELL), created_at: '1750000000' }
	}),
	create(SearchResultSchema, {
		id: COMPLIANCE_ID,
		name: 'Check disk encryption',
		description: 'LUKS present',
		fields: {
			name: 'Check disk encryption',
			type: String(ActionType.SHELL),
			is_compliance: 'true',
			created_at: '1750000100'
		}
	}),
	create(SearchResultSchema, {
		id: PACKAGE_ID,
		name: 'Install Firefox',
		description: '',
		fields: { name: 'Install Firefox', type: String(ActionType.PACKAGE), created_at: '1750000200' }
	})
];

beforeEach(() => {
	// The overview is the landing level now; these tests pin the LIST level, one
	// zoom in, so they address it explicitly the way a list deep link does.
	nav.url = new URL('https://control.test/actions?zoom=list');
	api.search.mockReset();
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

/** Sort controls live in the row list's sort bar; addressing them by text alone
 *  would also reach the row overflow trigger and the date-range placeholders. */
function clickSort(label: string) {
	const button = [
		...document.querySelectorAll<HTMLButtonElement>('[data-testid="row-list-sort"] button')
	].find((b) => b.textContent?.trim().startsWith(label));
	if (!button) throw new Error(`no sort control named ${label}`);
	button.click();
}

async function mountAt(query: string) {
	// Every mount in this file addresses the list level explicitly.
	nav.url = new URL(`https://control.test/actions${query}${query ? '&' : '?'}zoom=list`);
	// The search is debounced; wait for the request THIS mount produced, not for
	// one a previous mount left in flight.
	const before = api.search.mock.calls.length;
	render(ActionsPage);
	await vi.waitFor(() => expect(api.search.mock.calls.length).toBeGreaterThan(before), {
		timeout: 3000
	});
}

describe('actions list — sort keys reach the server as SortFields', () => {
	it('searches the actions scope newest-first by default', async () => {
		await mountAt('');

		const args = lastSearchArgs();
		expect(args.scope).toBe(SearchScope.ACTIONS);
		expect(args.sortField).toBe(SortField.CREATED_AT);
		expect(args.sortDirection).toBe(SortDirection.DESC);
		expect(args.query).toBe('');
		expect(args.tagFilters).toBeUndefined();
		expect(args.dateFilters).toBeUndefined();
	});

	it.each([
		['name', SortField.NAME],
		['type', SortField.TYPE],
		['created', SortField.CREATED_AT],
		['updated', SortField.UPDATED_AT]
	])('maps the ?sort=%s deep link to its SortField', async (sortKey, expected) => {
		await mountAt(`?sort=${sortKey}&sortDir=desc`);

		expect(lastSearchArgs().sortField).toBe(expected);
		expect(lastSearchArgs().sortDirection).toBe(SortDirection.DESC);
	});

	it('falls back to the default sort for an unknown sort key', async () => {
		await mountAt('?sort=not-a-column');

		expect(lastSearchArgs().sortField).toBe(SortField.CREATED_AT);
	});

	it('switches to the name key ascending when its sort control is clicked', async () => {
		await mountAt('');
		await expect.element(browser.getByText('Install Firefox')).toBeVisible();

		clickSort(m.actions_table_name());

		await vi.waitFor(
			() => {
				expect(lastSearchArgs().sortField).toBe(SortField.NAME);
				expect(lastSearchArgs().sortDirection).toBe(SortDirection.ASC);
			},
			{ timeout: 3000 }
		);
	});
});

/** Local midnight of a Y-M-D, as the epoch seconds the server compares against. */
function localMidnight(year: number, month: number, day: number): bigint {
	return BigInt(Math.floor(new Date(year, month - 1, day, 0, 0, 0, 0).getTime() / 1000));
}

describe('actions list — date ranges reach the server as date filters', () => {
	it('sends a created_at range from local midnight to the end of the last day', async () => {
		await mountAt('?createdStart=2026-01-05&createdEnd=2026-01-31');

		expect(lastSearchArgs().dateFilters).toEqual([
			{
				field: 'created_at',
				start: localMidnight(2026, 1, 5),
				// Inclusive end of 31 Jan == local midnight of 1 Feb.
				end: localMidnight(2026, 2, 1)
			}
		]);
	});

	it('leaves the end open when only a start bound is picked', async () => {
		await mountAt('?updatedStart=2026-03-02');

		expect(lastSearchArgs().dateFilters).toEqual([
			{ field: 'updated_at', start: localMidnight(2026, 3, 2), end: 0n }
		]);
	});

	it('leaves the start open when only an end bound is picked', async () => {
		await mountAt('?updatedEnd=2026-03-02');

		expect(lastSearchArgs().dateFilters).toEqual([
			{ field: 'updated_at', start: 0n, end: localMidnight(2026, 3, 3) }
		]);
	});

	it('sends both ranges when created and updated are filtered together', async () => {
		await mountAt('?createdStart=2026-01-05&updatedEnd=2026-02-10');

		expect(lastSearchArgs().dateFilters).toEqual([
			{ field: 'created_at', start: localMidnight(2026, 1, 5), end: 0n },
			{ field: 'updated_at', start: 0n, end: localMidnight(2026, 2, 11) }
		]);
	});

	it('ignores a malformed date param instead of breaking the search', async () => {
		await mountAt('?createdStart=not-a-date');

		expect(lastSearchArgs().dateFilters).toBeUndefined();
		expect(lastSearchArgs().scope).toBe(SearchScope.ACTIONS);
	});
});

describe('actions list — filters reach the server as tag filters', () => {
	it('sends one pipe-joined type tag for a multi-type selection', async () => {
		await mountAt('?types=shell,package');

		expect(lastSearchArgs().tagFilters).toEqual({
			type: `${ActionType.SHELL}|${ActionType.PACKAGE}`
		});
	});

	it('sends the compliance flag on its own field, ANDed with a concrete type', async () => {
		await mountAt('?types=shell,compliance');

		expect(lastSearchArgs().tagFilters).toEqual({
			type: String(ActionType.SHELL),
			is_compliance: 'true'
		});
	});

	it('keeps the "not in action set" filter as an assigned=false tag', async () => {
		await mountAt('?unassigned=1');

		expect(lastSearchArgs().tagFilters).toEqual({ assigned: 'false' });
	});

	it('combines every filter and the search term in one request', async () => {
		await mountAt('?query=firefox&types=package,compliance&unassigned=1&page=2&pageSize=10');

		const args = lastSearchArgs();
		expect(args.query).toBe('firefox');
		expect(args.tagFilters).toEqual({
			type: String(ActionType.PACKAGE),
			is_compliance: 'true',
			assigned: 'false'
		});
		expect(args.pageSize).toBe(10);
		expect(args.pageToken).toBe('10'); // offset = (2 - 1) * 10
	});

	it('drops an unknown type slug instead of sending a bogus tag', async () => {
		await mountAt('?types=not-a-type');

		expect(lastSearchArgs().tagFilters).toBeUndefined();
	});
});

describe('actions list — the rendered rows are the server page', () => {
	it('renders every returned action with its ULID and type badge', async () => {
		await mountAt('?types=shell,compliance&unassigned=1');

		for (const name of ['Rotate log files', 'Check disk encryption', 'Install Firefox']) {
			await expect.element(browser.getByText(name)).toBeVisible();
		}
		for (const id of [SHELL_ID, COMPLIANCE_ID, PACKAGE_ID]) {
			await expect.element(browser.getByText(id)).toBeVisible();
		}

		// A SHELL action flagged is_compliance reads as a compliance check, not a
		// shell script — the distinction the list page has always drawn.
		for (const label of [
			m.actions_type_compliance_check(),
			m.actions_type_shell(),
			m.actions_type_package()
		]) {
			await expect.element(browser.getByText(label, { exact: true })).toBeVisible();
		}
	});

	it('makes each row a link to its action detail', async () => {
		await mountAt('');
		await expect.element(browser.getByText('Rotate log files')).toBeVisible();

		const links = [
			...document.querySelectorAll<HTMLAnchorElement>('[data-testid="row-list-link"]')
		].map((a) => a.getAttribute('href'));
		expect(links).toEqual([
			`/actions/${SHELL_ID}`,
			`/actions/${COMPLIANCE_ID}`,
			`/actions/${PACKAGE_ID}`
		]);
	});

	it('renders the list in the row grammar — never a table', async () => {
		await mountAt('');
		await expect.element(browser.getByText('Rotate log files')).toBeVisible();

		expect(document.querySelector('[data-testid="row-list"]')).not.toBeNull();
		expect(document.querySelectorAll('table').length).toBe(0);
	});

	it('distinguishes an empty result set from a filtered one', async () => {
		api.search.mockResolvedValue({ results: [], totalCount: 0 });
		await mountAt('');

		await expect.element(browser.getByText(m.actions_empty())).toBeVisible();
		await expect.element(browser.getByText(m.actions_empty_hint())).toBeVisible();
	});

	it('offers a different search term when a filter is what emptied the page', async () => {
		api.search.mockResolvedValue({ results: [], totalCount: 0 });
		await mountAt('?unassigned=1');

		await expect.element(browser.getByText(m.common_try_different_search())).toBeVisible();
	});
});
