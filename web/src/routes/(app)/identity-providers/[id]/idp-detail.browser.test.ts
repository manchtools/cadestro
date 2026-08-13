// The identity-provider editor's commit contract.
//
// The provider detail is a multi-section form — connection, endpoints, JIT — and
// every one of those sections feeds ONE committable edit state. That state lives
// in the shell's context pill exactly like the role editor's: the card carries no
// Save button, the commit sends the loaded provider with the edited fields
// swapped in, and Cancel puts every buffer back to the loaded baseline.
//
// The pill is that context from the moment the provider loads, not only while it
// diverges: it is this provider's ACTION BAR, so Delete and the SCIM lifecycle
// live on it. `dirty` is what an edit changes, and it is what turns Save, Stash
// and Cancel on. SCIM enable/disable/rotate and the delete are still one-shot
// writes that never join the form's commit.

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page as browser } from 'vitest/browser';
import { create } from '@bufbuild/protobuf';
import { IdentityProviderSchema } from '$sdk/powermanage/v1/control_pb';
import { IdentityProviderType } from '$sdk/powermanage/v1/common_pb';
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

// Hoisted: the `$app/state` mock factory is lifted above the module body and can
// only close over hoisted values.
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
	const common = await import('$sdk/powermanage/v1/common_pb');
	const control = await import('$sdk/powermanage/v1/control_pb');
	const actions = await import('$sdk/powermanage/v1/actions_pb');
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

// SCIM is server state, so the fake server owns it: enabling flips it and the
// next read reflects that, exactly like the real round trip the page performs.
let scimEnabled = false;

// The fixed system roles ListRoles always returns (plus any custom roles).
const ROLE_ADMIN_ID = '00000000000000000000000001';
const ROLE_USER_ID = '00000000000000000000000002';

const stored = () =>
	create(IdentityProviderSchema, {
		id: IDP_ID,
		name: 'Corporate Entra',
		slug: 'entra',
		providerType: IdentityProviderType.OIDC,
		enabled: true,
		clientId: 'client-abc',
		issuerUrl: 'https://login.example.test',
		authorizationUrl: 'https://login.example.test/authorize',
		tokenUrl: 'https://login.example.test/token',
		userinfoUrl: 'https://login.example.test/userinfo',
		scopes: ['openid', 'profile', 'email'],
		autoCreateUsers: true,
		autoLinkByEmail: false,
		trustEmailAssertions: false,
		defaultRoleId: ROLE_USER_ID,
		groupClaim: 'groups',
		scimEnabled,
		scimEndpointUrl: scimEnabled ? 'https://control.test/scim/v2/entra' : ''
	});

/** The whole request the page sends when nothing but `name` was touched. The
 *  secret is absent, not blank — an untouched secret must never be overwritten. */
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
	// UpdateIdentityProvider is FULL-REPLACE: omitting this field would CLEAR
	// the stored default role on every unrelated save.
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
			{ id: ROLE_ADMIN_ID, name: 'Admin', description: 'Full access', permissions: [] },
			{ id: ROLE_USER_ID, name: 'User', description: 'Standard access', permissions: [] }
		]
	});
	api.enableSCIM.mockImplementation(async () => {
		scimEnabled = true;
		return { token: 'scim-token-once' };
	});
});

