// Pure logic behind the server-mode DataTable. These three functions carry the
// bugs that hide in every list page (sort-toggle direction, off-by-one
// pagination, filter/offset mapping), so they're pinned here — matching the
// repo's pure-TS test style (no component-test harness exists).
import { describe, it, expect } from 'vitest';
import { SearchScope, SortField, SortDirection } from '$sdk/powermanage/v1/common_pb';
import { nextSort, pageMath, buildSearchArgs, type SortState } from './list-logic';

describe('nextSort', () => {
	it('flips direction when the active column is clicked again', () => {
		expect(nextSort({ key: 'hostname', dir: 'asc' }, 'hostname')).toEqual({
			key: 'hostname',
			dir: 'desc'
		});
		expect(nextSort({ key: 'hostname', dir: 'desc' }, 'hostname')).toEqual({
			key: 'hostname',
			dir: 'asc'
		});
	});

	it('selects a new column at its default direction (asc)', () => {
		expect(nextSort({ key: 'hostname', dir: 'desc' }, 'status')).toEqual({
			key: 'status',
			dir: 'asc'
		});
	});

	it('honors a per-key default direction override', () => {
		const def = (k: string): 'asc' | 'desc' => (k === 'lastSeen' ? 'desc' : 'asc');
		expect(nextSort({ key: 'hostname', dir: 'asc' }, 'lastSeen', def)).toEqual({
			key: 'lastSeen',
			dir: 'desc'
		});
	});
});

describe('pageMath', () => {
	it('computes pages/offset/showing for a full first page', () => {
		expect(pageMath(57, 1, 25)).toEqual({
			totalPages: 3,
			clampedPage: 1,
			offset: 0,
			showingFrom: 1,
			showingTo: 25
		});
	});

	it('computes the last, partial page', () => {
		expect(pageMath(57, 3, 25)).toEqual({
			totalPages: 3,
			clampedPage: 3,
			offset: 50,
			showingFrom: 51,
			showingTo: 57
		});
	});

	it('treats an empty set as page 1 of 1 showing 0', () => {
		expect(pageMath(0, 1, 25)).toEqual({
			totalPages: 1,
			clampedPage: 1,
			offset: 0,
			showingFrom: 0,
			showingTo: 0
		});
	});

	it('clamps an out-of-range page down to the last page', () => {
		expect(pageMath(30, 9, 25).clampedPage).toBe(2);
	});

	it('clamps a below-range page up to 1', () => {
		expect(pageMath(30, 0, 25).clampedPage).toBe(1);
	});
});

describe('buildSearchArgs', () => {
	const config = {
		scope: SearchScope.DEVICES,
		sortFieldMap: {
			hostname: SortField.HOSTNAME,
			lastSeen: SortField.LAST_SEEN_AT
		} as Record<string, SortField>,
		filterToTags: (f: { status: string[] }) =>
			f.status.length === 1 ? { status: f.status[0] } : undefined
	};
	const base = {
		query: '  web ',
		sort: { key: 'hostname', dir: 'asc' } as SortState,
		page: 2,
		pageSize: 25,
		filters: { status: [] as string[] }
	};

	it('maps trimmed query, scope, pageSize, offset, sort field + direction', () => {
		const [query, scope, pageSize, pageToken, dateFilters, tags, sortField, sortDir] =
			buildSearchArgs(base, config);
		expect(query).toBe('web');
		expect(scope).toBe(SearchScope.DEVICES);
		expect(pageSize).toBe(25);
		expect(pageToken).toBe('25'); // offset = (2 - 1) * 25
		expect(dateFilters).toBeUndefined();
		expect(tags).toBeUndefined();
		expect(sortField).toBe(SortField.HOSTNAME);
		expect(sortDir).toBe(SortDirection.ASC);
	});

	it('emits a tag only when exactly one status is selected', () => {
		expect(buildSearchArgs({ ...base, filters: { status: ['online'] } }, config)[5]).toEqual({
			status: 'online'
		});
		// both selected → "all", so no server-side filter
		expect(
			buildSearchArgs({ ...base, filters: { status: ['online', 'offline'] } }, config)[5]
		).toBeUndefined();
	});

	it('maps a descending sort direction', () => {
		expect(
			buildSearchArgs({ ...base, sort: { key: 'lastSeen', dir: 'desc' } }, config)[7]
		).toBe(SortDirection.DESC);
	});
});

