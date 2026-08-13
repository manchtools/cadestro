// Behaviour contract for /setup as a bootstrap onboarding flow.
//
// `control bootstrap-admin` prints <origin>/setup#bootstrap_token=<T>. The web
// reads the token from the fragment, and — after the server URL is known —
// spends it on exactly ONE CreateIdentityProvider call under the
// PowerManage-Bootstrap scheme. The token is memory-only: never persisted,
// never turned into a session. These tests pin the fragment read, the
// server→provider step transition that must not lose the token, the single
// bootstrap call (and the absence of the Bearer path), success cleanup, the
// spent/expired-token error, and the no-storage guarantee.
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { ConnectError, Code } from '@connectrpc/connect';
import * as m from '$lib/paraglide/messages';

const TOKEN = 'BOOTSTRAP-TOKEN-XYZ';

// The fixed system-role IDs. The bootstrap page may NOT call ListRoles (the
// token buys exactly one RPC), so its role select is hardwired to these.
const ROLE_USER_ID = '00000000000000000000000002';

const api = vi.hoisted(() => ({
	createIdentityProviderWithBootstrapToken: vi.fn(),
	// The Bearer/session path — MUST NOT be called during bootstrap.
	createIdentityProvider: vi.fn()
}));
const cfg = vi.hoisted(() => ({ serverUrl: 'https://control.test' }));
const navi = vi.hoisted(() => ({ goto: vi.fn() }));

vi.mock('$lib/sdk', async () => {
	const common = await import('$sdk/powermanage/v1/common_pb');
	return {
		...common,
		apiClient: api,
		configStore: {
			get serverUrl() {
				return cfg.serverUrl;
			},
			set serverUrl(v: string) {
				cfg.serverUrl = v;
			},
			get isConfigured() {
				return cfg.serverUrl.length > 0;
			}
		}
	};
});

vi.mock('$lib/navigation', () => ({
	goto: (path: string) => navi.goto(path),
	pushState: vi.fn(),
	replaceState: vi.fn()
}));

// No real /health probing or page reload in tests.
vi.mock('$lib/version', () => ({ checkAndSwitchVersion: vi.fn(async () => false) }));

import SetupPage from './+page.svelte';

const field = (id: string) => document.querySelector<HTMLInputElement>(`#${id}`);

function type(input: HTMLInputElement, value: string) {
	input.value = value;
	input.dispatchEvent(new Event('input', { bubbles: true }));
}

const button = (label: string) =>
	[...document.querySelectorAll<HTMLButtonElement>('button')].find(
		(b) => b.textContent?.trim() === label
	);

async function fillProvider(over: Partial<Record<string, string>> = {}) {
	await vi.waitFor(() => expect(field('idpName')).toBeTruthy());
	const values: Record<string, string> = {
		idpName: 'Keycloak',
		idpSlug: 'keycloak',
		idpClientId: 'power-manage',
		idpClientSecret: 's3cr3t',
		idpIssuerUrl: 'https://sso.example.com/realms/pm',
		idpScopes: '',
		...over
	};
	for (const [id, value] of Object.entries(values)) type(field(id)!, value);
}

async function submitProvider() {
	const b = await vi.waitFor(() => {
		const btn = button(m.setup_bootstrap_submit());
		expect(btn).toBeTruthy();
		expect(btn!.disabled).toBe(false);
		return btn!;
	});
	b.click();
}

function setHashToken(token: string) {
	window.location.hash = `bootstrap_token=${token}`;
}

beforeEach(() => {
	vi.clearAllMocks();
	cfg.serverUrl = 'https://control.test';
	api.createIdentityProviderWithBootstrapToken.mockResolvedValue({ id: 'IDP1', slug: 'keycloak' });
	localStorage.clear();
	sessionStorage.clear();
	setHashToken(TOKEN);
});

afterEach(() => {
	window.location.hash = '';
	localStorage.clear();
	sessionStorage.clear();
});

