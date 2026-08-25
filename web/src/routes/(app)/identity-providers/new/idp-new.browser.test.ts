

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page as browser } from 'vitest/browser';
import * as m from '$lib/paraglide/messages';

const api = vi.hoisted(() => ({
	createIdentityProvider: vi.fn(),
	listIdentityProviders: vi.fn(),
	deleteIdentityProvider: vi.fn(),
	listRoles: vi.fn()
}));
const drafts = vi.hoisted(() => ({ types: [] as string[] }));
const nav = vi.hoisted(() => ({ url: new URL('https://control.test/identity-providers/new') }));

vi.mock('$lib/sdk', async () => {
	const control = await import('$contract/cadestro/v1/control_pb');
	const common = await import('$contract/cadestro/v1/common_pb');
	const actions = await import('$contract/cadestro/v1/actions_pb');
	return {
		...actions,
		...control,
		...common,
		apiClient: api,
		authStore: { user: { id: '01JQZZ0000000000000000000A' }, hasPermission: () => true },
		configStore: { serverUrl: 'https://control.test' },
		formatTimestamp: () => '2026-08-01',
		formatTimestampDateTime: () => '2026-08-01 09:00',
		fetchAllPages: vi.fn(async () => []),
		useDraft: <T>(type: string, _id: string, initial: T) => {
			drafts.types.push(type);
			let data = initial;
			return {
				get data() {
					return data;
				},
				set data(next: T) {
					data = next;
				},
				update() {},
				clear: async () => {},
				get hasDraft() {
					return false;
				}
			};
		}
	};
});

vi.mock('$app/paths', () => ({ base: '', assets: '' }));
vi.mock('$app/navigation', () => ({
	goto: vi.fn(),
	pushState: vi.fn(),
	replaceState: vi.fn(),
	afterNavigate: vi.fn(),
	beforeNavigate: vi.fn()
}));
vi.mock('$app/state', () => ({
	page: {
		get url() {
			return nav.url;
		},
		params: {}
	}
}));

import { goto } from '$app/navigation';
import { IdentityProviderType } from '$contract/cadestro/v1/common_pb';
import NewIdpPage from './+page.svelte';
import IdpPage from '../+page.svelte';
import StageRail from '$lib/components/shell/stage-rail.svelte';
import {
	shell,
	resetShell,
	setShellPath,
	stashContext,
	commitContext,
	pillMode
} from '$lib/shell/shell.svelte';

const ROUTE = '/identity-providers/new';
const IDP_ID = '01JQZZ4A7K3M9P2Q6R8T1V0W5X';

const ROLE_ADMIN_ID = '00000000000000000000000001';
const ROLE_USER_ID = '00000000000000000000000002';

beforeEach(() => {
	vi.clearAllMocks();
	drafts.types = [];
	resetShell();
	setShellPath(ROUTE);
	nav.url = new URL('https://control.test/identity-providers/new');
	api.createIdentityProvider.mockResolvedValue({ id: IDP_ID, name: 'Keycloak' });
	api.listIdentityProviders.mockResolvedValue({ providers: [] });
	api.listRoles.mockResolvedValue({
		roles: [
			{ id: ROLE_ADMIN_ID, name: 'Admin', description: 'Full access', permissions: [] },
			{ id: ROLE_USER_ID, name: 'User', description: 'Standard access', permissions: [] }
		]
	});
});

const field = (id: string) => document.querySelector<HTMLInputElement>(`#${id}`);

function type(input: HTMLInputElement, value: string) {
	input.value = value;
	input.dispatchEvent(new Event('input', { bubbles: true }));
}

async function fillProvider(over: Partial<Record<string, string>> = {}) {
	await vi.waitFor(() => expect(field('idpName')).toBeTruthy());
	const values: Record<string, string> = {
		idpName: 'Keycloak',
		idpSlug: 'keycloak',
		idpClientId: 'cadestro',
		idpClientSecret: 's3cr3t',
		idpIssuerUrl: 'https://sso.example.com/realms/pm',
		idpScopes: '',
		...over
	};
	for (const [id, value] of Object.entries(values)) type(field(id)!, value);
}

