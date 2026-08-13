// The dev sign-in seed has two paths: a REAL session fetched from a devauth
// control server (POST /dev/session), and the offline fake session used when no
// such server answers. Both must land an admin session the (app) auth gate
// accepts — that gate is the whole point of the bypass.
import { describe, expect, it, vi, afterEach } from 'vitest';
import { seedDevSession, seedFakeSession } from './skip-auth';
import { authStore, configStore } from '$lib/sdk';

// The module bails out under SSR, so the browser path needs `browser` true and
// a `window` to read the origin from.
vi.mock('$app/environment', () => ({ browser: true }));

const ORIGIN = 'https://localhost:5173';

// The unit project runs in node. The SDK's config store gates its persistence on
// `typeof window !== 'undefined'` (wrappers.svelte.ts), so pretending to be a
// browser means also providing the storage that gate implies — otherwise the
// store reaches for a sessionStorage node does not have.
function stubBrowser() {
	const store = new Map<string, string>();
	const storage = {
		getItem: (k: string) => store.get(k) ?? null,
		setItem: (k: string, v: string) => void store.set(k, v),
		removeItem: (k: string) => void store.delete(k),
		clear: () => store.clear()
	};
	vi.stubGlobal('sessionStorage', storage);
	vi.stubGlobal('localStorage', storage);
	vi.stubGlobal('window', { location: { origin: ORIGIN }, sessionStorage: storage, localStorage: storage });
}

afterEach(() => {
	vi.unstubAllGlobals();
	vi.restoreAllMocks();
});

describe('dev skip-auth seed', () => {
	it('produces an admin session the (app) auth gate accepts', () => {
		seedFakeSession();
		expect(configStore.isConfigured).toBe(true);
		expect(authStore.isAuthenticated).toBe(true);
		expect(authStore.isAdmin).toBe(true);
	});

	it('prefers the REAL session from /dev/session, so every RPC works and not just the shell', async () => {
		const fetchMock = vi.fn().mockResolvedValue({
			ok: true,
			json: async () => ({
				accessToken: 'real-access-token',
				refreshToken: 'real-refresh-token',
				expiresAt: new Date(Date.now() + 5 * 60_000).toISOString(),
				userId: '01KZ9D3N0NH8ZHA41WPDDFE76D',
				email: 'dev-admin@localhost',
				displayName: 'Dev Admin'
			})
		});
		stubBrowser();
		vi.stubGlobal('fetch', fetchMock);

		await seedDevSession();

		// It talks to THIS origin: vite proxies the control paths, so the browser
		// never faces control's self-signed certificate.
		expect(fetchMock).toHaveBeenCalledWith(`${ORIGIN}/dev/session`, { method: 'POST' });
		expect(authStore.accessToken).toBe('real-access-token');
		expect(authStore.isAuthenticated).toBe(true);
		expect(authStore.isAdmin).toBe(true);
	});

	it('falls back to the offline fake session when no devauth server answers', async () => {
		stubBrowser();
		vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('connection refused')));
		vi.spyOn(console, 'debug').mockImplementation(() => {});

		await seedDevSession();

		// Serverless UI dev still works: an admin session, just not a real token.
		expect(authStore.accessToken).toBe('dev-skip-auth');
		expect(authStore.isAuthenticated).toBe(true);
		expect(authStore.isAdmin).toBe(true);
	});
});
