<script lang="ts">
	import { base } from '$app/paths';
	import { goto } from '$lib/navigation';
	import { toast } from 'svelte-sonner';
	import { apiClient, fetchAllPages, type DeviceGroup, formatTimestampDateTime } from '$lib/sdk';
	import { SearchScope, SortField } from '$contract/cadestro/v1/common_pb';
	import * as m from '$lib/paraglide/messages';
	import { Button } from '$lib/components/ui/button';
	import { Chip } from '$lib/components/fleet';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { MultiSelectCombobox } from '$lib/components/ui/multi-select';
	import ConfirmDeleteDialog from '$lib/components/confirm-delete-dialog.svelte';
	import PageShell from '$lib/components/page-shell.svelte';
	import { Users, Plus, MoreHorizontal, RefreshCw, Trash2, Zap } from '@lucide/svelte';
	import { getLocalizedError } from '$lib/errors';
	import { registerPageSearch } from '$lib/shell/page-search.svelte';
	import { codecs } from '$lib/url-state';
	import { searchResultToDeviceGroup } from '$lib/search-adapters';
	import { RowList, DataTablePagination, createSearchListState } from '$lib/components/data-table';

	type SortKey = 'name' | 'members' | 'created';

	type Zoom = 'overview' | 'list';
	const ZOOMS = ['overview', 'list'] as const;
	const ZOOM_LABEL: Record<Zoom, () => string> = { overview: m.zoom_overview, list: m.zoom_list };
	const ZOOM_CODEC = codecs.enum<Zoom>(ZOOMS, 'overview');

	const table = createSearchListState<DeviceGroup, SortKey, { zoom: Zoom; type: string[] }>({
		scope: SearchScope.DEVICE_GROUPS,
		adapter: searchResultToDeviceGroup,
		sortKeys: ['name', 'members', 'created'],
		defaultSort: 'name',

		sortFieldMap: {
			name: SortField.NAME,
			members: SortField.MEMBER_COUNT,
			created: SortField.CREATED_AT
		},
		sortDir: (key) => (key === 'created' ? 'desc' : 'asc'),
		filters: {
			zoom: { key: 'zoom', codec: ZOOM_CODEC },
			type: { key: 'type', codec: codecs.stringArray([]) }
		},
		filterToTags: (f) =>
			f.type.length === 1 ? { is_dynamic: f.type[0] === 'dynamic' ? 'true' : 'false' } : undefined,

		paused: (f) => f.zoom !== 'list'
	});

	let overview = $state<DeviceGroup[] | null>(null);
	let sweeping = $state(false);
	let sweepError = $state<string | null>(null);

	let swept = false;

	async function sweep() {
		swept = true;
		sweeping = true;
		sweepError = null;
		try {
			overview = await fetchAllPages<DeviceGroup>(async (size, token) => {
				const resp = await apiClient.listDeviceGroups(size, token);
				return { items: resp.groups, nextPageToken: resp.nextPageToken };
			});
		} catch (err) {
			sweepError = getLocalizedError(err);
			console.error(err);
		} finally {
			sweeping = false;
		}
	}

	$effect(() => {
		if (table.filters.zoom !== 'overview' || swept) return;
		void sweep();
	});

	function refresh() {
		if (table.filters.zoom === 'overview') void sweep();
		else table.refresh();
	}

	let deleteDialogOpen = $state(false);
	let groupToDelete = $state<DeviceGroup | null>(null);

	const typeFilterItems = [
		{ id: 'static', label: m.device_groups_static() },
		{ id: 'dynamic', label: m.device_groups_dynamic() }
	];

	const sortOptions = [
		{ key: 'name' as const, label: m.common_name() },
		{ key: 'members' as const, label: m.device_groups_table_devices() },
		{ key: 'created' as const, label: m.device_groups_table_created() }
	];

	function confirmDelete(group: DeviceGroup) {
		groupToDelete = group;
		deleteDialogOpen = true;
	}

	async function deleteGroup() {
		if (!groupToDelete) return;
		try {
			await apiClient.deleteDeviceGroup((groupToDelete.id?.value ?? ''));
			toast.success(m.device_groups_deleted());
			table.patchRows((rows) => rows.filter((g) => g.id?.value !== groupToDelete!.id?.value));
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			deleteDialogOpen = false;
			groupToDelete = null;
		}
	}

	$effect(() =>
		registerPageSearch({
			scope: SearchScope.DEVICE_GROUPS,
			label: m.nav_device_groups,
			get query() {
				return table.query;
			},
			setQuery: (value) => table.setSearch(value),
			clear: () => table.setSearch('')
		})
	);
</script>

