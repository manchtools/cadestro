

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page as browser, userEvent } from 'vitest/browser';
import * as m from '$lib/paraglide/messages';

const USER_ID = '01JQZZ0000000000000000000A';
const API_TOKEN_ID = 'api-token-1';
const apiToken = () => ({
	id: { value: API_TOKEN_ID },
	name: 'automation',
	createdAt: { seconds: BigInt(Math.floor(Date.now() / 1000) - 60), nanos: 0 },
	expiresAt: { seconds: BigInt(Math.floor(Date.now() / 1000) + 86400), nanos: 0 }
});

const api = vi.hoisted(() => ({
	listIdentityLinks: vi.fn(),
	unlinkIdentity: vi.fn(),
	getCurrentUser: vi.fn(),
	addUserSshKey: vi.fn(),
	removeUserSshKey: vi.fn(),
	rebuildSearchIndex: vi.fn(),
	getServerSettings: vi.fn(),
	updateServerSettings: vi.fn(),
	createApiToken: vi.fn(),
	listApiTokens: vi.fn(),
	revokeApiToken: vi.fn()
}));
const auth = vi.hoisted(() => ({
	granted: new Set<string>(),
	user: {

		id: { value: '01JQZZ0000000000000000000A' },
		email: 'operator@example.test',
		linuxUsername: 'operator',
		roleGrants: [{ role: { name: 'fleet-admin' } }]
	}
}));
const nav = vi.hoisted(() => ({ goto: vi.fn() }));
const tour = vi.hoisted(() => ({ startTour: vi.fn() }));

vi.mock('$lib/sdk', async () => {
	const control = await import('$contract/cadestro/v1/control_pb');
	const common = await import('$contract/cadestro/v1/common_pb');
	const { fetchAllPages } = await import('$lib/sdk/paginate');
	return {
		...control,
		...common,
		apiClient: api,
		fetchAllPages,
		formatTimestampDateTime: () => '2026-08-01 09:00',
		configStore: { serverUrl: 'https://control.test' },
		authStore: {
			get user() {
				return auth.user;
			},
			isAdmin: false,
			hasPermission: (p: string) => auth.granted.has(p),
			logout: vi.fn()
		}
	};
});
vi.mock('$lib/navigation', () => ({ goto: nav.goto, pushState: vi.fn(), replaceState: vi.fn() }));
vi.mock('$lib/onboarding', () => ({ startTour: tour.startTour }));

import SettingsPage from './+page.svelte';

const ALL_PERMISSIONS = [
	'AddUserSshKey:self',
	'RebuildSearchIndex',
	'GetServerSettings',
	'UpdateServerSettings',
	'CreateApiToken',
	'ListApiTokens',
	'RevokeApiToken'
];

beforeEach(() => {
	for (const fn of Object.values(api)) fn.mockReset();
	nav.goto.mockReset();
	tour.startTour.mockReset();
	auth.granted = new Set(ALL_PERMISSIONS);
	api.listIdentityLinks.mockResolvedValue({
		links: [{ id: { value: 'link-1' }, providerName: 'Keycloak', externalEmail: 'operator@idp.test' }]
	});
	api.getCurrentUser.mockResolvedValue({
		sshPublicKeys: [{ id: { value: 'key-1' }, publicKey: 'ssh-ed25519 AAAAC3Nz', comment: 'laptop' }]
	});
	api.getServerSettings.mockResolvedValue({
		settings: { userProvisioningEnabled: false, sshAccessForAll: false }
	});
	api.addUserSshKey.mockResolvedValue({});
	api.removeUserSshKey.mockResolvedValue({});
	api.rebuildSearchIndex.mockResolvedValue(undefined);
	api.updateServerSettings.mockResolvedValue({});
	api.listApiTokens.mockResolvedValue({ tokens: [apiToken()], nextPageToken: '' });
	api.createApiToken.mockResolvedValue({ token: apiToken(), value: 'bearer-secret' });
	api.revokeApiToken.mockResolvedValue(undefined);
	vi.stubGlobal(
		'fetch',
		vi.fn().mockResolvedValue({ ok: true, json: async () => ({ version: '2026.8.1' }) })
	);
});

async function mount() {
	render(SettingsPage);
	await vi.waitFor(() => expect(api.listIdentityLinks).toHaveBeenCalled(), { timeout: 3000 });
}

