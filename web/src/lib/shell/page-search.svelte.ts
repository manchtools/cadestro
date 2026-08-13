// The page ↔ pill search seam.
//
// A list page owns a query (its `createSearchListState` / `createClientListState`
// instance). ⌘K opens the pill's search mode, and the operator expects to be
// searching WHAT THEY ARE LOOKING AT — not a global palette that navigates away.
// So a list page publishes its query here on mount and withdraws it on unmount;
// the pill chrome reads the registration to preselect a leading facet and to
// route keystrokes into the page's own list state.
//
// Module state only, with no RPC/auth import: this file is part of the
// chrome-stays-API-free slice (`no-api-import.test.ts`), which is why the scope
// is a bare SearchScope NUMBER rather than the generated enum type — the
// registering route owns the contract types, the chrome only forwards them.
import { untrack } from 'svelte';

export interface PageSearchRegistration {
	/** The `SearchScope` value this page's list reads, or `null` for pages served
	 *  by a plain list RPC (roles, tokens, identity providers, my devices,
	 *  terminal sessions) that the Search RPC has no scope for. */
	scope: number | null;
	/** The page's own name — the label of the leading facet chip. A message
	 *  accessor, called at render time so a locale switch reaches the pill. */
	label: () => string;
	/** The page's live query. A getter, so the pill seeds from current state. */
	readonly query: string;
	/** Route a keystroke into the page's list (the page's own `setSearch`). */
	setQuery(value: string): void;
	/** Drop the page's query without closing the pill. */
	clear(): void;
}

// `$state.raw`, for two reasons that both bite hard:
//
//   * plain `$state` DEEP-PROXIES the object, so the identity check in the
//     release below would compare a proxy with its target — never equal, nothing
//     would ever be withdrawn, and every page would inherit the previous page's
//     scope. (Svelte says so in dev: `state_proxy_equality_mismatch`.)
//   * raw assignment is a pure WRITE. A version counter bumped with `count++`
//     reads the signal it writes, and pages register from inside an `$effect` —
//     so that effect would depend on its own write and re-run forever
//     (`effect_update_depth_exceeded`), taking the whole list page down with it,
//     not just the search.
//
// Callers therefore also get the REAL object back and can compare by identity.
let entry = $state.raw<PageSearchRegistration | null>(null);

/**
 * Publish this page as the active search scope. Returns the release function,
 * so the canonical call site is an effect whose cleanup withdraws it:
 *
 * ```ts
 * $effect(() => registerPageSearch({ scope, label, get query() {…}, … }));
 * ```
 *
 * The release is identity-guarded: if the next page registered before this one
 * unmounted, withdrawing must not blank out the newcomer's registration.
 */
export function registerPageSearch(next: PageSearchRegistration): () => void {
	entry = next;
	return () => {
		// Untracked: the teardown must not make the registering effect depend on
		// the very signal that effect writes.
		if (untrack(() => entry) !== next) return; // a newer page already owns it
		entry = null;
	};
}

/** The registration of the currently mounted list page, or `null`. */
export function activePageSearch(): PageSearchRegistration | null {
	return entry;
}

/** Test/reset seam — no page is registered in a pristine shell. */
export function resetPageSearch() {
	entry = null;
}
