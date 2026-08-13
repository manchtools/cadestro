// Server-mode DataTable composition: the server does sort/filter/pagination via
// apiClient.search, so this is a lean runes wrapper over url-state + shadcn
// Table — no client-side row model. See devices/+page.svelte for the canonical use.
export { default as DataTable } from './data-table.svelte';
// The headerless list grammar — same TableView contract, row-shaped instead of
// column-shaped. Use it for every browse surface; keep DataTable only where a
// true column grid is the right reading (dense evidence).
export { default as RowList } from './row-list.svelte';
export { default as DataTablePagination } from './data-table-pagination.svelte';
export { createSearchListState, type TableView, type SearchListOptions } from './list-state.svelte';
// Client mode: rows come from a list RPC (no Search scope exists for them) and
// query/sort/filter/pagination run in the browser. See ./client-list-state.
export { createClientListState, type ClientListOptions } from './client-list-state.svelte';
export { type SortDir, type SortState, type SearchDateFilter } from './list-logic';
