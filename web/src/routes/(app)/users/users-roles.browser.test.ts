// The users list's role columns come off the SEARCH DOCUMENT (server 031605d):
// role_ids/role_names are direct grants, inherited_role_ids/_names one entry
// per (role, group) pair. The document carries NO grant scope kinds — so the
// row renders chips from it, but everything scope-aware (revoke, the assign
// dialog's unscoped-exclusion) must resolve the full GetUser record instead of
// consuming a fabricated UNSPECIFIED scope. These tests pin both halves.
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import { create } from '@bufbuild/protobuf';
import { SearchResultSchema } from '$contract/cadestro/v1/control_pb';
import { RoleGrantScopeKind } from '$contract/cadestro/v1/common_pb';
import * as m from '$lib/paraglide/messages';
import UsersPage from './+page.svelte';

const USER_ID = '01JQZ8N4M9T0K3V7X2C5B6D8E1';
const NEVER_ID = '01JQZ8N4M9T0K3V7X2C5B6D8E2';

const ROLE_ADMIN = '00000000000000000000000001';
const ROLE_USER = '00000000000000000000000002';
const ROLE_HELP = '01JR0ROLEHELPDESK000000000';
const ROLE_VIEW = '01JR0ROLEVIEWER00000000000';

const api = vi.hoisted(() => ({
	search: vi.fn(),
	getUser: vi.fn(),
	revokeRoleFromUser: vi.fn(),
	assignRoleToUser: vi.fn(),
	listRoles: vi.fn(),
	listDeviceGroups: vi.fn(),
	listUserGroups: vi.fn(),
	listPermissions: vi.fn()
}));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));

vi.mock('svelte-sonner', () => ({ toast }));
vi.mock('$app/state', () => ({
	page: { url: new URL('https://control.test/users'), params: {} }
}));
vi.mock('$lib/sdk', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/sdk')>()),
	apiClient: api
}));

beforeEach(() => {
	vi.clearAllMocks();
	api.search.mockResolvedValue({
		results: [
			create(SearchResultSchema, {
				id: USER_ID,
				name: 'ops@example.test',
				fields: {
					email: 'ops@example.test',
					last_login_at: '1750000000',
					// Direct grants: Admin, Helpdesk, and the system User role.
					role_ids: `${ROLE_ADMIN}, ${ROLE_HELP}, ${ROLE_USER}`,
					role_names: 'Admin, Helpdesk, User',
					// Inherited pairs: Viewer via two groups (one chip), and
					// Helpdesk via a group — already granted directly, so no chip.
					inherited_role_ids: `${ROLE_VIEW}, ${ROLE_VIEW}, ${ROLE_HELP}`,
					inherited_role_names: 'Viewer, Viewer, Helpdesk'
				}
			}),
			create(SearchResultSchema, {
				id: NEVER_ID,
				name: 'new@example.test',
				fields: { email: 'new@example.test', last_login_at: '0' }
			})
		],
		totalCount: 2
	});
	// The detail record is the ONLY holder of scope truth: Helpdesk is held
	// twice, once unscoped and once scoped to a user group.
	api.getUser.mockResolvedValue({
		id: USER_ID,
		email: 'ops@example.test',
		roleGrants: [
			{
				role: { id: ROLE_HELP, name: 'Helpdesk' },
				scopeKind: RoleGrantScopeKind.USER_GROUP,
				scopeId: '01JR0GROUPSCOPE00000000000'
			},
			{
				role: { id: ROLE_HELP, name: 'Helpdesk' },
				scopeKind: RoleGrantScopeKind.UNSPECIFIED,
				scopeId: ''
			}
		],
		identityLinks: []
	});
	api.revokeRoleFromUser.mockResolvedValue(undefined);
	api.listRoles.mockResolvedValue({ roles: [] });
	api.listDeviceGroups.mockResolvedValue({ groups: [], nextPageToken: '' });
	api.listUserGroups.mockResolvedValue({ groups: [], nextPageToken: '' });
	api.listPermissions.mockResolvedValue({ permissions: [] });
});

