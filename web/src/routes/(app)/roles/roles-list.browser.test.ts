// Conversion contract for the roles list page.
//
// Roles have no Search scope, so ListRoles returns the whole set and the page's
// own matching / sorting / paging decide what an operator sees. These tests pin
// what the move to the shared row grammar (RowList) must not lose: the list is
// dense rows, not a table; every row links to its role detail; the permission
// count and the System marker still read off the row; and the system-role delete
// refusal — the page's only authorization-shaped guard — still holds.

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page as browser } from 'vitest/browser';
import { create } from '@bufbuild/protobuf';
import { RoleSchema } from '$contract/cadestro/v1/control_pb';
import * as m from '$lib/paraglide/messages';

const SYSTEM_ROLE_ID = '01JQZZ4A7K3M9P2Q6R8T1V0W5X';
const CUSTOM_ROLE_ID = '01JQZZ5B8N4P0R3S7T9V2W1X6Y';

const api = vi.hoisted(() => ({ listRoles: vi.fn(), deleteRole: vi.fn(), createRole: vi.fn() }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
// Mutable so each test can mount the page "at" a different deep link.
const nav = vi.hoisted(() => ({ url: new URL('https://control.test/roles') }));

vi.mock('svelte-sonner', () => ({ toast }));

// Only the client is faked; the generated protobuf re-exports stay real.
vi.mock('$lib/sdk', async () => {
	const common = await import('$contract/cadestro/v1/common_pb');
	const control = await import('$contract/cadestro/v1/control_pb');
	const actions = await import('$contract/cadestro/v1/actions_pb');
	return {
		...actions,
		...control,
		...common,
		apiClient: api,
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

import RolesPage from './+page.svelte';

const roles = [
	create(RoleSchema, {
		id: { value: SYSTEM_ROLE_ID },
		name: 'Admin',
		description: 'Full control',
		isSystem: true,
		permissions: ['devices:read', 'devices:write', 'roles:write']
	}),
	create(RoleSchema, {
		id: { value: CUSTOM_ROLE_ID },
		name: 'Helpdesk',
		description: '',
		isSystem: false,
		permissions: ['devices:read']
	})
];

beforeEach(() => {
	document.body.innerHTML = '';
	vi.clearAllMocks();
	nav.url = new URL('https://control.test/roles');
	api.listRoles.mockResolvedValue({ roles });
});

/** The rendered rows, addressed by the ULID each row shell carries. */
function rowKeys(): string[] {
	return [...document.querySelectorAll<HTMLElement>('[data-testid="row-list-row"]')].map(
		(el) => el.getAttribute('data-row-key') ?? ''
	);
}

/** Sort controls live in the row list's sort bar; addressing them by text alone
 *  would also reach the row overflow triggers. */
function clickSort(label: string) {
	const button = [
		...document.querySelectorAll<HTMLButtonElement>('[data-testid="row-list-sort"] button')
	].find((b) => b.textContent?.trim().startsWith(label));
	if (!button) throw new Error(`no sort control named ${label}`);
	button.click();
}

async function mountAt(query: string) {
	nav.url = new URL(`https://control.test/roles${query}`);
	render(RolesPage);
	await vi.waitFor(() => expect(api.listRoles).toHaveBeenCalled(), { timeout: 3000 });
}

describe('roles list — the row grammar', () => {
	it('renders the list as dense rows, never a table', async () => {
		await mountAt('');
		await vi.waitFor(() => expect(rowKeys()).toEqual([SYSTEM_ROLE_ID, CUSTOM_ROLE_ID]), {
			timeout: 3000
		});

		expect(document.querySelector('[data-testid="row-list"]')).not.toBeNull();
		expect(document.querySelectorAll('table').length).toBe(0);
	});

	it('makes each row a link to its role detail', async () => {
		await mountAt('');
		await vi.waitFor(() => expect(rowKeys()).toHaveLength(2), { timeout: 3000 });

		const links = [
			...document.querySelectorAll<HTMLAnchorElement>('[data-testid="row-list-link"]')
		].map((a) => a.getAttribute('href'));
		expect(links).toEqual([`/roles/${SYSTEM_ROLE_ID}`, `/roles/${CUSTOM_ROLE_ID}`]);
	});

	it('keeps the ULID, the System marker and the permission count on the row', async () => {
		await mountAt('');

		await expect.element(browser.getByText('Admin')).toBeVisible();
		await expect.element(browser.getByText(SYSTEM_ROLE_ID)).toBeVisible();
		await expect.element(browser.getByText(CUSTOM_ROLE_ID)).toBeVisible();
		await expect.element(browser.getByText(m.roles_system_badge())).toBeVisible();
		await expect
			.element(browser.getByText(m.roles_index_permissions_count({ count: 3 })))
			.toBeVisible();
		await expect
			.element(browser.getByText(m.roles_index_permissions_count({ count: 1 })))
			.toBeVisible();
		// A role without one still says so rather than showing an empty gap.
		await expect.element(browser.getByText(m.common_no_description())).toBeVisible();
	});

	it('re-sorts by permission count from the sort bar', async () => {
		await mountAt('');
		// name asc is the default: Admin before Helpdesk.
		await vi.waitFor(() => expect(rowKeys()).toEqual([SYSTEM_ROLE_ID, CUSTOM_ROLE_ID]), {
			timeout: 3000
		});

		clickSort(m.roles_permission_count());
		// permissions asc: Helpdesk (1) before Admin (3).
		await vi.waitFor(() => expect(rowKeys()).toEqual([CUSTOM_ROLE_ID, SYSTEM_ROLE_ID]), {
			timeout: 3000
		});
	});

	it('matches the search box query against the role name', async () => {
		await mountAt('?query=helpdesk');

		await vi.waitFor(() => expect(rowKeys()).toEqual([CUSTOM_ROLE_ID]), { timeout: 3000 });
	});
});

describe('roles list — the system-role delete refusal survives the conversion', () => {
	async function openRowMenu(index: number) {
		const triggers = document.querySelectorAll<HTMLButtonElement>('button[aria-label="Actions"]');
		expect(triggers.length).toBe(2);
		triggers[index].click();
		return vi.waitFor(() => {
			const el = document.querySelector('[role="menuitem"][data-highlighted], [role="menuitem"]');
			if (!el) throw new Error('row menu did not open');
			return el;
		});
	}

	it('disables delete for a system role and never calls DeleteRole', async () => {
		await mountAt('');
		await vi.waitFor(() => expect(rowKeys()).toHaveLength(2), { timeout: 3000 });

		await openRowMenu(0);
		const items = [...document.querySelectorAll('[role="menuitem"]')];
		const deleteItem = items.find((el) => el.textContent?.includes(m.common_delete()));
		expect(deleteItem, 'the system row offers a delete item').toBeTruthy();
		expect(deleteItem!.getAttribute('aria-disabled')).toBe('true');
		expect(deleteItem!.hasAttribute('data-disabled')).toBe(true);

		(deleteItem as HTMLElement).click();
		expect(api.deleteRole).not.toHaveBeenCalled();
	});

	it('leaves delete enabled for a custom role', async () => {
		await mountAt('');
		await vi.waitFor(() => expect(rowKeys()).toHaveLength(2), { timeout: 3000 });

		await openRowMenu(1);
		const items = [...document.querySelectorAll('[role="menuitem"]')];
		const deleteItem = items.find((el) => el.textContent?.includes(m.common_delete()));
		expect(deleteItem, 'the custom row offers a delete item').toBeTruthy();
		expect(deleteItem!.getAttribute('aria-disabled')).not.toBe('true');
		expect(deleteItem!.hasAttribute('data-disabled')).toBe(false);
	});
});

describe('roles list — empty states', () => {
	it('distinguishes "no roles yet" from "nothing matched"', async () => {
		api.listRoles.mockResolvedValue({ roles: [] });
		await mountAt('');
		await expect.element(browser.getByText(m.roles_empty_hint())).toBeVisible();

		document.body.innerHTML = '';
		api.listRoles.mockResolvedValue({ roles });
		await mountAt('?query=nothing-matches-this');
		await expect.element(browser.getByText(m.common_try_different_search())).toBeVisible();
	});
});
