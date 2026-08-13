// Runes glue for the server-mode DataTable. Owns the standard list state
// (query / sort / pageSize / page + page-specific filters), seeds it from the
// URL, keeps it in the URL through the project's url-state helpers (push for
// committed changes, replace for transient search keystrokes, popstate for
// back/forward), and runs the debounced apiClient.search fetch. The pure,
// bug-prone bits live in ./list-logic (unit-tested there).
import { page } from '$app/state';
import { toast } from 'svelte-sonner';
import { apiClient, type SearchResult } from '$lib/sdk';
import type { SearchScope, SortField } from '$sdk/powermanage/v1/common_pb';
import {
	codecs,
	readURLParam,
	syncToURL,
	onPopstate,
	type Codec,
	type AnyCodecEntry
} from '$lib/url-state';
import { getLocalizedError } from '$lib/errors';
import {
	nextSort,
	pageMath,
	buildSearchArgs,
	type SortDir,
	type SortState,
	type SearchDateFilter
} from './list-logic';

export interface FilterDef<T> {
	/** URL search-param key this filter reads/writes. */
	key: string;
	codec: Codec<T>;
}
export type FilterDefs<F> = { [P in keyof F]: FilterDef<F[P]> };

export interface SearchListOptions<Row, K extends string, F extends Record<string, unknown>> {
	scope: SearchScope;
	/** Turn a raw SearchResult into the typed row (see $lib/search-adapters). */
	adapter: (r: SearchResult) => Row;
	sortKeys: readonly K[];
	sortFieldMap: Record<K, SortField>;
	defaultSort: K;
	/** Default direction when *switching* to a column (asc unless overridden,
	 *  e.g. timestamps desc-first). The URL-seeded initial direction is always
	 *  the stored value (default asc). */
	sortDir?: (key: K) => SortDir;
	/** Direction used when the URL carries no `?sortDir` (default 'asc'). Pages
	 *  whose default column is a timestamp want 'desc' so a bare link reads
	 *  newest-first — and, because this is also the codec default, `?sortDir` stays
	 *  out of the URL until the operator changes it. */
	defaultSortDir?: SortDir;
	pageSizes?: readonly string[];
	defaultPageSize?: string;
	filters?: FilterDefs<F>;
	filterToTags?: (filters: F) => Record<string, string> | undefined;
	/** Map filter state to SearchRequest.date_filters. Range filters (created_at,
	 *  occurred_at) have no tag-filter equivalent — tag matching is exact-value —
	 *  so a date range can only reach the server through here. */
	dateFilters?: (filters: F) => SearchDateFilter[] | undefined;
	/** Debounce before firing the search (default 300ms, matches the pre-existing pages). */
	debounceMs?: number;
	/** Suspend the fetch. A page with a semantic-zoom level that does NOT render
	 *  the list (the fleet's fleet/group zoom, the action library's overview) must
	 *  not spend a Search RPC on rows nobody is looking at — and the fleet keeps
	 *  that promise by unmounting its list level, which also drops the page's
	 *  `registerPageSearch`. A page that wants ⌘K to stay scoped at EVERY zoom
	 *  level keeps this state mounted instead and pauses it here.
	 *
	 *  Receives the live filter object rather than closing over the returned
	 *  state, so the call site never has to reference the binding it is still
	 *  constructing. Read inside the fetch effect after every filter has been
	 *  registered as a dependency, so lifting the pause re-runs the search with
	 *  whatever the URL says by then. */
	paused?: (filters: F) => boolean;
}

export function createSearchListState<
	Row,
	K extends string,
	F extends Record<string, unknown> = Record<string, never>