describe('/identity-providers/new — the commit is the pill\'s', () => {
	it('declares a route, which is what earns the Stash button', async () => {
		render(NewIdpPage);
		await fillProvider();
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });

		expect(shell.pill.context?.route).toBe(ROUTE);
		expect(shell.pill.context?.commitLabel).toBe(m.common_create());
	});

	it('never writes the client secret to a persisted draft', async () => {
		render(NewIdpPage);
		await fillProvider();
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });

		expect(drafts.types).toEqual([]);
	});

	it('creates the provider with the exact request the dialog sent', async () => {
		render(NewIdpPage);
		await fillProvider();
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });

		expect(commitContext()).toBe(true);
		await vi.waitFor(() => expect(api.createIdentityProvider).toHaveBeenCalledTimes(1));
		expect(api.createIdentityProvider.mock.calls[0][0]).toEqual({
			name: 'Keycloak',
			slug: 'keycloak',
			providerType: IdentityProviderType.OIDC,
			clientId: 'cadestro',
			clientSecret: 's3cr3t',
			issuerUrl: 'https://sso.example.com/realms/pm',

			scopes: ['openid', 'profile', 'email'],

			autoCreateUsers: false,
			defaultRoleId: ''
		});
		await vi.waitFor(() =>
			expect(vi.mocked(goto).mock.calls[0]?.[0]).toBe(`/identity-providers/${IDP_ID}`)
		);
	});

	it('sends autoCreateUsers=true and the chosen defaultRoleId when JIT is opted into', async () => {
		render(NewIdpPage);
		await fillProvider();
		await vi.waitFor(() => expect(api.listRoles).toHaveBeenCalled(), { timeout: 3000 });

		await browser.getByLabelText(m.idp_field_auto_create_users()).click();
		await browser.getByLabelText(m.idp_field_default_role()).click();
		await browser.getByRole('option', { name: 'User' }).click();
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });

		expect(commitContext()).toBe(true);
		await vi.waitFor(() => expect(api.createIdentityProvider).toHaveBeenCalledTimes(1));
		expect(api.createIdentityProvider.mock.calls[0][0]).toMatchObject({
			autoCreateUsers: true,
			defaultRoleId: ROLE_USER_ID
		});
	});

	it('splits an explicit scopes list the way the dialog did', async () => {
		render(NewIdpPage);
		await fillProvider({ idpScopes: ' openid , groups ,, email ' });
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });

		expect(commitContext()).toBe(true);
		await vi.waitFor(() => expect(api.createIdentityProvider).toHaveBeenCalledTimes(1));
		expect(api.createIdentityProvider.mock.calls[0][0].scopes).toEqual([
			'openid',
			'groups',
			'email'
		]);
	});

	it.each([
		['a malformed slug', { idpSlug: 'Key Cloak' }],
		['a non-URL issuer', { idpIssuerUrl: 'sso.example.com' }],
		['a missing client secret', { idpClientSecret: '' }]
	])('blocks the commit at the STORE on %s', async (_label, over) => {
		render(NewIdpPage);
		await fillProvider();
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });

		await fillProvider(over);
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(false), { timeout: 3000 });
		expect(commitContext()).toBe(false);
		expect(api.createIdentityProvider).not.toHaveBeenCalled();
	});
});

describe('/identity-providers/new — the third exit: stash, walk away, restore', () => {
	it('navigates home and rebuilds a buffer that still commits', async () => {
		const first = await render(NewIdpPage);
		await fillProvider();
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });

		expect(stashContext()).toBe('draft:identity-provider:create');
		expect(shell.drafts[0].route).toBe(ROUTE);
		await new Promise((r) => setTimeout(r, 50));
		expect(pillMode()).toBe('nav');

		await first.unmount();
		setShellPath('/devices');
		const rail = await render(StageRail);
		(document.querySelector('[data-testid="stage-draft"]') as HTMLElement).click();

		await vi.waitFor(() => expect(vi.mocked(goto).mock.calls[0]?.[0]).toBe(ROUTE));
		expect(pillMode()).toBe('nav');

		expect(shell.drafts).toHaveLength(0);
		await rail.unmount();

		setShellPath(ROUTE);
		render(NewIdpPage);

		await vi.waitFor(() => expect(field('idpName')?.value).toBe('Keycloak'));
		expect(field('idpSlug')?.value).toBe('keycloak');
		expect(field('idpClientId')?.value).toBe('cadestro');

		expect(field('idpClientSecret')?.value).toBe('s3cr3t');
		expect(field('idpIssuerUrl')?.value).toBe('https://sso.example.com/realms/pm');
		expect(shell.drafts).toHaveLength(0);

		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });
		expect(commitContext()).toBe(true);
		await vi.waitFor(() => expect(api.createIdentityProvider).toHaveBeenCalledTimes(1));
		expect(api.createIdentityProvider.mock.calls[0][0].clientSecret).toBe('s3cr3t');
	});
});

describe('/identity-providers — the list page hands creation to the route', () => {
	it('navigates instead of opening a dialog', async () => {
		nav.url = new URL('https://control.test/identity-providers');
		render(IdpPage);
		await vi.waitFor(() => expect(api.listIdentityProviders).toHaveBeenCalled(), { timeout: 3000 });

		const create = await vi.waitFor(() => {
			const button = [...document.querySelectorAll<HTMLButtonElement>('button')].find(
				(b) => b.textContent?.trim() === m.idp_create()
			);
			expect(button).toBeTruthy();
			return button!;
		});
		create.click();

		await vi.waitFor(() =>
			expect(vi.mocked(goto).mock.calls[0]?.[0]).toBe('/identity-providers/new')
		);
		expect(document.querySelectorAll('[role="dialog"]')).toHaveLength(0);
	});
});
