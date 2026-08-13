import { goto as kitGoto, pushState as kitPushState, replaceState as kitReplaceState } from '$app/navigation';
import { base } from '$app/paths';

export function goto(path: string, opts?: Parameters<typeof kitGoto>[1]) {
	return kitGoto(`${base}${path}`, opts);
}

/**
 * Wrapper around SvelteKit `pushState` that prepends the configured `base`
 * path. Use this for shallow routing so URLs are correct under non-empty
 * `BASE_PATH` deployments.
 */
export function pushState(path: string, state: App.PageState) {
	return kitPushState(`${base}${path}`, state);
}

/**
 * Wrapper around SvelteKit `replaceState` that prepends the configured `base`
 * path. Same caveats as `pushState`. Note: `replaceState` must NOT be called
 * inside `$effect` — see project memory.
 */
export function replaceState(path: string, state: App.PageState) {
	return kitReplaceState(`${base}${path}`, state);
}