<PageShell contentClass="space-y-4">

	{#snippet header()}
		{@const busy = table.filters.zoom === 'overview' ? sweeping : table.loading}
		<div class="flex flex-wrap items-center gap-x-3 gap-y-2">
			<div>
				<h1 class="text-2xl font-bold">{m.device_groups_title()}</h1>
				<p class="text-muted-foreground">{m.device_groups_subtitle()}</p>
			</div>
			<div
				role="group"
				aria-label={m.zoom_label()}
				class="inline-flex overflow-hidden rounded-lg border font-mono text-[0.68rem]"
			>
				{#each ZOOMS as z (z)}
					<button
						type="button"
						data-testid="device-groups-zoom-{z}"
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
				<Button onclick={() => goto('/device-groups/new')}>
					<Plus class="mr-2 h-4 w-4" />
					{m.device_groups_create()}
				</Button>
			</div>
		</div>
	{/snippet}

	{#if table.filters.zoom === 'overview'}

		{#if sweepError}
			<div class="rounded-xl border border-crit/50 bg-crit-soft p-4 text-sm text-crit">
				{sweepError}
			</div>
		{:else if overview !== null && overview.length === 0}
			<div
				class="flex flex-col items-center justify-center rounded-xl border bg-surface px-6 py-12 text-center"
			>
				<Users class="mb-4 h-10 w-10 text-faint" />
				<h3 class="font-semibold">{m.device_groups_empty()}</h3>
				<p class="text-muted-foreground">{m.device_groups_empty_hint()}</p>
			</div>
		{:else}
			<div data-testid="device-groups-overview" class="space-y-2 rounded-xl border bg-sunken p-3">
				<div class="font-mono text-[0.62rem] uppercase tracking-[0.08em] text-faint">
					{m.device_groups_overview_caption()}
				</div>
				<div class="grid grid-cols-[repeat(auto-fill,minmax(180px,1fr))] gap-2">
					{#each overview ?? [] as group (group.id?.value ?? '')}
						<button
							type="button"
							data-testid="overview-tile"
							data-entity-id={group.id?.value ?? ''}
							data-dynamic={group.isDynamic ? 'true' : 'false'}
							onclick={() => goto(`/devices?zoom=group&group=${group.id?.value ?? ''}`)}
							class="flex flex-col gap-1.5 rounded-[10px] border bg-surface p-2.5 text-left hover:border-border-strong focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
						>
							<span class="flex min-w-0 items-center gap-1.5">

								{#if group.isDynamic}
									<Zap class="h-3.5 w-3.5 shrink-0 text-accent-ink" />
								{:else}
									<Users class="h-3.5 w-3.5 shrink-0 text-accent-ink" />
								{/if}
								<span class="truncate font-mono text-[0.75rem] font-semibold">{group.name}</span>
							</span>
							<span class="flex flex-wrap items-center gap-1.5">
								{#if group.isDynamic}
									<Chip tone="info"><Zap class="h-3 w-3" />{m.device_groups_dynamic()}</Chip>
								{:else}
									<Chip tone="idle" label={m.device_groups_static()} />
								{/if}
								<Chip
									tone={group.memberCount > 0 ? 'ok' : 'idle'}
									label={m.policies_member_count({ count: group.memberCount })}
								/>
							</span>
						</button>
					{/each}
				</div>
			</div>
		{/if}
	{:else}

	<RowList {table} {sortOptions} rowKey={(g) => (g.id?.value ?? '')} href={(g) => `${base}/device-groups/${(g.id?.value ?? '')}`}>
		{#snippet filters()}
			<MultiSelectCombobox
				items={typeFilterItems}
				selected={table.filters.type}
				onSelectedChange={(next) => table.setFilter('type', next)}
				placeholder={m.device_groups_filter_all_types()}
				searchPlaceholder={m.common_search()}
				class="w-44"
			/>
		{/snippet}
		{#snippet row(group)}
			<div class="grid h-6 w-6 shrink-0 place-items-center rounded-md bg-accent-soft">
				<Users class="h-3.5 w-3.5 text-accent-ink" />
			</div>
			<span class="min-w-0">
				<span class="block truncate font-mono text-sm font-semibold">{group.name}</span>
				<span class="flex min-w-0 items-baseline gap-2">
					<span class="shrink-0 font-mono text-[0.66rem] text-faint">{group.id?.value ?? ''}</span>
					<span class="truncate text-xs text-muted-foreground">
						{group.description || m.common_no_description()}
					</span>
				</span>
			</span>
			<span class="flex shrink-0 items-center gap-1.5">
				<span title={m.common_type()}>
					{#if group.isDynamic}
						<Chip tone="info"><Zap class="h-3 w-3" />{m.device_groups_dynamic()}</Chip>
					{:else}
						<Chip tone="idle" label={m.device_groups_static()} />
					{/if}
				</span>
				<span title={m.device_groups_table_devices()}>
					<Chip tone={group.memberCount > 0 ? 'ok' : 'idle'} label={String(group.memberCount)} />
				</span>
			</span>
			<span
				class="ml-auto shrink-0 font-mono text-xs tabular-nums text-muted-foreground"
				title={m.device_groups_table_created()}
			>
				{formatTimestampDateTime(group.createdAt)}
			</span>
		{/snippet}

		{#snippet rowEnd(group)}
			<DropdownMenu.Root>
				<DropdownMenu.Trigger>
					{#snippet child({ props })}
						<Button variant="ghost" size="icon" aria-label={m.common_actions()} {...props}>
							<MoreHorizontal class="h-4 w-4" />
						</Button>
					{/snippet}
				</DropdownMenu.Trigger>
				<DropdownMenu.Content align="end">
					<DropdownMenu.Item onclick={() => confirmDelete(group)} class="text-destructive">
						<Trash2 class="mr-2 h-4 w-4" />
						{m.common_delete()}
					</DropdownMenu.Item>
				</DropdownMenu.Content>
			</DropdownMenu.Root>
		{/snippet}

		{#snippet empty()}
			<div class="flex flex-col items-center justify-center px-6 py-12 text-center">
				<Users class="mb-4 h-10 w-10 text-faint" />
				<h3 class="font-semibold">{m.device_groups_empty()}</h3>
				<p class="text-muted-foreground">
					{table.query || table.filters.type.length > 0
						? m.common_try_different_search()
						: m.device_groups_empty_hint()}
				</p>
			</div>
		{/snippet}
	</RowList>

	<DataTablePagination {table} />
	{/if}
</PageShell>

<ConfirmDeleteDialog
	bind:open={deleteDialogOpen}
	title={m.device_groups_delete_dialog_title()}
	description={m.device_groups_delete_dialog_description({ name: groupToDelete?.name ?? '' })}
	onconfirm={deleteGroup}
/>
