<script lang="ts">
 import { base } from '$app/paths';
 import { goto } from '$lib/navigation';
 import { toast } from 'svelte-sonner';
 import { DateRangePicker } from '$lib/components/ui/date-range-picker';
 import { dateCodec } from '$lib/components/ui/date-range-picker/date-codec';
 import { type DateValue, getLocalTimeZone } from '@internationalized/date';
 import { api } from '$lib/api';
 import { Permission, type ManagedAction } from '$contract/cadestro/v1/control_pb';
 import { DesiredState } from '$contract/cadestro/v1/common_pb';
 import { consoleContext } from '$lib/console-context.svelte';
 import { collectPages, formatDate as formatTimestampDateTime } from '$lib/console';
 import { Button } from '$lib/components/ui/button';
 import { Chip, Stat, TONE_FILL } from '$lib/components/fleet';
 import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
 import { MultiSelectCombobox } from '$lib/components/ui/multi-select';
 import ConfirmDeleteDialog from '$lib/components/confirm-delete-dialog.svelte';
 import PageShell from '$lib/components/page-shell.svelte';
 import { Zap, Plus, MoreHorizontal, RefreshCw, Trash2 } from '@lucide/svelte';
 import * as m from '$lib/paraglide/messages';
 import { getLocalizedError } from '$lib/errors';
 import { registerPageSearch } from '$lib/shell/page-search.svelte';
 import { codecs } from '$lib/url-state';
 import { RowList, DataTablePagination, createClientListState } from '$lib/components/data-table';
 import { actionChoice, getActionTypeInfoByValue, TILE_VALUES } from '$lib/components/actions/action-type';
 import LibraryOverview from './library-overview.svelte';
 import { buildBubbles, summarize, bucketOf } from './library-model';
 const { can } = consoleContext();
 type SortKey = 'name' | 'type' | 'created' | 'updated';
 type Zoom = 'list' | 'overview';
 const ZOOMS = ['list', 'overview'] as const;
 const ZOOM_LABEL = { list: m.actions_zoom_list, overview: m.actions_zoom_overview };
 let allActions = $state<ManagedAction[]>([]);
 const table = createClientListState<ManagedAction, SortKey, { zoom: Zoom; types: string[]; createdStart: DateValue | undefined; createdEnd: DateValue | undefined }>({
  load: async () => {
   allActions = can(Permission.LIST_ACTIONS) ? await collectPages(async (pageToken) => { const r = await api.listActions({ pageSize: 100, pageToken }); return { items: r.actions, nextPageToken: r.nextPageToken }; }) : [];
   return allActions;
  },
  searchFields: (a) => [a.name, a.description],
  sortKeys: ['name', 'type', 'created', 'updated'], defaultSort: 'created',
  sortComparators: { name: (a,b) => a.name.localeCompare(b.name), type: (a,b) => actionChoice(a).localeCompare(actionChoice(b)), created: (a,b) => Number((a.createdAt?.seconds ?? 0n) - (b.createdAt?.seconds ?? 0n)), updated: (a,b) => Number((a.updatedAt?.seconds ?? 0n) - (b.updatedAt?.seconds ?? 0n)) },
  sortDir: (key) => key === 'created' || key === 'updated' ? 'desc' : 'asc',
  filters: { zoom: { key: 'zoom', codec: codecs.enum<Zoom>(ZOOMS, 'overview') }, types: { key: 'types', codec: codecs.stringArray([]) }, createdStart: { key: 'createdStart', codec: dateCodec }, createdEnd: { key: 'createdEnd', codec: dateCodec } },
  filterRow: (a,f) => (!f.types.length || f.types.includes(bucketOf(a))) && (!f.createdStart || Number(a.createdAt?.seconds ?? 0n)*1000 >= f.createdStart.toDate(getLocalTimeZone()).getTime()) && (!f.createdEnd || Number(a.createdAt?.seconds ?? 0n)*1000 < f.createdEnd.add({ days: 1 }).toDate(getLocalTimeZone()).getTime())
 });
 const snapshot = $derived({ actions: allActions, total: allActions.length, truncated: false });
 const sweeping = $derived(table.loading);
 const sweepError = $derived(table.error);
 const bubbles = $derived(buildBubbles(allActions));
 const summary = $derived(summarize(allActions));
 const overviewEmpty = $derived(!table.loading && !allActions.length);
 const typeFilterItems = TILE_VALUES.map(value => ({ id: value === 'COMPLIANCE_CHECK' ? 'compliance' : value.toLowerCase(), label: getActionTypeInfoByValue(value).label }));
 const sortOptions = [{ key: 'name' as const, label: m.actions_table_name() }, { key: 'type' as const, label: m.actions_table_type() }, { key: 'created' as const, label: m.actions_table_created() }, { key: 'updated' as const, label: m.actions_table_updated() }];
 const anyFilterActive = $derived(!!table.query || !!table.filters.types.length || !!table.filters.createdStart || !!table.filters.createdEnd);
 let deleteDialogOpen = $state(false);
 let actionToDelete = $state<ManagedAction | null>(null);
 function focusType(bucket: string) { table.filters.types = [bucket]; table.setFilter('zoom', 'list'); }
 function refresh() { table.refresh(); }
 function getDisplayInfo(action: ManagedAction) { return getActionTypeInfoByValue(actionChoice(action)); }
 function confirmDelete(action: ManagedAction) { actionToDelete = action; deleteDialogOpen = true; }
 async function deleteAction() {
  if (!actionToDelete || !can(Permission.DELETE_ACTION)) return;
  try { await api.deleteAction({ id: actionToDelete.id }); toast.success(m.actions_deleted()); table.refresh(); }
  catch (error) { toast.error(getLocalizedError(error)); } finally { deleteDialogOpen = false; }
 }
 $effect(() => registerPageSearch({ scope: null, label: m.nav_actions, get query() { return table.query; }, setQuery: table.setSearch, clear: () => table.setSearch('') }));
