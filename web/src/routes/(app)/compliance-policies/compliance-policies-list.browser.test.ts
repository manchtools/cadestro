// Conversion contract for the compliance-policies list page.
//
// The page runs on the shared createSearchListState + RowList machinery. The
// created_at range travels through the `dateFilters` option into
// SearchRequest.date_filters (the server's only interval channel — tag filters
// are matched by exact value), and `defaultSortDir: 'desc'` keeps a bare
// /compliance-policies link newest-first.
//
// These tests pin the request each deep link produces, plus the two columns that
// only render because the row adapter reads the raw search document —
// searchResultToCompliancePolicy drops the indexed `rule_count` and `created_at`
// fields, so a row built from the typed policy alone would read "0 rules" with a
// blank creation date.

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page as browser } from 'vitest/browser';
import { create } from '@bufbuild/protobuf';
import { SearchResultSchema } from '$sdk/powermanage/v1/control_pb';
import { SearchScope, SortField, SortDirection } from '$sdk/powermanage/v1/common_pb';
import * as m from '$lib/paraglide/messages';

const POLICY_A = '01JQZZ4A7K3M9P2Q6R8T1V0W5X';
const POLICY_B = '01JQZZ5B8N4P0R3S7T9V2W1X6Y';
const CREATED_AT_SECONDS = 1750000000;

const api = vi.hoisted(() => ({
	search: vi.fn(),
	deleteCompliancePolicy: vi.fn(),
	getCompliancePolicy: vi.fn(),
	listActions: vi.fn()
}));
// Mutable so each test can mount the page "at" a different deep link.
const nav = vi.hoisted(() => ({ url: new URL('https://control.test/compliance-policies?zoom=list') }));

// Only the client is faked; the generated protobuf re-exports stay real so the
// page's SearchScope / SortField constants are the production ones.
vi.mock('$lib/sdk', async () => {
	const common = await import('$sdk/powermanage/v1/common_pb');
	const control = await import('$sdk/powermanage/v1/control_pb');
	const actions = await import('$sdk/powermanage/v1/actions_pb');
	return {
		...actions,
		...control,
		...common,
		apiClient: api,
		// The detail sheet reaches the action barrel (ActionCreateForm), which now
		// also carries the pipeline builders — they call useDraft, so the fake
		// module must export it or the whole graph fails to import.
		useDraft: <T>(_type: string, _id: string, initial: T) => ({
			data: { ...initial },
			update: () => {},
			clear: async () => {}
		}),
		formatTimestamp: () => '—',
		formatTimestampDateTime: () => '2026-08-01 09:00',
		formatDuration: () => '—',
		fetchAllPages: vi.fn(async () => [])
	};
});

