

import { page } from '$app/state';
import { toast } from 'svelte-sonner';
import { apiClient, type SearchResult } from '$lib/sdk';
import type { SearchScope, SortField } from '$contract/cadestro/v1/common_pb';
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

	key: string;
	codec: Codec<T>;
}
export type FilterDefs<F> = { [P in keyof F]: FilterDef<F[P]> };

export interface SearchListOptions<Row, K extends string, F extends Record<string, unknown>> {
	scope: SearchScope;

	adapter: (r: SearchResult) => Row;
	sortKeys: readonly K[];
	sortFieldMap: Record<K, SortField>;
	defaultSort: K;

	sortDir?: (key: K) => SortDir;

	defaultSortDir?: SortDir;
	pageSizes?: readonly string[];
	defaultPageSize?: string;
	filters?: FilterDefs<F>;
	filterToTags?: (filters: F) => Record<string, string> | undefined;

	dateFilters?: (filters: F) => SearchDateFilter[] | undefined;

	debounceMs?: number;

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

	$effect(() => {

		for (const name of filterNames) void filters[name];

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

		filters,

		setSearch(value: string) {
			query = value;
			currentPage = 1;
			commitURL('replace');
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

		patchRows(fn: (rows: Row[]) => Row[]) {
			rows = fn(rows);
		}
	};
}

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
