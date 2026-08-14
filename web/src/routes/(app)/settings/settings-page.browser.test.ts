// Conversion contract for Settings.
//
// The page was rebuilt as explanation-left / control-right section blocks. A
// re-skin at that size can quietly drop a capability or a permission gate, so
// these tests pin both: every capability still reaches its RPC, and the two
// server-wide blocks stay behind the permission that guards their RPC.
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page as browser, userEvent } from 'vitest/browser';
import * as m from '$lib/paraglide/messages';

const USER_ID = '01JQZZ0000000000000000000A';

const api = vi.hoisted(() => ({
	listIdentityLinks: vi.fn(),
	unlinkIdentity: vi.fn(),
	getCurrentUser: vi.fn(),
	addUserSshKey: vi.fn(),
	removeUserSshKey: vi.fn(),
	rebuildSearchIndex: vi.fn(),
	getServerSettings: vi.fn(),
	updateServerSettings: vi.fn()
}));
const auth = vi.hoisted(() => ({
	granted: new Set<string>(),
	user: {
		// literal, not USER_ID: vi.hoisted() runs before module consts exist
		id: '01JQZZ0000000000000000000A',
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
	return {
		...control,
		...common,
		apiClient: api,
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

const ALL_PERMISSIONS = ['AddUserSshKey:self', 'RebuildSearchIndex', 'GetServerSettings', 'UpdateServerSettings'];

beforeEach(() => {
	for (const fn of Object.values(api)) fn.mockReset();
	nav.goto.mockReset();
	tour.startTour.mockReset();
	auth.granted = new Set(ALL_PERMISSIONS);
	api.listIdentityLinks.mockResolvedValue({
		links: [{ id: 'link-1', providerName: 'Keycloak', externalEmail: 'operator@idp.test' }]
	});
	api.getCurrentUser.mockResolvedValue({
		sshPublicKeys: [{ id: 'key-1', publicKey: 'ssh-ed25519 AAAAC3Nz', comment: 'laptop' }]
	});
	api.getServerSettings.mockResolvedValue({
		settings: { userProvisioningEnabled: false, sshAccessForAll: false }
	});
	api.addUserSshKey.mockResolvedValue({});
	api.removeUserSshKey.mockResolvedValue({});
	api.rebuildSearchIndex.mockResolvedValue(undefined);
	api.updateServerSettings.mockResolvedValue({});
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
			m.settings_search_index(),
			m.settings_provisioning(),
			m.settings_server_config(),
			m.common_danger_zone()
		]) {
			await expect.element(browser.getByRole('heading', { name: heading })).toBeVisible();
		}

		// Evidence rows still read the session and the config store.
		await expect.element(browser.getByText('operator@example.test')).toBeVisible();
		await expect.element(browser.getByText('fleet-admin')).toBeVisible();
		await expect.element(browser.getByText('https://control.test')).toBeVisible();
		await expect.element(browser.getByText('operator', { exact: true })).toBeVisible();
		await expect.element(browser.getByText('Keycloak')).toBeVisible();
		await expect.element(browser.getByText('ssh-ed25519 AAAAC3Nz')).toBeVisible();
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
		// Order matters: the steps resolve against the live DOM, so the navigation
		// has to have happened before the tour is asked to start.
		expect(nav.goto).toHaveBeenCalledWith('/devices');
		expect(nav.goto.mock.invocationCallOrder[0]).toBeLessThan(
			tour.startTour.mock.invocationCallOrder[0]
		);
	});

	it('keeps sign-out behind its confirmation dialog', async () => {
		await mount();
		// exact: the danger zone also holds "Clear Data & Sign Out".
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
		expect(browser.getByRole('button', { name: m.settings_ssh_add_key() }).elements()).toHaveLength(0);
		expect(api.getServerSettings).not.toHaveBeenCalled();
		expect(api.getCurrentUser).not.toHaveBeenCalled();

		// The operator's own blocks survive the loss of every admin permission.
		await expect.element(browser.getByRole('heading', { name: m.settings_account() })).toBeVisible();
		await expect.element(browser.getByRole('heading', { name: m.common_danger_zone() })).toBeVisible();
	});
});
