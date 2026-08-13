// Tests for the client-mode list factory. Everything the server does for
// createSearchListState (match, filter, sort, slice) happens here in the
// browser, so those are the bugs that can hide: a query that only looks at one
// field, a sort that ignores its direction, a filter that doesn't reset the
// page, a patch that silently refetches, a URL that doesn't round-trip.
//
// Runs in the node project (see vitest.config.ts): the file is a `.svelte.ts`
// module, so runes compile; `onMount` is a no-op there, hence the seams below.
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

const env = vi.hoisted(() => ({
	href: 'https://control.test/roles',
	mounts: [] as Array<() => void>,
	history: [] as Array<{ mode: 'push' | 'replace'; href: string }>,
	toasts: [] as string[]
}));

vi.mock('$app/state', () => ({
	page: {
		get url() {
			return new URL(env.href);
		}
	}
}));

vi.mock('$app/navigation', () => {
	const apply = (mode: 'push' | 'replace', target: string) => {
		env.href = new URL(target, env.href).href;
		env.history.push({ mode, href: env.href });
	};
	return {
		pushState: (target: string) => apply('push', target),
		replaceState: (target: string) => apply('replace', target)
	};
});

// The node build of `svelte` stubs onMount out entirely; capture it instead so
// the test can decide when the initial load runs.
vi.mock('svelte', async (importOriginal) => ({
	...(await importOriginal<typeof import('svelte')>()),
	onMount: (fn: () => void) => {
		env.mounts.push(fn);
	}
}));

vi.mock('svelte-sonner', () => ({
	toast: {
		error: (msg: string) => env.toasts.push(msg),
		success: () => {}
	}
}));

import { codecs } from '$lib/url-state';
import { createClientListState } from './client-list-state.svelte';

interface Row {
	id: string;
	name: string;
	status: 'active' | 'disabled';
	created: number;
}

const FIXTURES: Row[] = [
	{ id: '01alpha', name: 'Alpha', status: 'active', created: 30 },
	{ id: '01beta', name: 'beta', status: 'disabled', created: 20 },
	{ id: '01gamma', name: 'Gamma', status: 'active', created: 50 },
	{ id: '01delta', name: 'Delta', status: 'disabled', created: 10 },
	{ id: '01epsilon', name: 'Epsilon', status: 'active', created: 40 }
];

type SortKey = 'name' | 'created';

function make(load: () => Promise<Row[]> = async () => FIXTURES.map((r) => ({ ...r }))) {
	return createClientListState<Row, SortKey, { status: string[] }>({
		load,
		searchFields: (r) => [r.name, r.id],
		sortKeys: ['name', 'created'],
		sortComparators: {
			name: (a, b) => a.name.localeCompare(b.name),
			created: (a, b) => a.created - b.created
		},
		defaultSort: 'name',
		// Timestamps read newest-first, names A→Z.
		sortDir: (key) => (key === 'created' ? 'desc' : 'asc'),
		pageSizes: ['2', '10', '25'],
		defaultPageSize: '10',
		filters: { status: { key: 'status', codec: codecs.stringArray([]) } },
		filterRow: (r, f) => f.status.length === 0 || f.status.includes(r.status)
	});
}

/** Run the captured onMount callbacks (the initial load) and settle them. */
async function mount() {
	const pending = env.mounts.splice(0);
	for (const fn of pending) fn();
	await vi.waitFor(() => expect(env.mounts).toHaveLength(0));
	await new Promise((resolve) => setTimeout(resolve, 0));
}

const names = (rows: Row[]) => rows.map((r) => r.name);

beforeEach(() => {
	env.href = 'https://control.test/roles';
	env.mounts.length = 0;
	env.history.length = 0;
	env.toasts.length = 0;
	vi.stubGlobal('window', {
		location: {
			get href() {
				return env.href;
			}
		},
		addEventListener: () => {},
		removeEventListener: () => {}
	});
});

