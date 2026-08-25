import { onMount } from 'svelte';
import { pushState, replaceState } from '$app/navigation';

// URL-as-state primitives. All list pages persist filter / sort /
// pagination state in the URL so that:
//
//   1. Back navigation actually restores the previous filter set —
//      pushState(...) on each committed change makes every filter
//      transition a real history entry.
//   2. Sharing a filtered view by copy-pasting the URL works.
//   3. A page reload preserves the operator's working context.
//
// Hard rules — every page using these helpers MUST follow them or
// risk re-introducing the hydration / infinite-loop classes of bug
// the project's already burned itself on:
//
//   • Never call syncToURL from inside a $effect. Doing so during
//     hydration crashes the SvelteKit router with "Cannot call
//     replaceState(...) before router is initialized" — the effect
//     fires before `goto`/`pushState` are wired up. Bind URL writes
//     to user-interaction callbacks (onValueChange, onclick, form
//     submit) only.
//   • Never use $page.url as a $effect dependency that then writes
//     back via syncToURL. That's an infinite loop: write → URL
//     change → effect re-runs → write again. URL writes go in event
//     handlers; URL reads happen at init and inside an
//     afterNavigate('popstate') callback to handle back/forward.
//   • Use pushState for *committed* changes (multi-select close,
//     date-picker confirm, sort header click). Use replaceState for
//     *transient* ones (debounced search input keystrokes) so back
//     goes to the last intentional state, not the last typed
//     character.

export type Codec<T> = {
	/** Parse a URL search-param value (or null when absent) into the typed value. */
	parse(param: string | null): T;
	/** Serialize a typed value to a string. Return null to omit the param entirely
	 *  (typically when the value equals its default — keeps URLs short). */
	serialize(value: T): string | null;
};

export const codecs = {
	string(defaultValue = ''): Codec<string> {
		return {
			parse: (p) => p ?? defaultValue,
			serialize: (v) => (v === defaultValue ? null : v)
		};
	},

	/** Comma-separated list. Empty list omits the param. */
	stringArray(defaultValue: string[] = []): Codec<string[]> {
		const def = defaultValue.join(',');
		return {
			parse: (p) => {
				if (p === null) return [...defaultValue];
				if (p === '') return [];
				return p.split(',').filter((s) => s.length > 0);
			},
			serialize: (v) => {
				const s = v.join(',');
				return s === def ? null : s;
			}
		};
	},

	bool(defaultValue = false): Codec<boolean> {
		return {
			parse: (p) => {
				if (p === null) return defaultValue;
				return p === '1' || p === 'true';
			},
			serialize: (v) => (v === defaultValue ? null : v ? '1' : '0')
		};
	},

	int(defaultValue = 0): Codec<number> {
		return {
			parse: (p) => {
				if (p === null) return defaultValue;
				const n = Number(p);
				return Number.isFinite(n) ? n : defaultValue;
			},
			serialize: (v) => (v === defaultValue ? null : String(v))
		};
	},

	/** Parameter restricted to a known set; falls back to defaultValue on bad input. */
	enum<T extends string>(allowed: readonly T[], defaultValue: T): Codec<T> {
		const set = new Set<string>(allowed);
		return {
			parse: (p) => (p !== null && set.has(p) ? (p as T) : defaultValue),
			serialize: (v) => (v === defaultValue ? null : v)
		};
	}
};

/** Read a URL param synchronously. Use at component init (with the URL from `$page`)
 *  and inside an afterNavigate('popstate') callback to re-sync on back/forward. */
export function readURLParam<T>(url: URL, key: string, codec: Codec<T>): T {
	return codec.parse(url.searchParams.get(key));
}

/** Sync one or more params to the URL in a single history operation.
 *  Must be called from a user-interaction callback (NOT from a $effect — see header).
 *  Default mode is 'push' (history entry, back-nav restores prior filter set);
 *  pass 'replace' for transient updates that shouldn't bloat history.
 *
 */
export type AnyCodecEntry = [string, unknown, Codec<unknown>];

export function syncToURL(updates: AnyCodecEntry[], mode: 'push' | 'replace' = 'push') {
	if (typeof window === 'undefined') return;
	const url = new URL(window.location.href);
	for (const [key, value, codec] of updates) {
		const serialized = codec.serialize(value);
		if (serialized === null) {
			url.searchParams.delete(key);
		} else {
			url.searchParams.set(key, serialized);
		}
	}
	// URLSearchParams percent-encodes commas as %2C — RFC 3986 says they're
	// reserved query delimiters, but list-style filters like
	// `?types=3,200,400` are far more readable unencoded and every browser
	// accepts both forms in the address bar. Reading via
	// URLSearchParams.get() decodes them back transparently, so the
	// round-trip works either way.
	const queryString = url.searchParams.toString().replaceAll('%2C', ',');
	const target = url.pathname + (queryString ? '?' + queryString : '') + url.hash;
	if (mode === 'push') {
		pushState(target, {});
	} else {
		replaceState(target, {});
	}
}

/** Convenience wrapper for the single-key case. */
export function syncOneToURL<T>(
	key: string,
	value: T,
	codec: Codec<T>,
	mode: 'push' | 'replace' = 'push'
) {
	syncToURL([[key, value, codec]], mode);
}

/** Subscribe to browser back/forward — call from a component's top-level
 *  script to re-sync local state from the URL when the user navigates
 *  through history.
 *
 *  Why not afterNavigate('popstate')? SvelteKit's afterNavigate is keyed
 *  to load-function navigation. Shallow pushState/replaceState updates
 *  (what syncToURL uses to keep filters in the URL) don't re-run load,
 *  and back/forward between two shallow-pushed states often doesn't fire
 *  afterNavigate at all — the path didn't change, only the query string
 *  did. The native popstate event is the source of truth here: it fires
 *  for every history pop, shallow or not.
 *
 *  Pass a handler that reads from the supplied URL (already constructed
 *  from window.location at the time popstate fires) and assigns to your
 *  local $state variables. Don't write back to the URL from inside this
 *  handler — that would re-enter the back/forward loop. */
export function onPopstate(handler: (url: URL) => void) {
	onMount(() => {
		const fn = () => handler(new URL(window.location.href));
		window.addEventListener('popstate', fn);
		return () => window.removeEventListener('popstate', fn);
	});
}