describe('settings — every capability keeps a home', () => {
	it('renders each section block for a fully permitted operator', async () => {
		await mount();

		for (const heading of [
			m.settings_account(),
			m.settings_appearance(),
			m.settings_ssh_identity(),
			m.settings_api_tokens(),
			m.settings_search_index(),
			m.settings_provisioning(),
			m.settings_server_config(),
			m.common_danger_zone()
		]) {
			await expect.element(browser.getByRole('heading', { name: heading })).toBeVisible();
		}

		await expect.element(browser.getByText('operator@example.test')).toBeVisible();
		await expect.element(browser.getByText('fleet-admin')).toBeVisible();
		await expect.element(browser.getByText('https://control.test')).toBeVisible();
		await expect.element(browser.getByText('operator', { exact: true })).toBeVisible();
		await expect.element(browser.getByText('Keycloak')).toBeVisible();
		await expect.element(browser.getByText('ssh-ed25519 AAAAC3Nz')).toBeVisible();
		await expect.element(browser.getByText('automation', { exact: true })).toBeVisible();
	});

	it('creates an API token with trimmed name and future expiry', async () => {
		await mount();
		await browser.getByRole('button', { name: m.settings_api_tokens_create() }).click();
		await browser.getByLabelText(m.settings_api_tokens_name()).fill('  deploy  ');
		await browser.getByLabelText(m.settings_api_tokens_expiry()).fill('2099-01-01T12:30');
		await browser.getByRole('button', { name: m.settings_api_tokens_create(), exact: true }).last().click();

		await vi.waitFor(() => expect(api.createApiToken).toHaveBeenCalledTimes(1), { timeout: 3000 });
		const [name, expiresAt] = api.createApiToken.mock.calls[0];
		expect(name).toBe('deploy');
		expect(expiresAt).toBeInstanceOf(Date);
		expect(expiresAt.getTime()).toBeGreaterThan(Date.now());
		await expect.element(browser.getByLabelText(m.settings_api_tokens_value())).toHaveValue('bearer-secret');

		await browser.getByRole('button', { name: m.common_cancel(), exact: true }).click();
		await browser.getByRole('button', { name: m.settings_api_tokens_create(), exact: true }).click();
		expect(browser.getByLabelText(m.settings_api_tokens_value()).elements()).toHaveLength(0);
	});

	it('revokes an API token only after confirmation', async () => {
		await mount();
		await browser.getByRole('button', { name: m.settings_api_tokens_revoke() }).click();
		expect(api.revokeApiToken).not.toHaveBeenCalled();

		await browser.getByRole('button', { name: m.settings_api_tokens_revoke(), exact: true }).last().click();
		await vi.waitFor(() => expect(api.revokeApiToken).toHaveBeenCalledWith(API_TOKEN_ID), { timeout: 3000 });
	});

	it('rebuilds the search index through its own RPC', async () => {
		await mount();
		await browser.getByRole('button', { name: m.settings_rebuild_search_index() }).click();
		await vi.waitFor(() => expect(api.rebuildSearchIndex).toHaveBeenCalledTimes(1), { timeout: 3000 });
	});

	it('writes both provisioning switches through UpdateServerSettings', async () => {
		await mount();
		await browser.getByRole('switch', { name: m.settings_user_provisioning() }).click();
		await vi.waitFor(() => expect(api.updateServerSettings).toHaveBeenCalledWith(true, false), {
			timeout: 3000
		});
	});

	it('adds an SSH key with its comment', async () => {
		await mount();
		await browser.getByRole('button', { name: m.settings_ssh_add_key() }).click();
		await browser.getByLabelText(m.settings_ssh_public_key()).fill('ssh-ed25519 NEWKEY');
		await browser.getByLabelText(m.settings_ssh_comment()).fill('workstation');
		await browser.getByRole('button', { name: m.settings_ssh_add_key(), exact: true }).last().click();

		await vi.waitFor(
			() => expect(api.addUserSshKey).toHaveBeenCalledWith(USER_ID, 'ssh-ed25519 NEWKEY', 'workstation'),
			{ timeout: 3000 }
		);
	});

	it('removes an SSH key only after the confirmation', async () => {
		await mount();
		await browser.getByRole('button', { name: m.settings_ssh_remove_key_title() }).click();
		expect(api.removeUserSshKey).not.toHaveBeenCalled();

		await browser.getByRole('button', { name: m.common_delete() }).click();
		await vi.waitFor(() => expect(api.removeUserSshKey).toHaveBeenCalledWith(USER_ID, 'key-1'), {
			timeout: 3000
		});
	});

	it('replays the guided tour from the fleet surface, where its anchors live', async () => {
		await mount();
		await browser.getByRole('button', { name: m.onboarding_restart_tour() }).click();

		await vi.waitFor(() => expect(tour.startTour).toHaveBeenCalledTimes(1), { timeout: 3000 });

		expect(nav.goto).toHaveBeenCalledWith('/devices');
		expect(nav.goto.mock.invocationCallOrder[0]).toBeLessThan(
			tour.startTour.mock.invocationCallOrder[0]
		);
	});

	it('keeps sign-out behind its confirmation dialog', async () => {
		await mount();

		await browser.getByRole('button', { name: m.settings_sign_out(), exact: true }).first().click();
		await expect.element(browser.getByText(m.settings_sign_out_confirm())).toBeVisible();
		await userEvent.keyboard('{Escape}');
	});
});

describe('settings — server-wide capabilities stay permission-gated', () => {
	it('omits the blocks whose RPC the session may not call', async () => {
		auth.granted = new Set();
		await mount();

		expect(browser.getByRole('heading', { name: m.settings_search_index() }).elements()).toHaveLength(0);
		expect(browser.getByRole('heading', { name: m.settings_provisioning() }).elements()).toHaveLength(0);
		expect(browser.getByRole('heading', { name: m.settings_api_tokens() }).elements()).toHaveLength(0);
		expect(browser.getByRole('button', { name: m.settings_ssh_add_key() }).elements()).toHaveLength(0);
		expect(api.getServerSettings).not.toHaveBeenCalled();
		expect(api.getCurrentUser).not.toHaveBeenCalled();
		expect(api.listApiTokens).not.toHaveBeenCalled();
		expect(api.createApiToken).not.toHaveBeenCalled();
		expect(api.revokeApiToken).not.toHaveBeenCalled();

		await expect.element(browser.getByRole('heading', { name: m.settings_account() })).toBeVisible();
		await expect.element(browser.getByRole('heading', { name: m.common_danger_zone() })).toBeVisible();
	});
});
