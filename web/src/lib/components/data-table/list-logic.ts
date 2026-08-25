

import { SearchScope, SortField, SortDirection } from '$contract/cadestro/v1/common_pb';

export type SortDir = 'asc' | 'desc';
export type SortState<K extends string = string> = { key: K; dir: SortDir };

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

export function pageMath(total: number, page: number, pageSize: number): PageMath {
	const totalPages = Math.max(1, Math.ceil(total / pageSize));
	const clampedPage = Math.min(Math.max(1, page), totalPages);
	const offset = (clampedPage - 1) * pageSize;
	const showingFrom = total === 0 ? 0 : offset + 1;
	const showingTo = Math.min(clampedPage * pageSize, total);
	return { totalPages, clampedPage, offset, showingFrom, showingTo };
}

export type SearchDateFilter = { field: string; start: bigint; end: bigint };

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

	filterToTags?: (filters: F) => Record<string, string> | undefined;

	dateFilters?: (filters: F) => SearchDateFilter[] | undefined;
}

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
