<script lang="ts">
	// Movement F — the executions section reads as operations, not as a log.
	//
	// The data layer is unchanged: createSearchListState still owns query, sort,
	// filters, the created_at range and the URL contract, and still fetches with
	// apiClient.search over SearchScope.EXECUTIONS. Only the presentation above
	// it changed — rows are clustered into operation cards by ./operation-feed,
	// whose header comment carries the grouping rule in full.
	import { toast } from 'svelte-sonner';
	import {
		apiClient,
		type ActionExecution,
		type SearchResult,
		formatTimestampDateTime
	} from '$lib/sdk';
	import { ExecutionStatus, SearchScope, SortField } from '$sdk/powermanage/v1/common_pb';
	import { getActionTypeOptions } from '$lib/components/actions/action-type';
	import { Button } from '$lib/components/ui/button';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import * as Select from '$lib/components/ui/select';
	import { Badge } from '$lib/components/ui/badge';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { MultiSelectCombobox } from '$lib/components/ui/multi-select';
	import { DateRangePicker } from '$lib/components/ui/date-range-picker';
	import PageShell from '$lib/components/page-shell.svelte';
	import { createSearchListState, type SearchDateFilter } from '$lib/components/data-table';
	import { type DateValue, getLocalTimeZone, parseDate } from '@internationalized/date';
	import { Activity, ArrowDown, ArrowUp, ArrowUpDown, RefreshCw } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { getLocalizedError } from '$lib/errors';
	import { registerPageSearch } from '$lib/shell/page-search.svelte';
	import { getExecutionStatusLabel } from '$lib/execution-status';
	import { codecs, type Codec } from '$lib/url-state';
	import { searchResultToExecution } from '$lib/search-adapters';
	import OperationCard from './operation-card.svelte';
	import { groupOperations, type Operation } from './operation-feed';

	// device_hostname is indexed for free-text search but absent from the typed
	// ActionExecution proto, so the row carries it alongside — the adapter sees
	// the raw SearchResult, which is the only place it exists.
	type ExecutionRow = ActionExecution & { deviceHostname: string };

	function toRow(r: SearchResult): ExecutionRow {
		return { ...searchResultToExecution(r), deviceHostname: r.fields['device_hostname'] ?? '' };
	}

	// 'action' (action name) and 'duration' are not server-sortable fields
	// (executions sortable = device_hostname, status, action_type, created_at),
	// so the feed offers only the three that are.
	type SortKey = 'device' | 'status' | 'created';
	type Filters = {
		types: string[];
		status: string[];
		device: string;
		createdStart: DateValue | undefined;
		createdEnd: DateValue | undefined;
	};

	// Dates serialize as ISO YYYY-MM-DD; a bad value falls back to undefined
	// rather than crashing the page.
	const DATE_CODEC: Codec<DateValue | undefined> = {
		parse: (p) => {
			if (!p) return undefined;
			try {
				return parseDate(p);
			} catch (err) {
				console.warn(err);
				return undefined;
			}
		},
		serialize: (v) => (v ? v.toString() : null)
	};

	/** created_at range → the server's only interval channel (tags are exact-value). */
	function createdRange(f: Filters): SearchDateFilter[] | undefined {
		if (!f.createdStart && !f.createdEnd) return undefined;
		const tz = getLocalTimeZone();
		return [
			{
				field: 'created_at',
				start: f.createdStart ? BigInt(Math.floor(f.createdStart.toDate(tz).getTime() / 1000)) : 0n,
				// Inclusive end-of-day: the selected day plus one.
				end: f.createdEnd ? BigInt(Math.floor(f.createdEnd.toDate(tz).getTime() / 1000) + 86400) : 0n
			}
		];
	}

	const table = createSearchListState<ExecutionRow, SortKey, Filters>({
		scope: SearchScope.EXECUTIONS,
		adapter: toRow,
		sortKeys: ['device', 'status', 'created'],
		defaultSort: 'created',
		// Subset of the server's scopeSortableFields["executions"].
		sortFieldMap: {
			device: SortField.DEVICE_HOSTNAME,
			status: SortField.STATUS,
			created: SortField.CREATED_AT
		},
		// Timestamps read newest-first, both on a bare link and when switched to.
		defaultSortDir: 'desc',
		sortDir: (key) => (key === 'created' ? 'desc' : 'asc'),
		filters: {
			types: { key: 'types', codec: codecs.stringArray([]) },
			status: { key: 'status', codec: codecs.stringArray([]) },
			device: { key: 'device', codec: codecs.string('') },
			createdStart: { key: 'createdStart', codec: DATE_CODEC },
			createdEnd: { key: 'createdEnd', codec: DATE_CODEC }
		},
		filterToTags: (f) => {
			const tags: Record<string, string> = {};
			if (f.status.length > 0) tags.status = f.status.join('|');
			if (f.types.length > 0) tags.action_type = f.types.join('|');
			if (f.device) tags.device_id = f.device;
			return tags;
		},
		dateFilters: createdRange
	});

	// The feed clusters the rows the current page returned; it never asks the
	// server for an operation, because the server has none to give.
	const operations = $derived(groupOperations(table.rows));

	let cancelDialogOpen = $state(false);
	let executionToCancel = $state<ExecutionRow | null>(null);
	let retryDialogOpen = $state(false);
	let operationToRetry = $state<Operation<ExecutionRow> | null>(null);
	let retrying = $state(false);

	const typeFilterItems = getActionTypeOptions()
		.filter((o) => o.value !== 'COMPLIANCE_CHECK')
		.map((o) => ({ id: String(o.type), label: o.label }));

	const statusFilterItems = [
		{ id: String(ExecutionStatus.PENDING), label: m.executions_status_pending() },
		{ id: String(ExecutionStatus.RUNNING), label: m.executions_status_running() },
		{ id: String(ExecutionStatus.SUCCESS), label: m.executions_status_success() },
		{ id: String(ExecutionStatus.FAILED), label: m.executions_status_failed() },
		{ id: String(ExecutionStatus.INDETERMINATE), label: m.executions_status_indeterminate() },
		{ id: String(ExecutionStatus.SKIPPED), label: m.executions_status_skipped() },
		{ id: String(ExecutionStatus.NOT_APPLICABLE), label: m.executions_status_not_applicable() },
		{ id: String(ExecutionStatus.TIMEOUT), label: m.executions_status_timeout() }
	];

	// The feed has no column headers, so the three server-sortable fields keep
	// their own control — the ?sort / ?sortDir contract is unchanged.
	const sortControls = [
		{ key: 'device' as const, label: m.executions_table_device() },
		{ key: 'status' as const, label: m.executions_table_status() },
		{ key: 'created' as const, label: m.executions_table_created() }
	];

	// Both bounds belong to one operator gesture: seed the start directly and let
	// the end's setFilter do the single page-reset + URL commit for the pair.
	function onDateRangeChange(v: { start?: DateValue; end?: DateValue }) {
		table.filters.createdStart = v.start;
		table.setFilter('createdEnd', v.end);
	}

	function shortId(id: string): string {
		return id.slice(0, 8) + '...';
	}

	function confirmCancel(execution: ExecutionRow) {
		executionToCancel = execution;
		cancelDialogOpen = true;
	}

	async function cancelExecution() {
		if (!executionToCancel) return;
		try {
			const updated = await apiClient.cancelExecution(executionToCancel.id);
			if (updated) {
				// The mutation returns the typed proto, which has no hostname — keep
				// the row's indexed one so the effect row doesn't fall back.
				table.patchRows((rows) =>
					rows.map((e) =>
						e.id === updated.id ? { ...updated, deviceHostname: e.deviceHostname } : e
					)
				);
			}
			toast.success(m.execution_cancelled_toast());
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			cancelDialogOpen = false;
			executionToCancel = null;
		}
	}

	function confirmRetry(operation: Operation<ExecutionRow>) {
		operationToRetry = operation;
		retryDialogOpen = true;
	}

	/**
	 * Re-dispatch to exactly the devices whose effect failed.
	 *
	 * DispatchToMultiple is the only retained RPC that fans one ManagedAction out
	 * over a device list, so the operation's own action id plus its failed-device
	 * subset is the whole request — nothing about the earlier dispatch is
	 * replayed, and no succeeded or queued device is touched.
	 */
	async function retryFailed() {
		const operation = operationToRetry;
		if (!operation?.retryable) return;
		retrying = true;
		try {
			const created = await apiClient.dispatchToMultiple(
				operation.retryDeviceIds,
				operation.actionId
			);
			// The typed response carries no hostname; reuse the one the failed row
			// already resolved so the new card reads as devices, not as id stubs.
			const hostnames = new Map(operation.effects.map((e) => [e.deviceId, e.deviceHostname]));
			table.patchRows((rows) => [
				...(created ?? []).map((e) => ({ ...e, deviceHostname: hostnames.get(e.deviceId) ?? '' })),
				...rows
			]);
			toast.success(m.executions_op_retry_done({ count: operation.retryDeviceIds.length }));
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			retrying = false;
			retryDialogOpen = false;
			operationToRetry = null;
		}
	}

	const filtered = $derived(
		!!table.query ||
			table.filters.types.length > 0 ||
			table.filters.status.length > 0 ||
			!!table.filters.device ||
			!!table.filters.createdStart ||
			!!table.filters.createdEnd
	);

	// The query lives in the pill now: ⌘K opens search already on this facet and
	// its keystrokes land on the same setSearch the removed input drove. The
	// registration is withdrawn on unmount so the next page never inherits it.
	$effect(() =>
		registerPageSearch({
			scope: SearchScope.EXECUTIONS,
			label: m.nav_executions,
			get query() {
				return table.query;
			},
			setQuery: (value) => table.setSearch(value),
			clear: () => table.setSearch('')
		})
	);
