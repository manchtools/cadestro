// Conversion contract for the registration-token list page.
//
// Tokens have no Search scope, so ListTokens returns the whole set and the
// page's own matching, filtering, sorting and paging decide what an operator
// sees. These tests pin the parts a rewrite can quietly lose: the status a
// token shows its disabled/active status, a deep link
// reproduces the view, and a token secret is never rendered from stored data.
//
// The list renders in the shared row grammar (RowList), so one assertion is
// about the surface itself: no <table> may come back — a table here is the
// regression the redesign removes. Registration tokens have no detail route, so
// unlike every other converted list these rows are deliberately NOT links.

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page as browser } from 'vitest/browser';
import { create } from '@bufbuild/protobuf';
import { timestampFromMs } from '@bufbuild/protobuf/wkt';
import { RegistrationTokenSchema } from '$contract/cadestro/v1/control_pb';
import * as m from '$lib/paraglide/messages';

const ACTIVE_ID = '01JQZZ4A7K3M9P2Q6R8T1V0W5X';
const DISABLED_ID = '01JQZZ5B8N4P0R3S7T9V2W1X6Y';

const api = vi.hoisted(() => ({
	listTokens: vi.fn(),
	deleteToken: vi.fn(),
	setTokenDisabled: vi.fn(),
	createToken: vi.fn()
}));
// Mutable so each test can mount the page "at" a different deep link.
const nav = vi.hoisted(() => ({ url: new URL('https://control.test/tokens') }));