afterEach(() => {
	vi.unstubAllGlobals();
	vi.restoreAllMocks();
});

describe('createClientListState — loading', () => {
	it('loads the full row set on mount and sorts it by the default key', async () => {
		const table = make();
		expect(table.loading).toBe(true);
		expect(table.rows).toEqual([]);

		await mount();

		expect(table.loading).toBe(false);
		expect(table.total).toBe(5);
		expect(names(table.rows)).toEqual(['Alpha', 'beta', 'Delta', 'Epsilon', 'Gamma']);
		expect(table.sortKey).toBe('name');
		expect(table.sortDir).toBe('asc');
	});

	it('surfaces a load failure as an error + toast and empties the rows', async () => {
		const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
		const table = make(async () => {
			throw new Error('rpc down');
		});
		await mount();

		expect(table.loading).toBe(false);
		expect(table.rows).toEqual([]);
		expect(table.total).toBe(0);
		expect(table.error).toBeTruthy();
		expect(env.toasts).toHaveLength(1);
		expect(consoleError).toHaveBeenCalledTimes(1);
	});

	it('refresh() re-runs the loader', async () => {
		let calls = 0;
		const table = make(async () => {
			calls++;
			return FIXTURES.slice(0, calls).map((r) => ({ ...r }));
		});
		await mount();
		expect(table.total).toBe(1);

		table.refresh();
		await vi.waitFor(() => expect(table.total).toBe(2));
		expect(calls).toBe(2);
	});
});

describe('createClientListState — query matching', () => {
	it('matches every declared search field, case-insensitively', async () => {
		const table = make();
		await mount();

		table.setSearch('AL');
		expect(names(table.rows)).toEqual(['Alpha']);

		// second declared field (the ULID column) matches too
		table.setSearch('01DELTA');
		expect(names(table.rows)).toEqual(['Delta']);

		table.setSearch('  ');
		expect(table.total).toBe(5);
	});

	it('resets to the first page when the query changes', async () => {
		const table = make();
		await mount();
		table.setPageSize('2');
		table.gotoPage(2);
		expect(table.page).toBe(2);

		table.setSearch('a');
		expect(table.page).toBe(1);
	});
});

describe('createClientListState — sorting', () => {
	it('flips the active column and opens a new column at its natural direction', async () => {
		const table = make();
		await mount();

		table.toggleSort('name');
		expect(table.sortDir).toBe('desc');
		expect(names(table.rows)).toEqual(['Gamma', 'Epsilon', 'Delta', 'beta', 'Alpha']);

		// switching column uses the per-key default (timestamps desc-first)
		table.toggleSort('created');
		expect(table.sortKey).toBe('created');
		expect(table.sortDir).toBe('desc');
		expect(names(table.rows)).toEqual(['Gamma', 'Epsilon', 'Alpha', 'beta', 'Delta']);

		table.toggleSort('created');
		expect(table.sortDir).toBe('asc');
		expect(names(table.rows)).toEqual(['Delta', 'beta', 'Alpha', 'Epsilon', 'Gamma']);
	});
});

describe('createClientListState — filters and pagination', () => {
	it('filters narrow the total, and the page slices what is left', async () => {
		const table = make();
		await mount();
		table.setPageSize('2');

		table.setFilter('status', ['active']);
		expect(table.total).toBe(3);
		expect(table.totalPages).toBe(2);
		expect(names(table.rows)).toEqual(['Alpha', 'Epsilon']);
		expect(table.showingFrom).toBe(1);
		expect(table.showingTo).toBe(2);

		table.gotoPage(2);
		expect(names(table.rows)).toEqual(['Gamma']);
		expect(table.showingFrom).toBe(3);
		expect(table.showingTo).toBe(3);

		// a filter change must not strand the operator on a now-empty page
		table.setFilter('status', ['disabled']);
		expect(table.page).toBe(1);
		expect(names(table.rows)).toEqual(['beta', 'Delta']);
	});

	it('clamps an out-of-range page down to the last one', async () => {
		const table = make();
		await mount();
		table.setPageSize('2');
		table.gotoPage(9);

		expect(table.page).toBe(3);
		expect(names(table.rows)).toEqual(['Gamma']);
	});

	it('reports an empty result set as page 1 of 1 showing nothing', async () => {
		const table = make();
		await mount();
		table.setSearch('no-such-row');

		expect(table.total).toBe(0);
		expect(table.rows).toEqual([]);
		expect(table.totalPages).toBe(1);
		expect(table.showingFrom).toBe(0);
		expect(table.showingTo).toBe(0);
	});
});

