// Behaviour contract for the SDK's bootstrap-provider path.
//
// The bootstrap-admin URL hands the web a single-use token that the server
// consumes as `Authorization: PowerManage-Bootstrap <T>` — NOT a Bearer
// session token. The one call it may spend the token on is
// CreateIdentityProvider. These tests pin the outgoing wire header on the
// real Connect transport: exactly the PowerManage-Bootstrap scheme, and never
// a Bearer/session token even when a session token happens to be available.
import { describe, it, expect, vi, afterEach } from 'vitest';
import { ApiClient } from '$pmSdk/client';
import { IdentityProviderType } from '$sdk/powermanage/v1/common_pb';

const SERVER_URL = 'https://control.test';

// A distinctive sentinel for the session token. If any code path attached a
// Bearer/session token to the bootstrap call, this string would surface in the
// captured headers and the assertions below would catch it.
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
	clientId: 'power-manage',
	clientSecret: 's3cr3t',
	issuerUrl: 'https://sso.example.com/realms/pm',
	scopes: ['openid', 'profile', 'email']
};

/**
 * Stub global fetch, capture the outgoing request headers, then abort so we
 * never have to fake a valid Connect response body. The header set is captured
 * synchronously at fetch-call time, before the (rejected) response resolves.
 */
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
	it('sends exactly one request carrying the PowerManage-Bootstrap Authorization header', async () => {
		const { headersOf, mock } = captureFetch();
		const client = makeClient();

		await expect(
			client.createIdentityProviderWithBootstrapToken('BOOT-TOKEN-123', providerData)
		).rejects.toBeTruthy();

		expect(mock).toHaveBeenCalledTimes(1);
		expect(headersOf()?.get('authorization')).toBe('PowerManage-Bootstrap BOOT-TOKEN-123');
	});

	it('never attaches a Bearer/session token, even when a session token is available', async () => {
		const { headersOf } = captureFetch();
		// A session token is present, but the bootstrap path must ignore it.
		const client = makeClient(() => SESSION_SENTINEL);

		await expect(
			client.createIdentityProviderWithBootstrapToken('BOOT-TOKEN-123', providerData)
		).rejects.toBeTruthy();

		const auth = headersOf()?.get('authorization');
		expect(auth).toBe('PowerManage-Bootstrap BOOT-TOKEN-123');
		expect(auth).not.toContain('Bearer');
		// The session token must not appear anywhere in the outgoing headers.
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
