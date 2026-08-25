

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page as browser } from 'vitest/browser';
import { create } from '@bufbuild/protobuf';
import { IdentityProviderSchema } from '$contract/cadestro/v1/control_pb';
import { IdentityProviderType } from '$contract/cadestro/v1/common_pb';
import * as m from '$lib/paraglide/messages';
import {
	shell,
	resetShell,
	pillMode,
	commitContext,
	requestCancelContext,
	confirmCancelContext,
	runPillAction,
	stashContext,
	restoreDraft,
	claimDraft,
	setShellPath
} from '$lib/shell/shell.svelte';

const IDP_ID = vi.hoisted(() => '01JR0B1C2D3E4F5G6H7J8K9M0N');

const api = vi.hoisted(() => ({
	getIdentityProvider: vi.fn(),
	updateIdentityProvider: vi.fn(),
	deleteIdentityProvider: vi.fn(),
	enableSCIM: vi.fn(),
	disableSCIM: vi.fn(),
	rotateSCIMToken: vi.fn(),
	listRoles: vi.fn()
}));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));

vi.mock('svelte-sonner', () => ({ toast }));
vi.mock('$app/paths', () => ({ base: '', assets: '' }));
vi.mock('$lib/navigation', () => ({ goto: vi.fn(), pushState: vi.fn(), replaceState: vi.fn() }));
vi.mock('$app/state', () => ({
	page: {
		url: new URL(`https://control.test/identity-providers/${IDP_ID}`),
		params: { id: IDP_ID }
	}
}));
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

import IdentityProviderDetailPage from './+page.svelte';

let scimEnabled = false;

const ROLE_ADMIN_ID = '00000000000000000000000001';
const ROLE_USER_ID = '00000000000000000000000002';

const stored = () =>
	create(IdentityProviderSchema, {
		id: { value: IDP_ID },
		name: 'Corporate Entra',
		slug: 'entra',
		providerType: IdentityProviderType.OIDC,
		enabled: true,
		clientId: { value: 'client-abc' },
		issuerUrl: 'https://login.example.test',
		authorizationUrl: 'https://login.example.test/authorize',
		tokenUrl: 'https://login.example.test/token',
		userinfoUrl: 'https://login.example.test/userinfo',
		scopes: ['openid', 'profile', 'email'],
		autoCreateUsers: true,
		autoLinkByEmail: false,
		trustEmailAssertions: false,
		defaultRoleId: { value: ROLE_USER_ID },
		groupClaim: 'groups',
		scimEnabled,
		scimEndpointUrl: scimEnabled ? 'https://control.test/scim/v2/entra' : ''
	});

const UNTOUCHED = {
	id: IDP_ID,
	enabled: true,
	clientId: 'client-abc',
	clientSecret: undefined,
	issuerUrl: 'https://login.example.test',
	authorizationUrl: 'https://login.example.test/authorize',
	tokenUrl: 'https://login.example.test/token',
	userinfoUrl: 'https://login.example.test/userinfo',
	scopes: ['openid', 'profile', 'email'],
	autoCreateUsers: true,
	autoLinkByEmail: false,
	trustEmailAssertions: false,

	defaultRoleId: ROLE_USER_ID,
	groupClaim: 'groups'
};

beforeEach(() => {
	document.body.innerHTML = '';
	vi.clearAllMocks();
	resetShell();
	scimEnabled = false;
	api.getIdentityProvider.mockImplementation(async () => stored());
	api.updateIdentityProvider.mockImplementation(async () => stored());
	api.listRoles.mockResolvedValue({
		roles: [
			{ id: { value: ROLE_ADMIN_ID }, name: 'Admin', description: 'Full access', permissions: [] },
			{ id: { value: ROLE_USER_ID }, name: 'User', description: 'Standard access', permissions: [] }
		]
	});
	api.enableSCIM.mockImplementation(async () => {
		scimEnabled = true;
		return { token: 'scim-token-once' };
	});
});

async function mount() {

	setShellPath(`/identity-providers/${IDP_ID}`);
	render(IdentityProviderDetailPage);
	await vi.waitFor(() => expect(api.getIdentityProvider).toHaveBeenCalledWith(IDP_ID), {
		timeout: 3000
	});
	await expect.element(browser.getByLabelText(m.idp_field_name())).toBeVisible();
}

const nameField = () => browser.getByLabelText(m.idp_field_name());