// Range filters (created_at / occurred_at) have no tag equivalent — the server
// matches tags by exact value and only compares DateRanges as intervals — so
// they travel in their own SearchRequest slot. These pin that the slot stays
// empty for callers that never opt in, and carries the caller's ranges verbatim
// for the ones that do.
describe('buildSearchArgs date filters', () => {
	const config = {
		scope: SearchScope.EXECUTIONS,
		sortFieldMap: { created: SortField.CREATED_AT } as Record<string, SortField>
	};
	const base = {
		query: '',
		sort: { key: 'created', dir: 'desc' } as SortState,
		page: 1,
		pageSize: 25,
		filters: { start: 1750000000n, end: 1752678400n }
	};
	const range = (start: bigint, end: bigint) => [{ field: 'created_at', start, end }];

	it('leaves the slot undefined when the caller supplies no dateFilters option', () => {
		expect(buildSearchArgs(base, config)[4]).toBeUndefined();
	});

	it('passes the caller ranges through verbatim', () => {
		const args = buildSearchArgs(base, {
			...config,
			dateFilters: (f: { start: bigint; end: bigint }) => range(f.start, f.end)
		});
		expect(args[4]).toEqual([
			{ field: 'created_at', start: 1750000000n, end: 1752678400n }
		]);
	});

	it('passes multiple ranges through in order', () => {
		const args = buildSearchArgs(base, {
			...config,
			dateFilters: () => [
				{ field: 'created_at', start: 1n, end: 2n },
				{ field: 'completed_at', start: 3n, end: 0n }
			]
		});
		expect(args[4]).toEqual([
			{ field: 'created_at', start: 1n, end: 2n },
			{ field: 'completed_at', start: 3n, end: 0n }
		]);
	});

	it('sends undefined rather than an empty array when no range is active', () => {
		expect(buildSearchArgs(base, { ...config, dateFilters: () => [] })[4]).toBeUndefined();
		expect(buildSearchArgs(base, { ...config, dateFilters: () => undefined })[4]).toBeUndefined();
	});

	it('reads the same filter object the tag mapper does', () => {
		const seen: Array<{ start: bigint; end: bigint }> = [];
		buildSearchArgs(base, {
			...config,
			dateFilters: (f: { start: bigint; end: bigint }) => {
				seen.push(f);
				return undefined;
			}
		});
		expect(seen).toEqual([base.filters]);
	});

	it('carries date ranges and tag filters on the same request, independently', () => {
		const args = buildSearchArgs(
			{ ...base, filters: { start: 10n, end: 20n, status: ['3', '4'] } },
			{
				...config,
				filterToTags: (f: { status: string[] }) =>
					f.status.length > 0 ? { status: f.status.join('|') } : undefined,
				dateFilters: (f: { start: bigint; end: bigint }) => range(f.start, f.end)
			}
		);
		expect(args[4]).toEqual([{ field: 'created_at', start: 10n, end: 20n }]);
		expect(args[5]).toEqual({ status: '3|4' });
	});

	it('keeps each slot independent when only one mapper yields a value', () => {
		const withTagsOnly = buildSearchArgs(
			{ ...base, filters: { start: 0n, end: 0n, status: ['3'] } },
			{
				...config,
				filterToTags: (f: { status: string[] }) => ({ status: f.status.join('|') }),
				dateFilters: () => undefined
			}
		);
		expect(withTagsOnly[4]).toBeUndefined();
		expect(withTagsOnly[5]).toEqual({ status: '3' });

		const withDatesOnly = buildSearchArgs(base, {
			...config,
			filterToTags: () => ({}),
			dateFilters: (f: { start: bigint; end: bigint }) => range(f.start, f.end)
		});
		expect(withDatesOnly[4]).toEqual([
			{ field: 'created_at', start: 1750000000n, end: 1752678400n }
		]);
		expect(withDatesOnly[5]).toBeUndefined();
	});
});
