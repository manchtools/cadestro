// Svelte context for the two "load from control-plane API" loaders
// that UserPicker + GroupParamsForm need. Using a context (instead of
// threading the loader props through every parent form) keeps the
// per-ActionType form files focused on their own concerns — only the
// orchestrator (action-create-form / edit-params-dialog) knows about
// the apiClient, and every descendant form resolves the loaders
// transparently.
//
// Consumers without the control-plane API (marketplace) simply never
// call setUserLoaders; the getter returns { loadUserActions: undefined,
// loadPlatformUsers: undefined }, and UserPicker / GroupParamsForm
// render empty-state pickers instead of fetching.

import { getContext, setContext } from 'svelte';
import type { ManagedAction } from '$contract/cadestro/v1/control_pb';
import { ActionType } from '$contract/cadestro/v1/actions_pb';
import { apiClient, fetchAllPages } from '$lib/sdk';

export interface UserLite {
	id: string;
	email: string;
	linuxUsername: string;
	disabled?: boolean;
}

export interface UserLoaders {
	loadUserActions?: () => Promise<ManagedAction[]>;
	loadPlatformUsers?: () => Promise<UserLite[]>;
}

const KEY = Symbol('pm.user-loaders');

export function setUserLoaders(loaders: UserLoaders): void {
	setContext(KEY, loaders);
}

export function getUserLoaders(): UserLoaders {
	return getContext<UserLoaders | undefined>(KEY) ?? {};
}

/**
 * Default loaders that hit the Control Server via `apiClient`. Wire these
 * from the action-create-form / edit-params-dialog orchestrators so the
 * inline loader objects don't drift between the two god components (F033).
 */
export const apiUserLoaders: UserLoaders = {
	// F022/F023: page through everything instead of capping silently at 100/200.
	loadUserActions: async () => {
		return fetchAllPages<ManagedAction>(async (pageSize, pageToken) => {
			const r = await apiClient.listActions(pageSize, pageToken, ActionType.USER);
			return { items: r.actions, nextPageToken: r.nextPageToken };
		});
	},
	loadPlatformUsers: async () => {
		return fetchAllPages<UserLite>(async (pageSize, pageToken) => {
			const r = await apiClient.listUsers(pageSize, pageToken);
			return { items: r.users, nextPageToken: r.nextPageToken };
		});
	}
};