</script>

<PageShell contentClass="space-y-4">

	{#snippet header()}
		{@const busy = table.filters.zoom === 'overview' ? sweeping : table.loading}
		<div class="flex flex-wrap items-center gap-x-3 gap-y-2">
			<div>
				<h1 class="text-2xl font-bold">{m.actions_title()}</h1>
				<p class="text-muted-foreground">{m.actions_subtitle()}</p>
			</div>
			<div
				data-tour="library-zoom"
				role="group"
				aria-label={m.actions_zoom_label()}
				class="inline-flex overflow-hidden rounded-lg border font-mono text-[0.68rem]"
			>
				{#each ZOOMS as z (z)}
					<button
						type="button"
						data-testid="library-zoom-{z}"
						aria-pressed={table.filters.zoom === z}
						onclick={() => table.setFilter('zoom', z)}
						class="border-r px-2.5 py-1 last:border-r-0 {table.filters.zoom === z
							? 'bg-accent-soft font-semibold text-accent-ink'
							: 'text-muted-foreground hover:text-foreground'}"
					>
						{ZOOM_LABEL[z]()}
					</button>
				{/each}
			</div>
			<div class="ml-auto flex flex-wrap items-center justify-end gap-2">

				<Button onclick={refresh} variant="outline" disabled={busy}>
					<span class="mr-2 h-4 w-4" class:animate-spin={busy}>
						<RefreshCw class="h-4 w-4" />
					</span>
					{m.common_refresh()}
				</Button>
				<Button disabled={!can(Permission.CREATE_ACTION)} onclick={() => goto('/actions/new')}>
					<Plus class="mr-2 h-4 w-4" />
					{m.actions_create()}
				</Button>
			</div>
		</div>

		{#if table.filters.zoom === 'overview' && snapshot}
			<div data-tour="library-summary" class="flex flex-wrap gap-2">
				<Stat tone="ok" value={summary.install} label={m.desired_state_present()} />
				<Stat tone="crit" value={summary.remove} label={m.desired_state_absent()} />
				<Stat tone="info" value={bubbles.length} label={m.actions_stat_types()} />
			</div>
		{/if}
	{/snippet}

	{#if table.filters.zoom === 'overview'}
		{#if sweepError}
			<p
				data-testid="library-error"
				class="rounded-lg border border-crit bg-crit-soft px-3 py-2 text-sm text-crit"
			>
				{sweepError}
			</p>
		{/if}

		{#if snapshot?.truncated}

			<p
				data-testid="library-truncated"
				class="rounded-lg border border-warn bg-warn-soft px-3 py-2 text-xs text-warn"
			>
				{m.actions_overview_truncated({ shown: snapshot.actions.length, total: snapshot.total })}
			</p>
		{/if}

		{#if table.query}

			<p data-testid="library-query-note" class="text-xs text-faint">
				{m.actions_overview_query_note()}
			</p>
		{/if}

		{#if sweeping && !snapshot}
			<p class="py-6 text-center text-sm text-muted-foreground">{m.common_loading()}</p>
		{:else if overviewEmpty}
			<div
				class="flex flex-col items-center justify-center rounded-xl border border-hair bg-surface px-6 py-12 text-center"
			>
				<Zap class="mb-4 h-10 w-10 text-faint" />
				<h3 class="font-semibold">{m.actions_empty()}</h3>
				<p class="text-muted-foreground">{m.actions_empty_hint()}</p>
			</div>
		{:else}
			<LibraryOverview {bubbles} onFocus={focusType} />

			<div class="flex flex-wrap gap-3.5 text-[0.7rem] text-muted-foreground">
				<span class="inline-flex items-center gap-1.5">
					<span aria-hidden="true" class="h-2.5 w-2.5 rounded-[3px] {TONE_FILL.ok}"></span>
					{m.desired_state_present()}
				</span>
				<span class="inline-flex items-center gap-1.5">
					<span aria-hidden="true" class="relative h-2.5 w-2.5 rounded-[3px] {TONE_FILL.crit}">
						<span
							class="absolute right-0 top-0 h-1.5 w-1.5 bg-marker-strong [clip-path:polygon(100%_0,0_0,100%_100%)]"
						></span>
					</span>
					{m.desired_state_absent()}
				</span>
			</div>
		{/if}
	{:else}

	<div data-tour="library-list">
	<RowList
		{table}
		{sortOptions}
		rowKey={(a) => (a.id?.value ?? '')}
		href={(can(Permission.GET_ACTION) || can(Permission.LIST_ACTIONS)) ? (a) => `${base}/actions/${a.id?.value ?? ''}` : undefined}
	>
		{#snippet filters()}
 <DateRangePicker start={table.filters.createdStart} end={table.filters.createdEnd} onChange={({start, end}) => { table.setFilter('createdStart', start); table.setFilter('createdEnd', end); }} placeholder="Created date" />
			<MultiSelectCombobox
				items={typeFilterItems}
				selected={table.filters.types}
				onSelectedChange={(next) => table.setFilter('types', next)}
				placeholder={m.actions_filter_all_types()}
				searchPlaceholder={m.common_search()}
				class="w-44"
			/>
		{/snippet}
		{#snippet row(action)}
			{@const info = getDisplayInfo(action)}
			{@const Icon = info.icon}
			{@const absent = action.desiredState === DesiredState.ABSENT}
			<div class="grid h-6 w-6 shrink-0 place-items-center rounded-md bg-accent-soft">
				<Icon class="h-3.5 w-3.5 text-accent-ink" />
			</div>
			<span class="min-w-0">
				<span class="block truncate text-sm font-semibold">{action.name}</span>
				<span class="flex min-w-0 items-baseline gap-2">
					<span class="shrink-0 font-mono text-[0.66rem] text-faint">{action.id?.value ?? ''}</span>
					<span class="truncate text-xs text-muted-foreground">
						{action.description || m.common_no_description()}
					</span>
				</span>
			</span>
			<span class="flex shrink-0 items-center gap-1.5">
				<Chip tone="info" label={info.label} />
				<Chip
					tone={absent ? 'crit' : 'ok'}
					label={absent ? m.desired_state_absent() : m.desired_state_present()}
				/>
			</span>

			<span
				class="ml-auto shrink-0 font-mono text-xs tabular-nums text-muted-foreground"
				title="{m.actions_table_created()}: {formatTimestampDateTime(
					action.createdAt
				)} · {m.actions_table_updated()}: {formatTimestampDateTime(action.updatedAt)}"
			>
				{formatTimestampDateTime(action.updatedAt)}
			</span>
		{/snippet}

		{#snippet rowEnd(action)}
			<DropdownMenu.Root>
				<DropdownMenu.Trigger>
					{#snippet child({ props })}
						<Button variant="ghost" size="icon" aria-label={m.common_actions()} {...props}>
							<MoreHorizontal class="h-4 w-4" />
						</Button>
					{/snippet}
				</DropdownMenu.Trigger>
				<DropdownMenu.Content align="end">
					<DropdownMenu.Item disabled={!can(Permission.DELETE_ACTION)} onclick={() => confirmDelete(action)} class="text-destructive">
						<Trash2 class="mr-2 h-4 w-4" />
						{m.common_delete()}
					</DropdownMenu.Item>
				</DropdownMenu.Content>
			</DropdownMenu.Root>
		{/snippet}

		{#snippet empty()}
			<div class="flex flex-col items-center justify-center px-6 py-12 text-center">
				<Zap class="mb-4 h-10 w-10 text-faint" />
				<h3 class="font-semibold">{m.actions_empty()}</h3>
				<p class="text-muted-foreground">
					{anyFilterActive ? m.common_try_different_search() : m.actions_empty_hint()}
				</p>
			</div>
		{/snippet}
	</RowList>
	</div>

	<DataTablePagination {table} />
	{/if}
</PageShell>

<ConfirmDeleteDialog
	bind:open={deleteDialogOpen}
	title={m.actions_delete_dialog_title()}
	description={m.actions_delete_dialog_description({ name: actionToDelete?.name ?? '' })}
	onconfirm={deleteAction}
/>
