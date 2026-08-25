import { goto as kitGoto, pushState as kitPushState, replaceState as kitReplaceState } from '$app/navigation';
import { base } from '$app/paths';

export function goto(path: string, opts?: Parameters<typeof kitGoto>[1]) {
	return kitGoto(`${base}${path}`, opts);
}

export function pushState(path: string, state: App.PageState) {
	return kitPushState(`${base}${path}`, state);
}

export function replaceState(path: string, state: App.PageState) {
	return kitReplaceState(`${base}${path}`, state);
}
