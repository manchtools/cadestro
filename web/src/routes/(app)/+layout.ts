

import { browser } from '$app/environment';
import { redirect } from '@sveltejs/kit';
import { authStore, configStore } from '$lib/sdk';
import { base } from '$app/paths';

export const ssr = false;

export const load = ({ url }) => {
	if (!browser) return {};

	if (!configStore.isConfigured) {
		throw redirect(307, `${base}/setup`);
	}

	if (!authStore.isAuthenticated) {

		const currentPath = url.pathname.replace(base, '') + url.search;
		throw redirect(
			307,
			`${base}/login?redirect=${encodeURIComponent(currentPath)}`
		);
	}

	return {};
};