describe('/setup — the bootstrap token onboarding flow', () => {
	// AC1
	it('reads the token from the fragment and carries it across the server-URL step into the provider step', async () => {
		// Server URL not yet known: the server step comes first.
		cfg.serverUrl = '';
		render(SetupPage);

		// Step 1: the server-URL form, not the provider form.
		await vi.waitFor(() => expect(field('serverUrl')).toBeTruthy());
		expect(field('idpName')).toBeFalsy();

		type(field('serverUrl')!, 'https://control.test');
		const cont = await vi.waitFor(() => {
			const b = button(m.setup_continue());
			expect(b).toBeTruthy();
			return b!;
		});
		cont.click();

		// Step 2: advancing must reveal the provider form WITHOUT a route change
		// that would drop the fragment.
		await fillProvider();
		await submitProvider();

		// The token that reaches the single call is the one parsed from the
		// fragment before the step transition.
		await vi.waitFor(() =>
			expect(api.createIdentityProviderWithBootstrapToken).toHaveBeenCalledTimes(1)
		);
		expect(api.createIdentityProviderWithBootstrapToken.mock.calls[0][0]).toBe(TOKEN);
	});

	// AC3
	it('issues exactly one bootstrap CreateIdentityProvider call and never the Bearer path', async () => {
		render(SetupPage);
		await fillProvider();
		await submitProvider();

		await vi.waitFor(() =>
			expect(api.createIdentityProviderWithBootstrapToken).toHaveBeenCalledTimes(1)
		);
		const [token, data] = api.createIdentityProviderWithBootstrapToken.mock.calls[0];
		expect(token).toBe(TOKEN);
		expect(data).toMatchObject({
			name: 'Keycloak',
			slug: 'keycloak',
			clientId: 'power-manage',
			clientSecret: 's3cr3t',
			issuerUrl: 'https://sso.example.com/realms/pm',
			// An empty scopes box still sends the provider form's three defaults.
			scopes: ['openid', 'profile', 'email'],
			// JIT bootstrap defaults: the first sign-in creates the account, and
			// it receives the least-privilege system User role.
			autoCreateUsers: true,
			defaultRoleId: ROLE_USER_ID
		});
		// The ordinary (session/Bearer) creation path is never used here.
		expect(api.createIdentityProvider).not.toHaveBeenCalled();
	});

	// The JIT defaults ride the single call untouched: auto-create ON and the
	// preselected system User role — chosen without any roles lookup, because
	// the spent-once token cannot pay for a second RPC.
	it('sends autoCreateUsers=true and the preselected User role by default', async () => {
		render(SetupPage);
		await fillProvider();
		await submitProvider();

		await vi.waitFor(() =>
			expect(api.createIdentityProviderWithBootstrapToken).toHaveBeenCalledTimes(1)
		);
		const [, data] = api.createIdentityProviderWithBootstrapToken.mock.calls[0];
		expect(data.autoCreateUsers).toBe(true);
		expect(data.defaultRoleId).toBe(ROLE_USER_ID);
	});

	// AC4
	it('on success strips the token from the URL fragment and navigates to /login', async () => {
		const replaceSpy = vi.spyOn(window.history, 'replaceState');
		render(SetupPage);
		await fillProvider();
		await submitProvider();

		await vi.waitFor(() => expect(navi.goto).toHaveBeenCalledWith('/login'));

		// The fragment (and thus the token) is scrubbed from the address bar.
		expect(replaceSpy).toHaveBeenCalled();
		expect(window.location.hash).toBe('');
	});

	// AC5
	it('shows a re-run message and does NOT retry when the token is spent/expired', async () => {
		api.createIdentityProviderWithBootstrapToken.mockRejectedValue(
			new ConnectError('invalid or expired bootstrap token', Code.Unauthenticated)
		);
		render(SetupPage);
		await fillProvider();
		await submitProvider();

		const alert = await vi.waitFor(() => {
			const el = document.querySelector('[role="alert"]');
			expect(el).toBeTruthy();
			return el!;
		});
		expect(alert.textContent).toContain(m.setup_bootstrap_token_rejected());

		// Exactly one attempt — the spent token is never retried automatically.
		expect(api.createIdentityProviderWithBootstrapToken).toHaveBeenCalledTimes(1);
		expect(navi.goto).not.toHaveBeenCalledWith('/login');
	});

	// AC6
	it('never writes the bootstrap token to localStorage or sessionStorage', async () => {
		render(SetupPage);
		await fillProvider();

		const scan = () => {
			const entries: string[] = [];
			for (const store of [localStorage, sessionStorage]) {
				for (let i = 0; i < store.length; i++) {
					const key = store.key(i)!;
					entries.push(key, store.getItem(key) ?? '');
				}
			}
			return entries.join(' ');
		};

		// Before submit: the token is held in memory only.
		expect(scan()).not.toContain(TOKEN);

		await submitProvider();
		await vi.waitFor(() =>
			expect(api.createIdentityProviderWithBootstrapToken).toHaveBeenCalledTimes(1)
		);

		// After the flow completes: still nowhere in web storage.
		expect(scan()).not.toContain(TOKEN);
	});
});
