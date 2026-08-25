// The action-library snapshot: the RPC read behind the overview zoom level.
//
// The overview draws the WHOLE library, so it cannot ride the paged Search list
// — that list is one page of at most 100 rows and its shape says nothing about
// the library. It sweeps ListActions instead (typed ManagedAction rows carrying
// `type`, `desired_state` and `params.shell.is_compliance`, which is every field
// the overview derives from) and pairs it with the server's own total_count.
//
// TRADEOFF, stated rather than hidden: this is a 1..MAX_PAGES round-trip sweep,
// so it is LAZY — nothing is fetched until the operator asks for the overview,
// and the list zoom (the default) never pays for it. The sweep is bounded, so
// one page load can never become an unbounded RPC storm, and when the bound is
// hit the surface says it is showing a partial library instead of implying the
// tiles are everything.
import { apiClient } from '$lib/sdk';
import type { ManagedAction } from '$contract/cadestro/v1/control_pb';

/** ListActions caps page_size at 100 (control.proto: `validate:"…,lte=100"`). */
export const PAGE_SIZE = 100;
/** Bound on the sweep — at most 2 000 actions in one overview. */
export const MAX_PAGES = 20;

export interface LibrarySnapshot {
	actions: ManagedAction[];
	/** Server-reported library size, even when the sweep stopped early. */
	total: number;
	truncated: boolean;
}

export async function loadLibrary(): Promise<LibrarySnapshot> {
	const actions: ManagedAction[] = [];
	// A paged sweep can see the same row twice if rows are created or deleted
	// between pages. Counting an action twice would inflate every number on the
	// overview, and a repeated ULID would crash the keyed tile grid outright —
	// so the id is deduplicated where the server's answer enters, once, rather
	// than left for each derivation to remember.
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