>(options: SearchListOptions<Row, K, F>) {
	const pageSizes = options.pageSizes ?? (['10', '25', '50', '100'] as const);
	const debounceMs = options.debounceMs ?? 300;
	const filterDefs = (options.filters ?? {}) as FilterDefs<F>;
	const filterNames = Object.keys(filterDefs) as (keyof F)[];

	const SEARCH_CODEC = codecs.string('');
	const SORT_KEY_CODEC = codecs.enum<K>(options.sortKeys, options.defaultSort);
	const SORT_DIR_CODEC = codecs.enum<SortDir>(
		['asc', 'desc'] as const,
		options.defaultSortDir ?? 'asc'
	);
	const PAGE_SIZE_CODEC = codecs.enum(pageSizes, options.defaultPageSize ?? '25');
	const PAGE_CODEC = codecs.int(1);

	const readFilters = (u: URL): F => {
		const out = {} as F;
		for (const name of filterNames) {
			const def = filterDefs[name];
			out[name] = readURLParam(u, def.key, def.codec);
		}
		return out;
	};

	const url0 = page.url;
	let query = $state(readURLParam(url0, 'query', SEARCH_CODEC));
	let sortKey = $state(readURLParam(url0, 'sort', SORT_KEY_CODEC));
	let sortDir = $state(readURLParam(url0, 'sortDir', SORT_DIR_CODEC));
	let pageSize = $state(readURLParam(url0, 'pageSize', PAGE_SIZE_CODEC));
	let currentPage = $state(readURLParam(url0, 'page', PAGE_CODEC));
	const filters = $state(readFilters(url0)) as F;

	let rows = $state<Row[]>([]);
	let total = $state(0);
	let loading = $state(true);
	let errorMsg = $state<string | null>(null);
	let refetchTick = $state(0);

	// Debounced search. Reading each piece of state below registers it as a
	// dependency, so any committed change (or a refresh() tick) re-runs the fetch.
	$effect(() => {
		// Read every filter so a change to any of them re-runs the search.
		// Deliberately NOT $state.snapshot: it deep-clones, which rebuilds class
		// instances as plain objects and strips their methods — a filter holding
		// an @internationalized/date CalendarDate would reach the mappers without
		// .toDate(). The mappers run synchronously inside this effect, so handing
		// them the live proxy keeps both the methods and the dependency tracking.
		for (const name of filterNames) void filters[name];
		// Paused: no request, and `loading` drops to false rather than sitting
		// true forever — a header that shares a spinner with this state would
		// otherwise claim a fetch that was never issued.
		if (options.paused?.(filters)) {
			loading = false;
			return;
		}
		const args = buildSearchArgs(
			{
				query,
				sort: { key: sortKey, dir: sortDir } as SortState<K>,
				page: currentPage,
				pageSize: parseInt(pageSize, 10),
				filters
			},
			{
				scope: options.scope,
				sortFieldMap: options.sortFieldMap,
				filterToTags: options.filterToTags,
				dateFilters: options.dateFilters
			}
		);
		void refetchTick;
		const timer = setTimeout(async () => {
			loading = true;
			errorMsg = null;
			try {
				const resp = await apiClient.search(...args);
				rows = resp.results.map(options.adapter);
				total = resp.totalCount;
			} catch (err) {
				rows = [];
				total = 0;
				errorMsg = getLocalizedError(err);
				toast.error(errorMsg);
				console.error(err);
			} finally {
				loading = false;
			}
		}, debounceMs);
		return () => clearTimeout(timer);
	});

	const math = $derived(pageMath(total, currentPage, parseInt(pageSize, 10)));

	function commitURL(mode: 'push' | 'replace') {
		const entries: AnyCodecEntry[] = [
			['query', query, SEARCH_CODEC],
			['sort', sortKey, SORT_KEY_CODEC],
			['sortDir', sortDir, SORT_DIR_CODEC],
			['pageSize', pageSize, PAGE_SIZE_CODEC],
			['page', currentPage, PAGE_CODEC]
		];
		for (const name of filterNames) {
			entries.push([filterDefs[name].key, filters[name], filterDefs[name].codec]);
		}
		syncToURL(entries, mode);
	}

	onPopstate((u) => {
		query = readURLParam(u, 'query', SEARCH_CODEC);
		sortKey = readURLParam(u, 'sort', SORT_KEY_CODEC);
		sortDir = readURLParam(u, 'sortDir', SORT_DIR_CODEC);
		pageSize = readURLParam(u, 'pageSize', PAGE_SIZE_CODEC);
		currentPage = readURLParam(u, 'page', PAGE_CODEC);
		const next = readFilters(u);
		for (const name of filterNames) filters[name] = next[name];
	});

	return {
		get rows() {
			return rows;
		},
		get total() {
			return total;
		},
		get loading() {
			return loading;
		},
		get error() {
			return errorMsg;
		},
		get query() {
			return query;
		},
		get sortKey() {
			return sortKey;
		},
		get sortDir() {
			return sortDir;
		},
		get page() {
			return math.clampedPage;
		},
		get totalPages() {
			return math.totalPages;
		},
		get pageSize() {
			return pageSize;
		},
		get pageSizes() {
			return pageSizes;
		},
		get showingFrom() {
			return math.showingFrom;
		},
		get showingTo() {
			return math.showingTo;
		},
		/** Reactive filter values (read e.g. `table.filters.status`). */
		filters,

		setSearch(value: string) {
			query = value;
			currentPage = 1;
			commitURL('replace'); // transient — one history entry per intentional change, not per keystroke
		},
		setFilter<P extends keyof F>(name: P, value: F[P]) {
			filters[name] = value;
			currentPage = 1;
			commitURL('push');
		},
		toggleSort(key: K) {
			const next = nextSort({ key: sortKey, dir: sortDir }, key, options.sortDir);
			sortKey = next.key;
			sortDir = next.dir;
			commitURL('push');
		},
		setPageSize(value: string) {
			pageSize = value;
			currentPage = 1;
			commitURL('push');
		},
		gotoPage(p: number) {
			currentPage = p;
			commitURL('push');
		},
		refresh() {
			refetchTick++;
		},
		/** Mutate the local rows without a refetch (optimistic delete/assign). */
		patchRows(fn: (rows: Row[]) => Row[]) {
			rows = fn(rows);
		}
	};
}

/**
 * The display/control surface the DataTable + pagination components consume —
 * a structural subset of createSearchListState's return, deliberately without
 * the page-specific filter generic so those components stay filter-agnostic.
 */
export interface TableView<Row, K extends string = string> {
	readonly rows: Row[];
	readonly total: number;
	readonly loading: boolean;
	readonly sortKey: K;
	readonly sortDir: SortDir;
	readonly page: number;
	readonly totalPages: number;
	readonly pageSize: string;
	readonly pageSizes: readonly string[];
	readonly showingFrom: number;
	readonly showingTo: number;
	toggleSort(key: K): void;
	gotoPage(p: number): void;
	setPageSize(value: string): void;
}
