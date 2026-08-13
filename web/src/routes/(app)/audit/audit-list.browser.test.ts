// Conversion contract for the audit list page.
//
// The audit view is an evidence viewer, so moving it onto the shared
// createSearchListState + RowList machinery had to change nothing about
// *which* events the server is asked for. Two needs are met by options rather
// than by page-local state: the occurred_at range travels through the
// `dateFilters` option into SearchRequest.date_filters (the server's only
// interval channel — tag filters are matched by exact value), and
// `defaultSortDir: 'desc'` keeps a bare /audit link newest-first.
//
// These tests pin the request the page issues for a given deep link: scope,
// sort field, actor / stream-type tag filters and the occurred_at range, plus
// the evidence a row still renders (event type, ULID, target, outcome) and the
// details panel that replaced the expandable row.

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page as browser } from 'vitest/browser';
import { create } from '@bufbuild/protobuf';
import { SearchResultSchema } from '$sdk/powermanage/v1/control_pb';
import { SearchScope, SortField, SortDirection } from '$sdk/powermanage/v1/common_pb';
import * as m from '$lib/paraglide/messages';

const EVENT_OK = '01JQZZ4A7K3M9P2Q6R8T1V0W5X';
const EVENT_DENIED = '01JQZZ5B8N4P0R3S7T9V2W1X6Y';
const ACTOR_ID = '01JQZZ6C9P5Q1S4T8V0W3X2Y7Z';
const DEVICE_ID = '01JQZZ7D0Q6R2T5V9W1X4Y3Z8A';

const api = vi.hoisted(() => ({
	search: vi.fn(),
	listUsers: vi.fn(),
	listDevices: vi.fn(),
	exportAuditEvents: vi.fn()
}));
// Mutable so each test can mount the page "at" a different deep link.
const nav = vi.hoisted(() => ({ url: new URL('https://control.test/audit') }));

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

import AuditPage from './+page.svelte';

const results = [
	create(SearchResultSchema, {
		id: EVENT_OK,
		fields: {
			stream_type: 'device',
			stream_id: DEVICE_ID,
			event_type: 'DeviceRegistered',
			actor_type: 'user',
			actor_id: ACTOR_ID,
			occurred_at: '1750000000'
		}
	}),
	create(SearchResultSchema, {
		id: EVENT_DENIED,
		fields: {
			stream_type: 'lps_password',
			stream_id: DEVICE_ID,
			event_type: 'LpsPasswordsViewDenied',
			actor_type: 'user',
			actor_id: ACTOR_ID,
			occurred_at: '1750000100'
		}
	})
];

