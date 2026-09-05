

import { onMount } from 'svelte';
import { page } from '$app/state';
import { toast } from 'svelte-sonner';
import {
	codecs,
	readURLParam,
	syncToURL,
	onPopstate,
	type Codec,
	type AnyCodecEntry
} from '$lib/url-state';
import { getLocalizedError } from '$lib/errors';
import { nextSort, pageMath, type SortDir } from './list-logic';
import type { FilterDefs } from './list-state.svelte';

export interface ClientListOptions<Row, K extends string, F extends Record<string, unknown>> {

	load: () => Promise<Row[]>;

	searchFields: (row: Row) => (string | null | undefined)[];
	sortKeys: readonly K[];

	sortComparators: Record<K, (a: Row, b: Row) => number>;
	defaultSort: K;

	sortDir?: (key: K) => SortDir;
	pageSizes?: readonly string[];
	defaultPageSize?: string;
	filters?: FilterDefs<F>;

	filterRow?: (row: Row, filters: F) => boolean;
}

export function createClientListState<
	Row,
	K extends string,
	F extends Record<string, unknown> = Record<string, never>
>(options: ClientListOptions<Row, K, F>) {
	const pageSizes = options.pageSizes ?? (['10', '25', '50', '100'] as const);
	const filterDefs = (options.filters ?? {}) as FilterDefs<F>;
	const filterNames = Object.keys(filterDefs) as (keyof F)[];

	const SEARCH_CODEC = codecs.string('');
	const SORT_KEY_CODEC = codecs.enum<K>(options.sortKeys, options.defaultSort);
	const PAGE_SIZE_CODEC = codecs.enum(pageSizes, options.defaultPageSize ?? '25');
	const PAGE_CODEC = codecs.int(1);

	const dirCodec = (key: K): Codec<SortDir> =>
		codecs.enum<SortDir>(['asc', 'desc'] as const, options.sortDir?.(key) ?? 'asc');

	const readFilters = (u: URL): F => {
		const out = {} as F;
		for (const name of filterNames) {
			const def = filterDefs[name];
			out[name] = readURLParam(u, def.key, def.codec);
		}
		return out;
	};

	const url0 = page.url;
	const seededSortKey = readURLParam(url0, 'sort', SORT_KEY_CODEC);
	let query = $state(readURLParam(url0, 'query', SEARCH_CODEC));
	let sortKey = $state(seededSortKey);
	let sortDir = $state(readURLParam(url0, 'sortDir', dirCodec(seededSortKey)));
	let pageSize = $state(readURLParam(url0, 'pageSize', PAGE_SIZE_CODEC));
	let currentPage = $state(readURLParam(url0, 'page', PAGE_CODEC));
	const filters = $state(readFilters(url0)) as F;

	let allRows = $state<Row[]>([]);
	let loading = $state(true);
	let errorMsg = $state<string | null>(null);

	async function load() {
		loading = true;
		errorMsg = null;
		try {
			allRows = await options.load();
		} catch (err) {
			allRows = [];
			errorMsg = getLocalizedError(err);
			toast.error(errorMsg);
			console.error(err);
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		void load();
	});

	const matched = $derived.by(() => {
		const needle = query.trim().toLowerCase();
		const snapshot = $state.snapshot(filters) as F;
		let out = allRows as Row[];
		if (needle) {
			out = out.filter((row) =>
				options.searchFields(row).some((field) => field?.toLowerCase().includes(needle))
			);
		}
		const filterRow = options.filterRow;
		if (filterRow) out = out.filter((row) => filterRow(row, snapshot));
		return out;
	});

	const sorted = $derived.by(() => {
		const compare = options.sortComparators[sortKey];
		const sign = sortDir === 'asc' ? 1 : -1;
		return [...matched].sort((a, b) => sign * compare(a, b));
	});

	const math = $derived(pageMath(sorted.length, currentPage, parseInt(pageSize, 10)));
	const rows = $derived(sorted.slice(math.offset, math.offset + parseInt(pageSize, 10)));

	function commitURL(mode: 'push' | 'replace') {
		const entries: AnyCodecEntry[] = [
			['query', query, SEARCH_CODEC],
			['sort', sortKey, SORT_KEY_CODEC],
			['sortDir', sortDir, dirCodec(sortKey)],
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
		sortDir = readURLParam(u, 'sortDir', dirCodec(sortKey));
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
			return sorted.length;
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
			void load();
		},

		patchRows(fn: (rows: Row[]) => Row[]) {
			allRows = fn(allRows as Row[]);
		}
	};
}