describe('identity-provider editor — the commit rides the context pill', () => {
	it('carries no save button on the page itself', async () => {
		await mount();

		expect(browser.getByRole('button', { name: m.common_save() }).elements()).toHaveLength(0);
		expect(browser.getByRole('button', { name: m.common_saving() }).elements()).toHaveLength(0);
	});

	it('holds the provider from load and goes dirty on the first edit', async () => {
		await mount();

		await vi.waitFor(() => expect(pillMode()).toBe('context'), { timeout: 3000 });
		expect(shell.pill.context?.id).toBe(`identity-provider:${IDP_ID}`);

		expect(shell.pill.context?.dirty).toBe(false);
		expect(commitContext()).toBe(false);

		expect(shell.pill.context?.extraActions?.map((a) => a.id)).toEqual([
			'scim-enable',
			'delete'
		]);

		await nameField().fill('Corporate Entra ID');

		await vi.waitFor(() => expect(shell.pill.context?.dirty).toBe(true), { timeout: 3000 });
		expect(pillMode()).toBe('context');
		expect(shell.pill.context?.valid).toBe(true);
		expect(shell.pill.context?.commitLabel).toBe(m.common_save());

		expect(api.updateIdentityProvider).not.toHaveBeenCalled();
	});

	it('commits the loaded provider with exactly the edited field swapped in', async () => {
		await mount();

		await nameField().fill('Corporate Entra ID');

		await vi.waitFor(() => expect(shell.pill.context?.dirty).toBe(true), { timeout: 3000 });

		expect(commitContext()).toBe(true);

		await vi.waitFor(() => expect(api.updateIdentityProvider).toHaveBeenCalledTimes(1), {
			timeout: 3000
		});
		expect(api.updateIdentityProvider.mock.calls[0][0]).toEqual({
			...UNTOUCHED,
			name: 'Corporate Entra ID'
		});
	});

	it('keeps the stored defaultRoleId when saving an unrelated edit', async () => {
		await mount();

		await nameField().fill('Corporate Entra ID');
		await vi.waitFor(() => expect(shell.pill.context?.dirty).toBe(true), { timeout: 3000 });
		expect(commitContext()).toBe(true);

		await vi.waitFor(() => expect(api.updateIdentityProvider).toHaveBeenCalledTimes(1), {
			timeout: 3000
		});
		expect(api.updateIdentityProvider.mock.calls[0][0].defaultRoleId).toBe(ROLE_USER_ID);
	});

	it('changes the default role through the ListRoles-backed select', async () => {
		await mount();
		await vi.waitFor(() => expect(api.listRoles).toHaveBeenCalled(), { timeout: 3000 });

		await browser.getByLabelText(m.idp_field_default_role()).click();
		await browser.getByRole('option', { name: 'Admin' }).click();

		await vi.waitFor(() => expect(shell.pill.context?.dirty).toBe(true), { timeout: 3000 });
		expect(commitContext()).toBe(true);

		await vi.waitFor(() => expect(api.updateIdentityProvider).toHaveBeenCalledTimes(1), {
			timeout: 3000
		});
		expect(api.updateIdentityProvider.mock.calls[0][0]).toEqual({
			...UNTOUCHED,
			name: 'Corporate Entra',
			defaultRoleId: ROLE_ADMIN_ID
		});
	});

	it('clears the default role via the "no default role" option, sending the empty string', async () => {
		await mount();
		await vi.waitFor(() => expect(api.listRoles).toHaveBeenCalled(), { timeout: 3000 });

		await browser.getByLabelText(m.idp_field_default_role()).click();
		await browser.getByRole('option', { name: m.idp_field_default_role_none() }).click();

		await vi.waitFor(() => expect(shell.pill.context?.dirty).toBe(true), { timeout: 3000 });
		expect(commitContext()).toBe(true);

		await vi.waitFor(() => expect(api.updateIdentityProvider).toHaveBeenCalledTimes(1), {
			timeout: 3000
		});
		expect(api.updateIdentityProvider.mock.calls[0][0].defaultRoleId).toBe('');
	});

	it('carries an edit made in a different section into the same commit', async () => {
		await mount();

		await browser.getByLabelText(m.idp_field_group_claim()).fill('roles');
		await vi.waitFor(() => expect(shell.pill.context?.dirty).toBe(true), { timeout: 3000 });
		expect(commitContext()).toBe(true);

		await vi.waitFor(() => expect(api.updateIdentityProvider).toHaveBeenCalledTimes(1), {
			timeout: 3000
		});
		expect(api.updateIdentityProvider.mock.calls[0][0]).toEqual({
			...UNTOUCHED,
			name: 'Corporate Entra',
			groupClaim: 'roles'
		});
	});

	it('cancels back to the loaded baseline and writes nothing', async () => {
		await mount();

		await nameField().fill('Corporate Entra ID');
		await vi.waitFor(() => expect(shell.pill.context?.dirty).toBe(true), { timeout: 3000 });

		requestCancelContext();
		expect(shell.pill.cancelPending).toBe(true);
		confirmCancelContext();

		await vi.waitFor(() => expect(shell.pill.context?.dirty).toBe(false), { timeout: 3000 });
		expect(pillMode()).toBe('context');
		await expect.element(nameField()).toHaveValue('Corporate Entra');
		expect(api.updateIdentityProvider).not.toHaveBeenCalled();
	});

	it('goes clean again when the edit is typed back to the loaded value', async () => {
		await mount();

		await nameField().fill('Corporate Entra ID');
		await vi.waitFor(() => expect(shell.pill.context?.dirty).toBe(true), { timeout: 3000 });

		await nameField().fill('Corporate Entra');

		await vi.waitFor(() => expect(shell.pill.context?.dirty).toBe(false), { timeout: 3000 });
		expect(commitContext()).toBe(false);
	});

	it('refuses to commit a provider whose display name was emptied', async () => {
		await mount();

		await nameField().fill('');

		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(false), { timeout: 3000 });
		expect(commitContext()).toBe(false);
		expect(api.updateIdentityProvider).not.toHaveBeenCalled();
	});

	it('stashes the draft to the stage and resumes it with the buffers intact', async () => {
		await mount();

		await nameField().fill('Corporate Entra ID');
		await vi.waitFor(() => expect(shell.pill.context?.dirty).toBe(true), { timeout: 3000 });

		const draftId = stashContext();
		expect(draftId).toBe(`draft:identity-provider:${IDP_ID}`);

		await vi.waitFor(() => expect(pillMode()).toBe('nav'), { timeout: 3000 });
		expect(shell.drafts.map((d) => d.id)).toEqual([draftId]);
		expect(api.updateIdentityProvider).not.toHaveBeenCalled();

		expect(restoreDraft(draftId!)).toBeNull();
		await vi.waitFor(() => expect(pillMode()).toBe('context'), { timeout: 3000 });
		expect(shell.drafts).toHaveLength(0);
		await expect.element(nameField()).toHaveValue('Corporate Entra ID');
	});

	it('carries the whole buffer through the restore, so it can be rebuilt from another route', async () => {
		await mount();

		await nameField().fill('Corporate Entra ID');
		await vi.waitFor(() => expect(shell.pill.context?.dirty).toBe(true), { timeout: 3000 });
		const draftId = stashContext();

		setShellPath('/devices');
		expect(restoreDraft(draftId!)).toBe(`/identity-providers/${IDP_ID}`);
		expect(pillMode()).toBe('nav');

		expect(shell.drafts).toHaveLength(0);
		const parked = claimDraft(`identity-provider:${IDP_ID}`) as { name: string };
		expect(parked.name).toBe('Corporate Entra ID');
	});
});

