// The page ↔ pill search seam.
//
// The bug this file exists to prevent is a STALE SCOPE: a list page publishes
// its query, the operator navigates away, and ⌘K on the next page still routes
// keystrokes into a list nobody is looking at. Every assertion below is paired
// with a positive control, so a seam that never registered anything cannot make
// the "it is gone" tests pass vacuously.
import { describe, it, expect, beforeEach } from 'vitest';
import {
	registerPageSearch,
	activePageSearch,
	resetPageSearch,
	type PageSearchRegistration
} from './page-search.svelte';

function fakePage(
	scope: number | null,
	label: string
): PageSearchRegistration & { calls: string[] } {
	const calls: string[] = [];
	let value = '';
	return {
		calls,
		scope,
		label: () => label,
		get query() {
			return value;
		},
		setQuery(next: string) {
			calls.push(next);
			value = next;
		},
		clear() {
			calls.push('<clear>');
			value = '';
		}
	};
}

beforeEach(() => resetPageSearch());

describe('page-search seam', () => {
	it('publishes nothing until a page registers', () => {
		expect(activePageSearch()).toBeNull();
	});

	it('publishes the registering page, then withdraws it on release', () => {
		const actions = fakePage(4, 'Actions');

		const release = registerPageSearch(actions);
		// positive control: the seam really carried this page…
		expect(activePageSearch()).toBe(actions);
		expect(activePageSearch()?.label()).toBe('Actions');

		release();
		// …so "null" here is a withdrawal, not an empty seam.
		expect(activePageSearch()).toBeNull();
	});

	it('routes writes into the registered page and reads its live query', () => {
		const actions = fakePage(4, 'Actions');
		registerPageSearch(actions);

		activePageSearch()?.setQuery('api');
		expect(actions.calls).toEqual(['api']);
		expect(activePageSearch()?.query).toBe('api');

		activePageSearch()?.clear();
		expect(actions.calls).toEqual(['api', '<clear>']);
		expect(activePageSearch()?.query).toBe('');
	});

	it('a page with no Search scope still registers, carrying null', () => {
		const roles = fakePage(null, 'Roles');
		registerPageSearch(roles);
		expect(activePageSearch()?.scope).toBeNull();
	});

	// THE stale-scope guard. Navigation can destroy the outgoing page AFTER the
	// incoming one mounted, so a release that blindly nulls the seam would blank
	// out the page the operator is actually looking at.
	it('a late release from the previous page never withdraws the current one', () => {
		const actions = fakePage(4, 'Actions');
		const devices = fakePage(1, 'Devices');

		const releaseActions = registerPageSearch(actions);
		expect(activePageSearch()).toBe(actions); // positive control

		registerPageSearch(devices);
		expect(activePageSearch()).toBe(devices);

		releaseActions(); // the outgoing page unmounts last
		expect(activePageSearch()).toBe(devices);
	});

	it('the second page does not inherit the first page’s query', () => {
		const actions = fakePage(4, 'Actions');
		const release = registerPageSearch(actions);
		actions.setQuery('api');
		expect(activePageSearch()?.query).toBe('api'); // positive control
		release();

		const devices = fakePage(1, 'Devices');
		registerPageSearch(devices);
		expect(activePageSearch()?.query).toBe('');
		expect(activePageSearch()?.label()).toBe('Devices');
	});
});