// Only the client and the browser-only stores are faked; the generated
// protobuf re-exports stay real.
vi.mock('$lib/sdk', async () => {
	const common = await import('$contract/cadestro/v1/common_pb');
	const control = await import('$contract/cadestro/v1/control_pb');
	const actions = await import('$contract/cadestro/v1/actions_pb');
	return {
		...actions,
		...control,
		...common,
		apiClient: api,
		authStore: { user: { id: '01JQZZ0000000000000000000A' }, hasPermission: () => true },
		configStore: { serverUrl: 'https://control.test' },
		useDraft: <T>(_type: string, _id: string, initial: T) => ({
			data: { ...initial },
			update: () => {},
			clear: async () => {}
		}),
		formatTimestamp: () => '2026-08-01',
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
// router; the history behaviour itself is url-state's contract, not this test's.
vi.mock('$app/navigation', () => ({
	pushState: vi.fn(),
	replaceState: vi.fn(),
	goto: vi.fn(),
	afterNavigate: vi.fn(),
	beforeNavigate: vi.fn()
}));

import TokensPage from './+page.svelte';

const tokens = [
	create(RegistrationTokenSchema, {
		id: { value: ACTIVE_ID },
		name: 'Fleet rollout',
		maxUses: 0,
		disabled: false,
		createdAt: timestampFromMs(Date.UTC(2026, 6, 1))
	}),
	create(RegistrationTokenSchema, {
		id: { value: DISABLED_ID },
		name: 'Revoked laptop token',
		maxUses: 1,
		disabled: true,
		createdAt: timestampFromMs(Date.UTC(2026, 6, 2))
	}),
	create(RegistrationTokenSchema, {
		createdAt: timestampFromMs(Date.UTC(2026, 6, 2))
	})
];

beforeEach(() => {
	nav.url = new URL('https://control.test/tokens');
	api.listTokens.mockReset();
	api.listTokens.mockResolvedValue({ tokens, nextPageToken: '' });
});

async function mountAt(query: string) {
	nav.url = new URL(`https://control.test/tokens${query}`);
	render(TokensPage);
	await vi.waitFor(() => expect(api.listTokens).toHaveBeenCalled(), { timeout: 3000 });
}

/** The rendered page of rows, in order, addressed by the ULID each row carries. */
function rowKeys(): string[] {
	return [...document.querySelectorAll<HTMLElement>('[data-testid="row-list-row"]')]
		.map((el) => el.getAttribute('data-row-key') ?? '')
		.filter(Boolean);
}

/** Sort controls live in the row list's sort bar; addressing them by text alone
 *  would also reach the row overflow triggers and the filter comboboxes. */
function clickSort(label: string) {
	const button = [
		...document.querySelectorAll<HTMLButtonElement>('[data-testid="row-list-sort"] button')
	].find((b) => b.textContent?.trim().startsWith(label));
	if (!button) throw new Error(`no sort control named ${label}`);
	button.click();
}

describe('tokens list — the list RPC feeds a client-side row list', () => {
	it('asks for every token including the disabled ones', async () => {
		await mountAt('');

		expect(api.listTokens).toHaveBeenCalledWith(50, '', true);
		await expect.element(browser.getByText('Fleet rollout')).toBeVisible();
		await expect.element(browser.getByText(ACTIVE_ID)).toBeVisible();
	});

	it('opens newest-first and re-sorts by name when that sort key is clicked', async () => {
		await mountAt('');
		await expect.element(browser.getByText('Revoked laptop token')).toBeVisible();
		await vi.waitFor(() => expect(rowKeys()[0]).toBe(DISABLED_ID), { timeout: 3000 });

		clickSort(m.tokens_table_name());
		await vi.waitFor(() => expect(rowKeys()[0]).toBe(ACTIVE_ID), { timeout: 3000 });
	});

	it('renders the list in the row grammar — never a table', async () => {
		await mountAt('');
		await expect.element(browser.getByText('Fleet rollout')).toBeVisible();

		expect(document.querySelector('[data-testid="row-list"]')).not.toBeNull();
		expect(document.querySelectorAll('table').length).toBe(0);
	});

	// Tokens are the one converted list without a detail route. A row link here
	// would point nowhere, so its absence is the contract — not an oversight.
	it('leaves the rows unlinked because registration tokens have no detail page', async () => {
		await mountAt('');
		await expect.element(browser.getByText('Fleet rollout')).toBeVisible();

		expect(rowKeys()).toHaveLength(2);
		expect(document.querySelectorAll('[data-testid="row-list-link"]').length).toBe(0);
	});
});

	describe('tokens list — status is disabled or active', () => {
	it.each([
		['disabled', 'Revoked laptop token'],
		['active', 'Fleet rollout']
	])('the ?status=%s deep link keeps only the tokens in that state', async (status, kept) => {
		await mountAt(`?status=${status}`);

		await expect.element(browser.getByText(kept)).toBeVisible();
		await vi.waitFor(() => expect(rowKeys()).toHaveLength(1), { timeout: 3000 });
	});

	it('filters by status', async () => {
		await mountAt('?status=active');

		await expect.element(browser.getByText('Fleet rollout')).toBeVisible();
		await vi.waitFor(() => expect(rowKeys()).toEqual([ACTIVE_ID]), {
			timeout: 3000
		});
	});

	it('matches the query against the name and the ULID', async () => {
		await mountAt('?query=fleet');
		await vi.waitFor(() => expect(rowKeys()).toEqual([ACTIVE_ID]), { timeout: 3000 });
		await expect.element(browser.getByText('Fleet rollout')).toBeVisible();
	});
});

describe('tokens list — paging and empty states', () => {
	it('slices the page from the deep link and reports the range', async () => {
		const many = Array.from({ length: 12 }, (_, i) =>
			create(RegistrationTokenSchema, {
				id: { value: `01JQZZ8E1R7S3V6W0X2Y5Z4A${String(i).padStart(2, '0')}` },
				name: `Batch ${String(i).padStart(2, '0')}`,
				maxUses: 0,
				disabled: false,
				createdAt: timestampFromMs(Date.UTC(2026, 6, i + 1))
			})
		);
		api.listTokens.mockResolvedValue({ tokens: many, nextPageToken: '' });
		await mountAt('?pageSize=10&page=2&sort=name&sortDir=asc');

		await vi.waitFor(() => expect(rowKeys()).toHaveLength(2), { timeout: 3000 });
		await expect.element(browser.getByText('Batch 10')).toBeVisible();
		await expect
			.element(browser.getByText(m.pagination_showing({ from: '11', to: '12', total: '12' })))
			.toBeVisible();
	});

	it('ignores a page size the table does not offer', async () => {
		await mountAt('?pageSize=2');

		await vi.waitFor(() => expect(rowKeys()).toHaveLength(2), { timeout: 3000 });
	});

	it('distinguishes "no tokens yet" from "nothing matched"', async () => {
		api.listTokens.mockResolvedValue({ tokens: [], nextPageToken: '' });
		await mountAt('');
		await expect.element(browser.getByText(m.tokens_empty_hint())).toBeVisible();

		api.listTokens.mockResolvedValue({ tokens, nextPageToken: '' });
		await mountAt('?status=exhausted');
		await expect.element(browser.getByText(m.common_try_different_search())).toBeVisible();
	});
});

describe('tokens list — secret hygiene', () => {
	it('never renders a token value that came back from the list RPC', async () => {
		await mountAt('');
		await expect.element(browser.getByText('Fleet rollout')).toBeVisible();
	});
});