describe('createClientListState — patchRows', () => {
	it('applies optimistic deletes and inserts without reloading', async () => {
		let calls = 0;
		const table = make(async () => {
			calls++;
			return FIXTURES.map((r) => ({ ...r }));
		});
		await mount();

		table.patchRows((rows) => rows.filter((r) => r.id !== '01alpha'));
		expect(table.total).toBe(4);
		expect(names(table.rows)).toEqual(['beta', 'Delta', 'Epsilon', 'Gamma']);

		table.patchRows((rows) => [
			{ id: '01zeta', name: 'Aardvark', status: 'active' as const, created: 60 },
			...rows
		]);
		expect(table.total).toBe(5);
		expect(names(table.rows)[0]).toBe('Aardvark');
		expect(calls).toBe(1);
	});

	it('keeps patched rows subject to the active query and filter', async () => {
		const table = make();
		await mount();
		table.setSearch('alpha');
		expect(table.total).toBe(1);

		table.patchRows((rows) => rows.filter((r) => r.id !== '01alpha'));
		expect(table.total).toBe(0);
	});
});

describe('createClientListState — URL round-trip', () => {
	it('writes committed state to the URL and re-seeds an identical view from it', async () => {
		const first = make();
		await mount();

		first.setSearch('a');
		first.setFilter('status', ['active', 'disabled']);
		first.toggleSort('created'); // natural direction for this key
		first.setPageSize('2');
		first.gotoPage(2);
		expect(names(first.rows)).toEqual(['beta', 'Delta']);

		const params = new URL(env.href).searchParams;
		expect(params.get('query')).toBe('a');
		expect(params.get('status')).toBe('active,disabled');
		// list filters stay comma-readable in the address bar
		expect(env.href).toContain('status=active,disabled');
		expect(params.get('sort')).toBe('created');
		// desc is `created`'s natural direction, so it stays out of the URL
		expect(params.get('sortDir')).toBeNull();
		expect(params.get('pageSize')).toBe('2');
		expect(params.get('page')).toBe('2');

		// search is transient (replace); filter/sort/page are committed (push)
		expect(env.history[0].mode).toBe('replace');
		expect(env.history.slice(1).map((h) => h.mode)).toEqual([
			'push',
			'push',
			'push',
			'push'
		]);

		const second = make();
		await mount();
		expect(second.query).toBe('a');
		expect(second.filters.status).toEqual(['active', 'disabled']);
		expect(second.sortKey).toBe('created');
		expect(second.sortDir).toBe('desc');
		expect(second.pageSize).toBe('2');
		expect(second.page).toBe(2);
		expect(names(second.rows)).toEqual(names(first.rows));
	});

	it('records a direction that differs from the column default', async () => {
		const first = make();
		await mount();
		first.toggleSort('created'); // -> desc (natural, omitted)
		first.toggleSort('created'); // -> asc (deliberate, recorded)

		expect(new URL(env.href).searchParams.get('sortDir')).toBe('asc');

		const second = make();
		await mount();
		expect(second.sortKey).toBe('created');
		expect(second.sortDir).toBe('asc');
		expect(names(second.rows)[0]).toBe('Delta');
	});
});
