import type { Codec } from '$lib/url-state';
import type { SortDir } from './list-logic';
export interface FilterDef<T> { key: string; codec: Codec<T>; }
export type FilterDefs<F> = { [P in keyof F]: FilterDef<F[P]> };
export interface TableView<Row, K extends string = string> {
	readonly rows: Row[];
	readonly total: number;
	readonly loading: boolean;
 readonly error?: string | null;
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