describe('identity-provider editor — the provider’s own actions ride the pill', () => {

	it('enables SCIM from the pill and swaps in the follow-on actions', async () => {
		await mount();
		await vi.waitFor(() => expect(pillMode()).toBe('context'), { timeout: 3000 });

		expect(browser.getByRole('button', { name: m.scim_enable() }).elements()).toHaveLength(0);

		runPillAction('scim-enable');

		await vi.waitFor(() => expect(api.enableSCIM).toHaveBeenCalledWith(IDP_ID), { timeout: 3000 });
		await expect.element(browser.getByText('scim-token-once')).toBeVisible();

		await vi.waitFor(
			() =>
				expect(shell.pill.context?.extraActions?.map((a) => a.id)).toEqual([
					'scim-rotate',
					'scim-disable',
					'delete'
				]),
			{ timeout: 3000 }
		);

		expect(shell.pill.context?.dirty).toBe(false);
	});

	it('routes Delete through its confirmation, not straight to the RPC', async () => {
		await mount();
		await vi.waitFor(() => expect(pillMode()).toBe('context'), { timeout: 3000 });

		runPillAction('delete');

		await expect.element(browser.getByText(m.idp_detail_confirm_delete())).toBeVisible();
		expect(api.deleteIdentityProvider).not.toHaveBeenCalled();

		await browser.getByRole('button', { name: m.common_delete(), exact: true }).click();
		await vi.waitFor(() => expect(api.deleteIdentityProvider).toHaveBeenCalledWith(IDP_ID), {
			timeout: 3000
		});
	});
});
