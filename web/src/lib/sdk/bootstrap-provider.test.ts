

import { describe, it, expect, vi, afterEach } from 'vitest';
import { ApiClient } from '$contractClient/client';
import { IdentityProviderType } from '$contract/cadestro/v1/common_pb';

const SERVER_URL = 'https://control.test';

const SESSION_SENTINEL = 'SESSION-SENTINEL-MUST-NOT-APPEAR';

function makeClient(getAccessToken: () => string | null = () => null): ApiClient {
	return new ApiClient({
		getServerUrl: () => SERVER_URL,
		getAccessToken,
		getRefreshToken: () => null,
		ensureValidToken: async () => {},
		refreshToken: async () => false,
		onUnauthenticated: () => {},
		onAuthResponse: () => {},
		onUserUpdated: () => {}
	});
}

const providerData = {
	name: 'Keycloak',
	slug: 'keycloak',
	providerType: IdentityProviderType.OIDC,
	clientId: 'cadestro',
	clientSecret: 's3cr3t',
	issuerUrl: 'https://sso.example.com/realms/pm',
	scopes: ['openid', 'profile', 'email']
};

function captureFetch(): { headersOf: () => Headers | undefined; mock: ReturnType<typeof vi.fn> } {
	let captured: Headers | undefined;
	const mock = vi.fn(async (_url: unknown, init: RequestInit) => {
		captured = new Headers(init.headers as HeadersInit);
		throw new Error('captured — no response needed');
	});
	vi.stubGlobal('fetch', mock);
	return { headersOf: () => captured, mock };
}

afterEach(() => {
	vi.unstubAllGlobals();
	vi.restoreAllMocks();
});

describe('ApiClient.createIdentityProviderWithBootstrapToken', () => {
	it('sends exactly one request carrying the Cadestro-Bootstrap Authorization header', async () => {
		const { headersOf, mock } = captureFetch();
		const client = makeClient();

		await expect(
			client.createIdentityProviderWithBootstrapToken('BOOT-TOKEN-123', providerData)
		).rejects.toBeTruthy();

		expect(mock).toHaveBeenCalledTimes(1);
		expect(headersOf()?.get('authorization')).toBe('Cadestro-Bootstrap BOOT-TOKEN-123');
	});

	it('never attaches a Bearer/session token, even when a session token is available', async () => {
		const { headersOf } = captureFetch();

		const client = makeClient(() => SESSION_SENTINEL);

		await expect(
			client.createIdentityProviderWithBootstrapToken('BOOT-TOKEN-123', providerData)
		).rejects.toBeTruthy();

		const auth = headersOf()?.get('authorization');
		expect(auth).toBe('Cadestro-Bootstrap BOOT-TOKEN-123');
		expect(auth).not.toContain('Bearer');

		const all = JSON.stringify([...(headersOf()?.entries() ?? [])]);
		expect(all).not.toContain(SESSION_SENTINEL);
	});

	it('targets the configured control server (reuses the existing transport base URL)', async () => {
		const { mock } = captureFetch();
		const client = makeClient();

		await expect(
			client.createIdentityProviderWithBootstrapToken('BOOT-TOKEN-123', providerData)
		).rejects.toBeTruthy();

		const url = String(mock.mock.calls[0][0]);
		expect(url.startsWith(SERVER_URL)).toBe(true);
	});
});
