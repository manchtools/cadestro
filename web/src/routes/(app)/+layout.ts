// Client-side auth gate for the (app) route group. F042.
//
// Previously the (app) +layout.svelte ran the configured/authenticated
// check inside `onMount`, which means the layout shell, sidebar, and
// nav structure all rendered before the redirect fired — a flash of
// nav for unauthenticated reloads, plus a brief leak of the menu
// structure to logged-out users.
//
// Moving the check into `+layout.ts` `load` runs it before the layout
// component is constructed. Returning a redirect throw is the
// canonical SvelteKit primitive for this. We import the auth/config
// stores directly because this is a pure-CSR SPA (no SvelteKit
// `server.ts` is used) — the wrappers are framework-free TS state.

import { browser } from '$app/environment';
import { redirect } from '@sveltejs/kit';
import { authStore, configStore } from '$lib/sdk';
import { base } from '$app/paths';

// CSR-only: this entire app runs on the client. ssr is disabled by the
// adapter, so `browser` is true on every load — but we keep the guard
// to short-circuit any future SSR pre-render.
export const ssr = false;

export const load = ({ url }) => {
	if (!browser) return {};

	if (!configStore.isConfigured) {
		throw redirect(307, `${base}/setup`);
	}

	if (!authStore.isAuthenticated) {
		// Preserve the originally-requested path so the user lands back here
		// after a successful login.
		const currentPath = url.pathname.replace(base, '') + url.search;
		throw redirect(
			307,
			`${base}/login?redirect=${encodeURIComponent(currentPath)}`
		);
	}

	return {};
};
