

import { apiClient } from '$lib/sdk';
import type { ManagedAction } from '$contract/cadestro/v1/control_pb';

export const PAGE_SIZE = 100;

export const MAX_PAGES = 20;

export interface LibrarySnapshot {
	actions: ManagedAction[];

	total: number;
	truncated: boolean;
}

export async function loadLibrary(): Promise<LibrarySnapshot> {
	const actions: ManagedAction[] = [];

	const seen = new Set<string>();
	let total = 0;
	let token = '';
	for (let i = 0; i < MAX_PAGES; i++) {
		const resp = await apiClient.listActions(PAGE_SIZE, token);
		for (const a of resp.actions) {
			if (seen.has((a.id?.value ?? ''))) continue;
			seen.add((a.id?.value ?? ''));
			actions.push(a);
		}
		total = resp.totalCount || actions.length;
		token = resp.nextPageToken;
		if (!token) return { actions, total, truncated: false };
	}
	return { actions, total, truncated: true };
}
