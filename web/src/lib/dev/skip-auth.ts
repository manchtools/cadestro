// TEMPORARY dev-only auth bypass: `VITE_SKIP_AUTH=1 npm run dev` signs the UI
// in without walking OIDC. It prefers a REAL admin session from a devauth
// control server (`go build -tags devauth`, run with CADESTRO_DEV_AUTH=1 and the
// same CADESTRO_DEV_AUTH_TOKEN as Vite)
// via POST /dev/session, so every RPC works — not just the shell. When no such
// server answers it falls back to a fake in-memory admin session, preserving
// serverless UI dev (RPCs then fail; pages render their empty/error states).
// The top-level guard is compile-time — `import.meta.env.DEV` is statically
// false in production builds, so the activation branch is eliminated and the
// env flag alone can never enable it there.
// Remove this module and its import in src/routes/+layout.svelte when done.
import { browser } from '$app/environment';
import { create } from '@bufbuild/protobuf';
import {
	authStore,
	configStore,
	UserSchema,
	RoleSchema,
	RoleGrantSchema
} from '$lib/sdk';

/** Stable bootstrap-admin role ID (server/internal/auth/reconcile.go); the
 *  auth store's `isAdmin` keys off exactly this grant. */
const ADMIN_ROLE_ID = '00000000000000000000000001';

/** The offline seed: a fake admin session for UI work with no control server.
 *  Exported so its contract — a user whose grants satisfy the (app) auth gate —
 *  can be asserted directly, without a window or a server. */
export function seedFakeSession() {
	if (!configStore.isConfigured) configStore.serverUrl = 'http://localhost:8080';
	const user = create(UserSchema, {
		id: '01DEVSKIPAUTHADMIN00000001',
		email: 'dev@localhost',
		displayName: 'Dev Admin',
		roleGrants: [
			create(RoleGrantSchema, {
				role: create(RoleSchema, { id: ADMIN_ROLE_ID, name: 'Administrator', isSystem: true })
			})
		]
	});
	// Expiry must stay inside the 2^31-1 ms setTimeout cap: a far-future date
	// (e.g. 2099) overflows the refresh timer and fires an immediate, failing
	// refresh. 20 days keeps the timer real and dormant; the seed re-runs on
	// every page load anyway while the flag is set.
	const expiresAt = new Date(Date.now() + 20 * 24 * 60 * 60 * 1000);
	authStore.setAuth('dev-skip-auth', 'dev-skip-auth', expiresAt, user);
}

export async function seedDevSession() {
	// The browser reaches control through THIS dev origin — vite.config.ts
	// proxies the control paths to it — so we point the SDK at our own origin
	// and never face control's self-signed cert. SSR has no window and makes no
	// real calls, so bail there.
	if (!browser) return;
	configStore.serverUrl = window.location.origin;
	// Prefer a REAL session so every RPC works. The proxied /dev/session only
	// answers when control is a devauth build run with CADESTRO_DEV_AUTH=1 and Vite
	// carries the matching CADESTRO_DEV_AUTH_TOKEN; otherwise
	// (or with no server) we fall back to the offline fake session.
	try {
		const res = await fetch(`${configStore.serverUrl}/dev/session`, { method: 'POST' });
		if (!res.ok) throw new Error(`dev session HTTP ${res.status}`);
		const s = await res.json();
		const user = create(UserSchema, {
			id: s.userId,
			email: s.email,
			displayName: s.displayName,
			roleGrants: [
				create(RoleGrantSchema, {
					role: create(RoleSchema, { id: ADMIN_ROLE_ID, name: 'Administrator', isSystem: true })
				})
			]
		});
		authStore.setAuth(s.accessToken, s.refreshToken, new Date(s.expiresAt), user);
		return;
	} catch (err) {
		console.debug('dev skip-auth: /dev/session unavailable, using offline fake session', err);
		seedFakeSession();
	}
}

if (import.meta.env.DEV && import.meta.env.VITE_SKIP_AUTH === '1') seedDevSession();
