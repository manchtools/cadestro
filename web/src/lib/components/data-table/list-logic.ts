// Pure, framework-free logic behind the server-mode DataTable. Kept out of the
// runes module so it's unit-testable without a Svelte runtime — the repo has no
// component-test harness, only pure-TS tests.
import { SearchScope, SortField, SortDirection } from '$contract/cadestro/v1/common_pb';

export type SortDir = 'asc' | 'desc';
export type SortState<K extends string = string> = { key: K; dir: SortDir };

/**
 * Header-click sort transition. Clicking the active column flips its direction;
 * clicking a different column selects it at its default direction (asc unless a
 * per-key override says otherwise — e.g. timestamps read better desc-first).
 */
export function nextSort<K extends string>(
	current: SortState<K>,
	clicked: K,
	defaultDir: (key: K) => SortDir = () => 'asc'
): SortState<K> {
	if (current.key === clicked) {
		return { key: clicked, dir: current.dir === 'asc' ? 'desc' : 'asc' };
	}
	return { key: clicked, dir: defaultDir(clicked) };
}

export interface PageMath {
	totalPages: number;
	clampedPage: number;
	offset: number;
	showingFrom: number;
	showingTo: number;
}

/**
 * Pagination display math. `totalPages` is at least 1 (an empty result set is
 * "page 1 of 1", not "of 0"). `page` is clamped into range so a stale,
 * out-of-bounds page from the URL renders the last page rather than a blank one.
 * `showingFrom`/`showingTo` are 1-based inclusive, and 0/0 for an empty set.
 */
export function pageMath(total: number, page: number, pageSize: number): PageMath {
	const totalPages = Math.max(1, Math.ceil(total / pageSize));
	const clampedPage = Math.min(Math.max(1, page), totalPages);
	const offset = (clampedPage - 1) * pageSize;
	const showingFrom = total === 0 ? 0 : offset + 1;
	const showingTo = Math.min(clampedPage * pageSize, total);
	return { totalPages, clampedPage, offset, showingFrom, showingTo };
}

/**
 * One range over an indexed timestamp field, in epoch seconds — the plain-object
 * shape `apiClient.search` accepts for SearchRequest.date_filters (it wraps them
 * in SearchDateFilterSchema itself). A `0n` bound means "unbounded on that side";
 * the server skips a zero bound when range-matching.
 *
 * Ranges cannot be expressed as tag filters: the server matches tags by exact
 * value (with `|` for OR), and only DateRanges are compared as intervals.
 */
export type SearchDateFilter = { field: string; start: bigint; end: bigint };

/** Positional argument tuple for `apiClient.search` (see contract/ts/client.ts:1846). */
export type SearchArgs = [
	query: string,
	scope: SearchScope,
	pageSize: number,
	pageToken: string,
	dateFilters: SearchDateFilter[] | undefined,
	tagFilters: Record<string, string> | undefined,
	sortField: SortField,
	sortDirection: SortDirection
];

export interface ListRequest<K extends string, F> {
	query: string;
	sort: SortState<K>;
	page: number;
	pageSize: number;
	filters: F;
}

export interface SearchConfig<K extends string, F> {
	scope: SearchScope;
	sortFieldMap: Record<K, SortField>;
	/** Map the page's filter state to server tag filters. Return undefined (or an
	 *  empty object) to send none. Page-specific — e.g. devices' single-value
	 *  online/offline rule, where "both selected" means "all", so no filter. */
	filterToTags?: (filters: F) => Record<string, string> | undefined;
	/** Map the page's filter state to server date ranges (SearchRequest.date_filters).
	 *  The only channel for range filters — see SearchDateFilter. Return undefined
	 *  (or an empty array) to send none. */
	dateFilters?: (filters: F) => SearchDateFilter[] | undefined;
}

/**
 * Build the positional `apiClient.search` args from list state. The request
 * offset is raw `(page - 1) * pageSize` — deliberately NOT clamped, because the
 * total is unknown until the response comes back. Callers reset `page` to 1 on
 * filter/search changes so an out-of-range offset can't strand the view;
 * `pageMath` does the display-time clamping once the total is known.
 */
export function buildSearchArgs<K extends string, F>(
	state: ListRequest<K, F>,
	config: SearchConfig<K, F>
): SearchArgs {
	const offset = (state.page - 1) * state.pageSize;
	const tags = config.filterToTags?.(state.filters);
	const dates = config.dateFilters?.(state.filters);
	return [
		state.query.trim(),
		config.scope,
		state.pageSize,
		String(offset),
		dates && dates.length > 0 ? dates : undefined,
		tags && Object.keys(tags).length > 0 ? tags : undefined,
		config.sortFieldMap[state.sort.key],
		state.sort.dir === 'asc' ? SortDirection.ASC : SortDirection.DESC
	];
}
