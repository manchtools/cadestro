

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

		expect(activePageSearch()).toBe(actions);
		expect(activePageSearch()?.label()).toBe('Actions');

		release();

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

	it('a late release from the previous page never withdraws the current one', () => {
		const actions = fakePage(4, 'Actions');
		const devices = fakePage(1, 'Devices');

		const releaseActions = registerPageSearch(actions);
		expect(activePageSearch()).toBe(actions);

		registerPageSearch(devices);
		expect(activePageSearch()).toBe(devices);

		releaseActions();
		expect(activePageSearch()).toBe(devices);
	});

	it('the second page does not inherit the first page’s query', () => {
		const actions = fakePage(4, 'Actions');
		const release = registerPageSearch(actions);
		actions.setQuery('api');
		expect(activePageSearch()?.query).toBe('api');
		release();

		const devices = fakePage(1, 'Devices');
		registerPageSearch(devices);
		expect(activePageSearch()?.query).toBe('');
		expect(activePageSearch()?.label()).toBe('Devices');
	});
});