</script>

<PageShell contentClass="space-y-4">
	<!-- ONE toolbar line: the filters ride the header beside Refresh/Create. The
	     search box is gone — ⌘K is the search, already scoped to this page. -->
	{#snippet header()}
		<div class="flex flex-wrap items-center gap-x-3 gap-y-2">
			<div>
				<h1 class="text-2xl font-bold">{m.executions_title()}</h1>
				<p class="text-muted-foreground">{m.executions_subtitle()}</p>
			</div>
			<div class="ml-auto flex flex-wrap items-center justify-end gap-2">
				{#if table.filters.device}
					<Badge variant="secondary" class="gap-1">
						{m.executions_table_device()}: {shortId(table.filters.device)}
						<!-- The glyph is decoration; the button's NAME is what a screen
						     reader (and a keyboard operator) gets. -->
						<button
							type="button"
							aria-label={m.common_clear_filter()}
							onclick={() => table.setFilter('device', '')}
							class="ml-1 rounded-sm hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
						>
							<span aria-hidden="true">&times;</span>
						</button>
					</Badge>
				{/if}
				<MultiSelectCombobox
					items={typeFilterItems}
					selected={table.filters.types}
					onSelectedChange={(next) => table.setFilter('types', next)}
					placeholder={m.executions_type_all()}
					searchPlaceholder={m.common_search()}
					class="w-44"
				/>
				<MultiSelectCombobox
					items={statusFilterItems}
					selected={table.filters.status}
					onSelectedChange={(next) => table.setFilter('status', next)}
					placeholder={m.executions_status_all()}
					searchPlaceholder={m.common_search()}
					class="w-44"
				/>
				<DateRangePicker
					start={table.filters.createdStart}
					end={table.filters.createdEnd}
					onChange={onDateRangeChange}
					placeholder={m.common_date_filter()}
					class="w-56"
				/>
				<Button onclick={() => table.refresh()} variant="outline" disabled={table.loading}>
					<span class="mr-2 h-4 w-4" class:animate-spin={table.loading}>
						<RefreshCw class="h-4 w-4" />
					</span>
					{m.common_refresh()}
				</Button>
			</div>
		</div>
	{/snippet}

	<div class="flex flex-wrap items-center justify-between gap-3">
		<p class="max-w-prose text-xs text-muted-foreground">
			{m.executions_feed_grouping_note({ count: table.rows.length })}
		</p>
		<div class="flex items-center gap-1.5">
			<span class="text-xs text-muted-foreground">{m.executions_feed_sort()}</span>
			{#each sortControls as control (control.key)}
				<button
					type="button"
					class="flex items-center gap-1 rounded-full border border-hair px-2.5 py-1 text-xs hover:bg-sunken {table.sortKey ===
					control.key
						? 'bg-accent-soft text-accent-ink'
						: 'text-muted-foreground'}"
					onclick={() => table.toggleSort(control.key)}
				>
					{control.label}
					{#if table.sortKey === control.key}
						{#if table.sortDir === 'asc'}<ArrowUp class="h-3 w-3" />{:else}<ArrowDown
								class="h-3 w-3"
							/>{/if}
					{:else}
						<ArrowUpDown class="h-3 w-3 opacity-50" />
					{/if}
				</button>
			{/each}
		</div>
	</div>

	<div class="space-y-2.5" data-tour="ops-feed" data-testid="ops-feed">
		{#if table.loading && table.rows.length === 0}
			{#each Array(3) as _, i (i)}
				<div class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
					<Skeleton class="h-4 w-64" />
				</div>
			{/each}
		{:else if operations.length === 0}
			<!-- Same plate as the loading skeleton above and as an operation card:
			     the feed is one idiom, so an empty feed is not suddenly a Card. -->
			<div
				class="flex flex-col items-center justify-center rounded-xl border border-hair bg-surface px-6 py-12 text-center shadow-plate"
			>
				<Activity class="mb-4 h-10 w-10 text-faint" />
				<h3 class="font-semibold">{m.executions_empty()}</h3>
				<p class="text-muted-foreground">
					{filtered ? m.common_try_different_search() : m.executions_empty_hint()}
				</p>
			</div>
		{:else}
			{#each operations as operation, i (operation.key)}
				<OperationCard
					{operation}
					open={i === 0}
					tour={i === 0 ? 'ops-card' : undefined}
					statusLabel={getExecutionStatusLabel}
					onretry={confirmRetry}
					oncancel={confirmCancel}
				/>
			{/each}
		{/if}
	</div>

	{#if table.total > 0}
		<div class="flex flex-wrap items-center justify-between gap-4">
			<div class="flex items-center gap-2 text-sm text-muted-foreground">
				<span>{m.pagination_rows_per_page()}</span>
				<Select.Root
					type="single"
					value={table.pageSize}
					onValueChange={(v) => v && table.setPageSize(v)}
				>
					<Select.Trigger class="w-18 h-8">{table.pageSize}</Select.Trigger>
					<Select.Content>
						{#each table.pageSizes as size (size)}
							<Select.Item value={size}>{size}</Select.Item>
						{/each}
					</Select.Content>
				</Select.Root>
			</div>
			<div class="flex items-center gap-3">
				<!-- Keyset honesty: a window into the stream, never "page 3 of 40". -->
				<span class="font-mono text-xs text-muted-foreground">
					{m.pagination_showing({
						from: String(table.showingFrom),
						to: String(table.showingTo),
						total: String(table.total)
					})}
				</span>
				<Button
					variant="outline"
					size="sm"
					onclick={() => table.gotoPage(table.page - 1)}
					disabled={table.page <= 1}
				>
					{m.executions_feed_show_previous()}
				</Button>
				<Button
					variant="outline"
					size="sm"
					onclick={() => table.gotoPage(table.page + 1)}
					disabled={table.page >= table.totalPages}
				>
					{m.executions_feed_show_next()}
				</Button>
			</div>
		</div>
	{/if}
</PageShell>

<AlertDialog.Root bind:open={cancelDialogOpen}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{m.executions_cancel_dialog_title()}</AlertDialog.Title>
			<AlertDialog.Description>{m.execution_cancel_confirm()}</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>{m.common_cancel()}</AlertDialog.Cancel>
			<AlertDialog.Action onclick={cancelExecution} variant="destructive">
				{m.execution_cancel()}
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

<AlertDialog.Root bind:open={retryDialogOpen}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{m.executions_op_retry_dialog_title()}</AlertDialog.Title>
			<AlertDialog.Description>
				{m.executions_op_retry_confirm({ count: operationToRetry?.retryDeviceIds.length ?? 0 })}
			</AlertDialog.Description>
		</AlertDialog.Header>
		{#if operationToRetry}
			<ul class="max-h-48 space-y-1 overflow-y-auto rounded-lg bg-sunken p-3 font-mono text-xs">
				{#each operationToRetry.effects.filter((e) => operationToRetry?.retryDeviceIds.includes(e.deviceId)) as effect (effect.id)}
					<li class="flex items-center justify-between gap-3">
						<span>{effect.deviceHostname || shortId(effect.deviceId)}</span>
						<span class="text-faint">{formatTimestampDateTime(effect.createdAt)}</span>
					</li>
				{/each}
			</ul>
		{/if}
		<AlertDialog.Footer>
			<AlertDialog.Cancel>{m.common_cancel()}</AlertDialog.Cancel>
			<AlertDialog.Action onclick={retryFailed} disabled={retrying}>
				{m.executions_op_retry_action()}
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
