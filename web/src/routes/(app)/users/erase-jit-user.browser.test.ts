import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { ErrorCode, ErrorDetailSchema } from '$contract/cadestro/v1/common_pb';
import { SearchResultSchema } from '$contract/cadestro/v1/control_pb';
import * as m from '$lib/paraglide/messages';
import UsersPage from './+page.svelte';

const USER_ID = '01JQZ8N4M9T0K3V7X2C5B6D8E1';
const USER_EMAIL = 'jit@example.test';

const api = vi.hoisted(() => ({
	search: vi.fn(),
	eraseJITUser: vi.fn()
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

// The server fails closed for SCIM-provisioned subjects: FailedPrecondition
// carrying ErrorDetail.code = "scim_managed_resource".
function scimManagedRejection() {
	return new ConnectError('user is managed by scim', Code.FailedPrecondition, undefined, [
		{ desc: ErrorDetailSchema, value: { code: ErrorCode.SCIM_MANAGED_RESOURCE } }
	]);
}

beforeEach(() => {
	vi.clearAllMocks();
	api.search.mockResolvedValue({
		results: [
			create(SearchResultSchema, {
				id: { value: USER_ID },
				name: USER_EMAIL,
				fields: { email: USER_EMAIL }
			})
		],
		totalCount: 1
	});
	api.eraseJITUser.mockResolvedValue(undefined);
});

// The list renders in the shared row grammar: the whole row body is ONE link to
// the user's detail page, so the email is no longer its own anchor. Address the
// row by a link whose accessible name mentions the user — the same identity the
// old per-name anchor carried.
const userRow = () => page.getByRole('link', { name: new RegExp(USER_EMAIL, 'i') });

async function renderList() {
	render(UsersPage);
	await expect.element(userRow()).toBeVisible();
}

async function openEraseDialog() {
	await page.getByRole('button', { name: m.common_actions() }).click();
	await page.getByRole('menuitem', { name: m.users_erase_action() }).click();
	const dialog = page.getByRole('alertdialog');
	await expect.element(dialog).toBeVisible();
	return dialog;
}

describe('users list — the row grammar', () => {
	it('renders each user as a linked row — never a table', async () => {
		await renderList();

		expect(document.querySelector('[data-testid="row-list"]')).not.toBeNull();
		expect(document.querySelectorAll('table').length).toBe(0);

		const rows = [...document.querySelectorAll<HTMLElement>('[data-testid="row-list-row"]')];
		expect(rows.map((r) => r.getAttribute('data-row-key'))).toEqual([USER_ID]);

		const links = [
			...document.querySelectorAll<HTMLAnchorElement>('[data-testid="row-list-link"]')
		].map((a) => a.getAttribute('href'));
		expect(links).toEqual([`/users/${USER_ID}`]);
	});
});

describe('users list — provisioning is not manual', () => {
	it('offers no create-user affordance and explains how users appear', async () => {
		await renderList();

		expect(page.getByRole('button', { name: /create user/i }).elements()).toHaveLength(0);
		expect(document.body.textContent).not.toMatch(/create user/i);
		await expect.element(page.getByText(m.users_provisioning_hint())).toBeVisible();
	});

	// The empty state used to carry a second create button.
	it('offers no create-user affordance when the list is empty', async () => {
		api.search.mockResolvedValue({ results: [], totalCount: 0 });
		render(UsersPage);

		await expect.element(page.getByText(m.users_empty())).toBeVisible();
		expect(page.getByRole('button', { name: /create user/i }).elements()).toHaveLength(0);
		expect(document.body.textContent).not.toMatch(/create user/i);
	});
});

describe('users list — erase JIT user', () => {
	it('erases the confirmed user through EraseJITUser and drops the row', async () => {
		await renderList();
		const dialog = await openEraseDialog();

		await expect.element(dialog.getByText(m.users_erase_dialog_scim_note())).toBeVisible();
		await dialog.getByRole('button', { name: m.users_erase_action() }).click();

		await vi.waitFor(() => expect(api.eraseJITUser).toHaveBeenCalledWith(USER_ID));
		await vi.waitFor(() => expect(toast.success).toHaveBeenCalledWith(m.users_erased()));
		await vi.waitFor(() => expect(userRow().elements()).toHaveLength(0));
	});

	it('keeps the row and shows the localized SCIM message when the server refuses', async () => {
		api.eraseJITUser.mockRejectedValue(scimManagedRejection());
		await renderList();
		const dialog = await openEraseDialog();

		await dialog.getByRole('button', { name: m.users_erase_action() }).click();

		await vi.waitFor(() =>
			expect(toast.error).toHaveBeenCalledWith(m.error_scim_managed_resource())
		);
		expect(api.eraseJITUser).toHaveBeenCalledWith(USER_ID);
		expect(toast.success).not.toHaveBeenCalled();
		await expect.element(userRow()).toBeVisible();
	});
});