// `state` is read by the detail sheet's shallow routing; the sheet stays closed
// while it is empty.
vi.mock('$app/state', () => ({
	page: {
		get url() {
			return nav.url;
		},
		state: {}
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

// Mocked above — imported so the row's shallow-routing call can be asserted.
import { pushState } from '$app/navigation';
import CompliancePoliciesPage from './+page.svelte';

const results = [
	create(SearchResultSchema, {
		id: POLICY_A,
		name: 'Security baseline',
		description: 'Disk encryption and screen lock',
		fields: {
			name: 'Security baseline',
			description: 'Disk encryption and screen lock',
			rule_count: '3',
			created_at: String(CREATED_AT_SECONDS)
		}
	}),
	create(SearchResultSchema, {
		id: POLICY_B,
		name: 'Empty policy',
		description: '',
		fields: { name: 'Empty policy', rule_count: '0', created_at: String(CREATED_AT_SECONDS) }
	})
];

beforeEach(() => {
	document.body.innerHTML = '';
	nav.url = new URL('https://control.test/compliance-policies?zoom=list');
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

async function mountAt(query: string) {
	// The overview is the landing level now; these tests pin the LIST level, one
	// zoom in, addressed explicitly the way a list deep link is.
	nav.url = new URL(
		`https://control.test/compliance-policies${query}${query ? '&' : '?'}zoom=list`
	);
	render(CompliancePoliciesPage);
	// The search is debounced; wait for the request the mount produced.
	await vi.waitFor(() => expect(api.search).toHaveBeenCalled(), { timeout: 3000 });
}

describe('compliance policies list — sort keys reach the server as SortFields', () => {
	it('searches the compliance-policies scope newest-first by default', async () => {
		await mountAt('');

		const args = lastSearchArgs();
		expect(args.scope).toBe(SearchScope.COMPLIANCE_POLICIES);
		expect(args.sortField).toBe(SortField.CREATED_AT);
		expect(args.sortDirection).toBe(SortDirection.DESC);
		expect(args.tagFilters).toBeUndefined();
		expect(args.dateFilters).toBeUndefined();
	});

	it.each([
		['name', SortField.NAME],
		['rules', SortField.RULE_COUNT],
		['created', SortField.CREATED_AT]
	])('maps the ?sort=%s deep link to its SortField', async (sortKey, expected) => {
		await mountAt(`?sort=${sortKey}&sortDir=asc`);

		expect(lastSearchArgs().sortField).toBe(expected);
		expect(lastSearchArgs().sortDirection).toBe(SortDirection.ASC);
	});
});

describe('compliance policies list — filters reach the server', () => {
	it('keeps "no rules assigned" as an exact rule_count=0 tag', async () => {
		await mountAt('?noRules=1');

		expect(lastSearchArgs().tagFilters).toEqual({ rule_count: '0' });
	});

	it('sends no tag filter when the checkbox is off', async () => {
		await mountAt('?query=baseline');

		expect(lastSearchArgs().query).toBe('baseline');
		expect(lastSearchArgs().tagFilters).toBeUndefined();
	});

	// The regression this page's deviation from createSearchListState exists to
	// prevent: date_filters are a separate SearchRequest field with no tag
	// equivalent, so moving onto buildSearchArgs unchanged would silently drop it.
	it('still sends the created_at range as a DateRange', async () => {
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
});

describe('compliance policies list — the rendered rows are the server page', () => {
	it('renders the rule count and creation date the typed policy does not carry', async () => {
		await mountAt('');

		await expect.element(browser.getByText('Security baseline')).toBeVisible();
		await expect.element(browser.getByText(POLICY_A)).toBeVisible();
		// searchResultToCompliancePolicy sets neither ruleCount nor createdAt, so
		// both cells can only be right if the page read the raw search fields.
		await expect
			.element(browser.getByText(m.compliance_policies_rule_count({ count: 3 })))
			.toBeVisible();
		await expect
			.element(
				browser.getByText(new Date(CREATED_AT_SECONDS * 1000).toLocaleDateString()).first()
			)
			.toBeVisible();
	});

	it('falls back to the empty-description placeholder', async () => {
		await mountAt('');

		await expect.element(browser.getByText(m.common_no_description())).toBeVisible();
	});

	it('renders the list in the row grammar — never a table', async () => {
		await mountAt('');
		await expect.element(browser.getByText('Security baseline')).toBeVisible();

		expect(document.querySelector('[data-testid="row-list"]')).not.toBeNull();
		expect(document.querySelectorAll('table').length).toBe(0);
	});

	// A policy has a Sheet, not a page, so the row must be a button — an anchor
	// would promise a URL, and a bare div would drop keyboard activation.
	it('makes each row a keyboard-operable button that opens the policy sheet', async () => {
		await mountAt('');
		await expect.element(browser.getByText('Security baseline')).toBeVisible();

		const buttons = [
			...document.querySelectorAll<HTMLElement>('[data-testid="compliance-policy-open"]')
		];
		expect(buttons).toHaveLength(results.length);
		for (const button of buttons) {
			expect(button.tagName).toBe('BUTTON');
			expect(button.getAttribute('type')).toBe('button');
		}
		expect(document.querySelectorAll('[data-testid="row-list-link"]').length).toBe(0);

		// The sheet opens through shallow routing, so the click's whole contract is
		// the pushState the detail sheet reads its policy id back out of.
		await browser.getByTestId('compliance-policy-open').first().click();
		await vi.waitFor(
			() =>
				expect(pushState).toHaveBeenCalledWith(`/compliance-policies/${POLICY_A}`, {
					compliancePolicySheet: POLICY_A
				}),
			{ timeout: 3000 }
		);
	});

	it('distinguishes an empty result set from a filtered one', async () => {
		api.search.mockResolvedValue({ results: [], totalCount: 0 });
		await mountAt('');

		await expect.element(browser.getByText(m.compliance_policies_empty())).toBeVisible();
		await expect.element(browser.getByText(m.compliance_policies_empty_hint())).toBeVisible();
	});

	it('offers a different search term when a filter is what emptied the page', async () => {
		api.search.mockResolvedValue({ results: [], totalCount: 0 });
		await mountAt('?noRules=1');

		await expect.element(browser.getByText(m.common_try_different_search())).toBeVisible();
	});
});
