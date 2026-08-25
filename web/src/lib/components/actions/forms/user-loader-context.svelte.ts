

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

export const apiUserLoaders: UserLoaders = {

	loadUserActions: async () => {
		return fetchAllPages<ManagedAction>(async (pageSize, pageToken) => {
			const r = await apiClient.listActions(pageSize, pageToken, ActionType.USER);
			return { items: r.actions, nextPageToken: r.nextPageToken };
		});
	},
	loadPlatformUsers: async () => {
		return fetchAllPages<UserLite>(async (pageSize, pageToken) => {
			const r = await apiClient.listUsers(pageSize, pageToken);
			return { items: r.users.map((user) => ({ ...user, id: user.id?.value ?? '' })), nextPageToken: r.nextPageToken };
		});
	}
};