const row = (email: string) =>
	[...document.querySelectorAll<HTMLElement>('[data-testid="row-list-row"]')].find((r) =>
		r.textContent?.includes(email)
	);

async function renderList() {
	render(UsersPage);
	await vi.waitFor(() => expect(row('ops@example.test')).toBeTruthy(), { timeout: 3000 });
}

describe('users list — role chips come off the search document', () => {
	it('renders direct grants and inherited roles as two deduped chip clusters', async () => {
		await renderList();
		const cluster = row('ops@example.test')!.querySelector(`[title="${m.users_table_role()}"]`)!;
		const chips = [...cluster.querySelectorAll('[data-testid="fleet-chip"], span')]
			.map((c) => c.textContent?.trim())
			.filter(Boolean);
		const text = cluster.textContent ?? '';

		// Direct: one chip per grant entry.
		expect(text).toContain('Admin');
		expect(text).toContain('Helpdesk');
		expect(text).toContain('User');
		// Inherited: Viewer once (two group pairs, deduped by role id) …
		expect(text.match(/Viewer/g)).toHaveLength(1);
		// … and Helpdesk not repeated as inherited beside its direct chip.
		expect(text.match(/Helpdesk/g)).toHaveLength(1);
		expect(chips.length).toBeGreaterThan(0);
	});

	it("renders last_login_at from the document, with '0' meaning never", async () => {
		await renderList();

		// The formatter renders undefined as 'Never'; a real epoch is a date.
		expect(row('new@example.test')!.textContent).toContain('Never');
		expect(row('ops@example.test')!.textContent).not.toContain('Never');
	});
});

describe('users list — scope truth stays with GetUser', () => {
	it('revokes through the detail record\'s REAL scopes, never a fabricated one', async () => {
		await renderList();

		await row('ops@example.test')!
			.querySelector<HTMLButtonElement>(`[aria-label="${m.common_actions()}"]`)!
			.click();
		await page
			.getByRole('menuitem', { name: `${m.roles_revoke_from_user()}: Helpdesk` })
			.click();

		await vi.waitFor(() => expect(api.getUser).toHaveBeenCalledWith(USER_ID), { timeout: 3000 });
		// Both real grants revoked, each with its true scope off GetUser.
		await vi.waitFor(() => expect(api.revokeRoleFromUser).toHaveBeenCalledTimes(2), {
			timeout: 3000
		});
		expect(api.revokeRoleFromUser).toHaveBeenCalledWith(
			USER_ID,
			ROLE_HELP,
			RoleGrantScopeKind.USER_GROUP,
			'01JR0GROUPSCOPE00000000000'
		);
		expect(api.revokeRoleFromUser).toHaveBeenCalledWith(
			USER_ID,
			ROLE_HELP,
			RoleGrantScopeKind.UNSPECIFIED,
			''
		);
	});

	it('never offers to revoke the system User role — its id is the contract\'s, not a guess', async () => {
		await renderList();

		await row('ops@example.test')!
			.querySelector<HTMLButtonElement>(`[aria-label="${m.common_actions()}"]`)!
			.click();

		await expect
			.element(page.getByRole('menuitem', { name: `${m.roles_revoke_from_user()}: Admin` }))
			.toBeVisible();
		expect(
			page.getByRole('menuitem', { name: `${m.roles_revoke_from_user()}: User` }).elements()
		).toHaveLength(0);
	});

	it('feeds the assign dialog from GetUser, not from the scope-less row', async () => {
		await renderList();

		await row('ops@example.test')!
			.querySelector<HTMLButtonElement>(`[aria-label="${m.common_actions()}"]`)!
			.click();
		await page.getByRole('menuitem', { name: m.roles_assign_to_user() }).click();

		await vi.waitFor(() => expect(api.getUser).toHaveBeenCalledWith(USER_ID), { timeout: 3000 });
	});
});