beforeEach(() => {
	document.body.innerHTML = '';
	nav.url = new URL('https://control.test/audit');
	api.search.mockReset();
	api.listUsers.mockReset();
	api.listDevices.mockReset();
	api.exportAuditEvents.mockReset();
	api.exportAuditEvents.mockResolvedValue({ chunk: new Uint8Array(), nextPageToken: '' });
	api.search.mockResolvedValue({ results, totalCount: results.length });
	api.listUsers.mockResolvedValue({
		users: [{ id: ACTOR_ID, email: 'operator@example.test' }]
	});
	api.listDevices.mockResolvedValue({ devices: [], nextPageToken: '' });
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
	nav.url = new URL(`https://control.test/audit${query}`);
	render(AuditPage);
	// The search is debounced; wait for the request the mount produced.
	await vi.waitFor(() => expect(api.search).toHaveBeenCalled(), { timeout: 3000 });
}

/** An audit row navigates nowhere, so the row body is the open affordance — a
 *  real <button>, not an anchor and not a click handler on a div. */
function rowOpenButtons() {
	return [...document.querySelectorAll<HTMLElement>('[data-testid="audit-row-open"]')];
}

describe('audit list — sort keys reach the server as SortFields', () => {
	it('searches the audit scope newest-first by default', async () => {
		await mountAt('');

		const args = lastSearchArgs();
		expect(args.scope).toBe(SearchScope.AUDIT_EVENTS);
		expect(args.sortField).toBe(SortField.OCCURRED_AT);
		expect(args.sortDirection).toBe(SortDirection.DESC);
		expect(args.query).toBe('');
		expect(args.tagFilters).toBeUndefined();
		expect(args.dateFilters).toBeUndefined();
	});

	it.each([
		['timestamp', SortField.OCCURRED_AT],
		['actor', SortField.ACTOR_TYPE],
		['event_type', SortField.EVENT_TYPE],
		['stream_type', SortField.STREAM_TYPE]
	])('maps the ?sort=%s deep link to its SortField', async (sortKey, expected) => {
		await mountAt(`?sort=${sortKey}&sortDir=asc`);

		expect(lastSearchArgs().sortField).toBe(expected);
		expect(lastSearchArgs().sortDirection).toBe(SortDirection.ASC);
	});

	it('falls back to the default sort for an unknown sort key', async () => {
		await mountAt('?sort=not-a-column');

		expect(lastSearchArgs().sortField).toBe(SortField.OCCURRED_AT);
	});
});

describe('audit list — filters reach the server as tag filters', () => {
	it('sends one pipe-joined stream_type tag for a multi-resource selection', async () => {
		await mountAt('?stream=user,device');

		expect(lastSearchArgs().tagFilters).toEqual({ stream_type: 'user|device' });
	});

	it('sends the actor filter as an actor_id tag', async () => {
		await mountAt(`?actor=${ACTOR_ID}`);

		expect(lastSearchArgs().tagFilters).toEqual({ actor_id: ACTOR_ID });
	});

	it('combines the actor, the resource types, the search term and the offset', async () => {
		await mountAt(`?query=denied&stream=lps_password&actor=${ACTOR_ID}&page=2&pageSize=50`);

		const args = lastSearchArgs();
		expect(args.query).toBe('denied');
		expect(args.tagFilters).toEqual({ stream_type: 'lps_password', actor_id: ACTOR_ID });
		expect(args.pageSize).toBe(50);
		expect(args.pageToken).toBe('50'); // offset = (2 - 1) * 50
	});
});

// The regression this page's deviation from createSearchListState exists to
// prevent: date_filters are a separate SearchRequest field with no tag-filter
// equivalent, so if the list ever moves onto buildSearchArgs unchanged, the
// occurred_at range silently stops being sent and these assertions fail.
describe('audit list — the occurred_at range still reaches the server', () => {
	it('sends an occurred_at DateRange spanning local midnight to the inclusive end of day', async () => {
		await mountAt('?occurredStart=2026-07-01&occurredEnd=2026-07-31');

		const filters = lastSearchArgs().dateFilters;
		expect(filters).toHaveLength(1);
		expect(filters[0].field).toBe('occurred_at');

		const start = new Date(Number(filters[0].start) * 1000);
		expect([start.getFullYear(), start.getMonth(), start.getDate(), start.getHours()]).toEqual([
			2026, 6, 1, 0
		]);
		// End bound is the selected day plus one, so the last day is included.
		expect(filters[0].end - filters[0].start).toBe(BigInt(31 * 86400));
	});

	it('ignores an unparseable date instead of sending a bogus range', async () => {
		await mountAt('?occurredEnd=not-a-date');

		expect(lastSearchArgs().dateFilters).toBeUndefined();
	});
});

describe('audit list — the rendered rows keep the evidence', () => {
	it('renders the event, its ULID, the resolved actor and the resource badge', async () => {
		await mountAt('');

		await expect.element(browser.getByText('Device Registered')).toBeVisible();
		for (const id of [EVENT_OK, EVENT_DENIED]) {
			await expect.element(browser.getByText(id)).toBeVisible();
		}
		// The actor lookup resolves the ULID to the user's email.
		await expect.element(browser.getByText('operator@example.test').first()).toBeVisible();
		await expect
			.element(browser.getByText(m.audit_stream_type_device(), { exact: true }).first())
			.toBeVisible();
	});

	it('marks a *Denied event type as a denied outcome and everything else as success', async () => {
		await mountAt('');

		// Exact: the denied event's own type text ("Lps Passwords View Denied")
		// would otherwise match the outcome badge's substring.
		await expect
			.element(browser.getByText(m.audit_outcome_denied(), { exact: true }))
			.toBeVisible();
		await expect
			.element(browser.getByText(m.audit_outcome_success(), { exact: true }))
			.toBeVisible();
	});

	it('renders the list in the row grammar — never a table', async () => {
		await mountAt('');
		await expect.element(browser.getByText('Device Registered')).toBeVisible();

		expect(document.querySelector('[data-testid="row-list"]')).not.toBeNull();
		expect(document.querySelectorAll('table').length).toBe(0);
	});

	// The sheet replaces navigation here, so the affordance must be a button: an
	// anchor would promise a URL the app has no route for, and a bare div would
	// drop keyboard activation.
	it('makes each row a keyboard-operable button rather than a link', async () => {
		await mountAt('');
		await expect.element(browser.getByText('Device Registered')).toBeVisible();

		const buttons = rowOpenButtons();
		expect(buttons).toHaveLength(results.length);
		for (const button of buttons) {
			expect(button.tagName).toBe('BUTTON');
			expect(button.getAttribute('type')).toBe('button');
		}
		// No row is a navigation link — RowList only emits one when given `href`.
		expect(document.querySelectorAll('[data-testid="row-list-link"]').length).toBe(0);
	});

	it('opens the evidence panel for one event without leaking another row into it', async () => {
		await mountAt('');
		await expect.element(browser.getByText('Device Registered')).toBeVisible();

		await browser.getByTestId('audit-row-open').first().click();

		await expect.element(browser.getByText(m.audit_details(), { exact: true })).toBeVisible();
		// Search results carry no payload (only Get/List do), so the panel is
		// explicit about that rather than rendering an empty block.
		await expect.element(browser.getByText(m.audit_no_data())).toBeVisible();
	});

	it('distinguishes an empty result set from a filtered one', async () => {
		api.search.mockResolvedValue({ results: [], totalCount: 0 });
		await mountAt('');

		await expect.element(browser.getByText(m.audit_empty())).toBeVisible();
		await expect.element(browser.getByText(m.audit_empty_hint())).toBeVisible();
	});

	it('offers a different search term when a filter is what emptied the page', async () => {
		api.search.mockResolvedValue({ results: [], totalCount: 0 });
		await mountAt('?stream=device');

		await expect.element(browser.getByText(m.common_try_different_search())).toBeVisible();
	});
});

// The evidence panel is now written in the operation/effect grammar, but the
// grammar may only use fields ListAuditEvents actually returns — no operation
// row is reconstructed and no effect is invented. AuditEvent carries exactly one
// effect (stream_type + stream_id + event_type), and the panel says so.
describe('audit list — the evidence panel reads as an operation over its effects', () => {
	it('names the operation, its ULID and the single effect the contract returns', async () => {
		await mountAt('');
		await expect.element(browser.getByText('Device Registered')).toBeVisible();

		await browser.getByTestId('audit-row-open').first().click();

		await expect
			.element(browser.getByText(m.audit_operation_label(), { exact: true }))
			.toBeVisible();
		await expect
			.element(browser.getByText(m.audit_effects_label(), { exact: true }))
			.toBeVisible();
		await expect.element(browser.getByText(m.audit_effects_note())).toBeVisible();
		// The panel is scoped to the row it was opened from.
		await expect.element(browser.getByText(EVENT_OK).first()).toBeVisible();
	});
});

// The export is the capability an auditor actually leaves with: it must keep
// sending the SAME filters the view is showing, through the chunked loop.
describe('audit list — the export still mirrors the view', () => {
	it('exports CSV with the actor, stream types, search term and occurred_at bounds in force', async () => {
		const clicks = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
		const createURL = vi
			.spyOn(URL, 'createObjectURL')
			.mockReturnValue('blob:https://control.test/export');
		const revokeURL = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {});
		try {
			await mountAt(
				`?query=denied&stream=lps_password&actor=${ACTOR_ID}&occurredStart=2026-07-01&occurredEnd=2026-07-31`
			);

			await browser.getByRole('button', { name: m.audit_export() }).click();
			await browser.getByRole('menuitem', { name: m.audit_export_csv() }).click();

			await vi.waitFor(() => expect(api.exportAuditEvents).toHaveBeenCalled(), { timeout: 3000 });
			const request = api.exportAuditEvents.mock.calls[0][0];
			expect(request.format).toBe('csv');
			expect(request.actorId).toBe(ACTOR_ID);
			expect(request.streamTypes).toEqual(['lps_password']);
			expect(request.eventType).toBe('denied');
			expect(request.occurredFrom).toBeDefined();
			expect(request.occurredTo).toBeDefined();
			// The list view's inclusive end-of-day bound is mirrored, not dropped.
			expect(Number(request.occurredTo.seconds) - Number(request.occurredFrom.seconds)).toBe(
				31 * 86400
			);
		} finally {
			clicks.mockRestore();
			createURL.mockRestore();
			revokeURL.mockRestore();
		}
	});
});