async function mount() {
	// The shell's idea of "where the app is": a stashed draft resumes IN PLACE
	// only while its owner is the mounted surface.
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

	// The pill is this provider's ACTION BAR for the whole visit — Delete and the
	// SCIM lifecycle hang off it from the moment the provider loads, so it cannot
	// wait for an edit to appear. What the first keystroke changes is `dirty`.
	it('holds the provider from load and goes dirty on the first edit', async () => {
		await mount();

		await vi.waitFor(() => expect(pillMode()).toBe('context'), { timeout: 3000 });
		expect(shell.pill.context?.id).toBe(`identity-provider:${IDP_ID}`);
		// nothing edited yet: nothing to save, and nothing worth parking
		expect(shell.pill.context?.dirty).toBe(false);
		expect(commitContext()).toBe(false);
		// …but the provider's own actions are already reachable
		expect(shell.pill.context?.extraActions?.map((a) => a.id)).toEqual([
			'scim-enable',
			'delete'
		]);

		await nameField().fill('Corporate Entra ID');

		await vi.waitFor(() => expect(shell.pill.context?.dirty).toBe(true), { timeout: 3000 });
		expect(pillMode()).toBe('context');
		expect(shell.pill.context?.valid).toBe(true);
		expect(shell.pill.context?.commitLabel).toBe(m.common_save());
		// Typing is not saving.
		expect(api.updateIdentityProvider).not.toHaveBeenCalled();
	});

	it('commits the loaded provider with exactly the edited field swapped in', async () => {
		await mount();

		await nameField().fill('Corporate Entra ID');
		// Wait on `dirty`, not on the mode: the mode is 'context' from load now, so
		// it would resolve before the keystroke ever reached the pill.
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

	// REGRESSION: UpdateIdentityProvider is a full replace on the server, so a
	// request that omits defaultRoleId silently CLEARS the stored default role.
	// Saving an edit that never touched JIT must still carry the loaded role.
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

	// The default-role select is fed by ListRoles and commits like every other
	// field: pick a different role, save, and the request carries the new ID.
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

		requestCancelContext(); // dirty → asks first
		expect(shell.pill.cancelPending).toBe(true);
		confirmCancelContext();

		// The bar itself stays — it is the provider's actions, not the edit's — but
		// it goes clean, so Save, Stash and Cancel are gone with the buffer.
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
		// The pill must actually let go — an effect that re-acquires here would
		// make Stash a no-op while the edit is still dirty.
		await vi.waitFor(() => expect(pillMode()).toBe('nav'), { timeout: 3000 });
		expect(shell.drafts.map((d) => d.id)).toEqual([draftId]);
		expect(api.updateIdentityProvider).not.toHaveBeenCalled();

		// Still the mounted surface, so this resumes in place — nothing to navigate.
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

		// The operator walked away: this page's component state is gone, so only
		// what the stash snapshotted can come back.
		setShellPath('/devices');
		expect(restoreDraft(draftId!)).toBe(`/identity-providers/${IDP_ID}`);
		expect(pillMode()).toBe('nav');
		// The card pops on the click; the buffer is staged for the surface, which
		// takes it with claimDraft when it mounts. (The write-only client secret
		// lives nowhere else, so this handoff is the only thing that saves it.)
		expect(shell.drafts).toHaveLength(0);
		const parked = claimDraft(`identity-provider:${IDP_ID}`) as { name: string };
		expect(parked.name).toBe('Corporate Entra ID');
	});
});

describe('identity-provider editor — the provider’s own actions ride the pill', () => {
	// SCIM enable/rotate/disable act on the provider as a whole, so they are pill
	// actions rather than a button column inside the SCIM card. They are still
	// one-shot writes: none of them arms or dirties the form's commit.
	it('enables SCIM from the pill and swaps in the follow-on actions', async () => {
		await mount();
		await vi.waitFor(() => expect(pillMode()).toBe('context'), { timeout: 3000 });
		// The card itself no longer carries the control.
		expect(browser.getByRole('button', { name: m.scim_enable() }).elements()).toHaveLength(0);

		runPillAction('scim-enable');

		await vi.waitFor(() => expect(api.enableSCIM).toHaveBeenCalledWith(IDP_ID), { timeout: 3000 });
		await expect.element(browser.getByText('scim-token-once')).toBeVisible();
		// Enabling changes what the NEXT action can be, and the still-held pill has
		// to say so — offering "Enable SCIM" on an enabled provider would be a lie.
		await vi.waitFor(
			() =>
				expect(shell.pill.context?.extraActions?.map((a) => a.id)).toEqual([
					'scim-rotate',
					'scim-disable',
					'delete'
				]),
			{ timeout: 3000 }
		);
		// A one-shot write is not an edit: nothing to save, nothing to park.
		expect(shell.pill.context?.dirty).toBe(false);
	});

	it('routes Delete through its confirmation, not straight to the RPC', async () => {
		await mount();
		await vi.waitFor(() => expect(pillMode()).toBe('context'), { timeout: 3000 });

		runPillAction('delete');

		// The pill is a shorter route to the gate, never a way past it.
		await expect.element(browser.getByText(m.idp_detail_confirm_delete())).toBeVisible();
		expect(api.deleteIdentityProvider).not.toHaveBeenCalled();

		await browser.getByRole('button', { name: m.common_delete(), exact: true }).click();
		await vi.waitFor(() => expect(api.deleteIdentityProvider).toHaveBeenCalledWith(IDP_ID), {
			timeout: 3000
		});
	});
});
