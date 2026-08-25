<script lang="ts">
	import { base } from '$app/paths';
	import { goto } from '$lib/navigation';
	import { toast } from 'svelte-sonner';
	import {
		apiClient,
		fetchAllPages,
		type Definition,
		type ActionSet,
		formatTimestampDateTime
	} from '$lib/sdk';
	import { SearchScope, SortField } from '$contract/cadestro/v1/common_pb';
	import { Button } from '$lib/components/ui/button';
	import { Chip } from '$lib/components/fleet';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import ConfirmDeleteDialog from '$lib/components/confirm-delete-dialog.svelte';
	import PageShell from '$lib/components/page-shell.svelte';
	import AddSetsDialog from '$lib/components/add-sets-dialog.svelte';
	import { DateRangePicker } from '$lib/components/ui/date-range-picker';
	import { type DateValue, getLocalTimeZone, parseDate } from '@internationalized/date';
	import { FolderTree, Plus, MoreHorizontal, RefreshCw, Trash2 } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { getLocalizedError } from '$lib/errors';
	import { registerPageSearch } from '$lib/shell/page-search.svelte';
	import { codecs } from '$lib/url-state';
	import { searchResultToDefinition } from '$lib/search-adapters';
	import {
		RowList,
		DataTablePagination,
		createSearchListState,
		type SearchDateFilter
	} from '$lib/components/data-table';

	// ── the drill-down grammar (Actions/Devices reference) ─────────────────────
	// The OVERVIEW is the landing level; the list is one zoom in. An explicit
	// ?zoom=list deep link still wins over the landing default.
	type Zoom = 'overview' | 'list';
	const ZOOMS = ['overview', 'list'] as const;
	const ZOOM_LABEL: Record<Zoom, () => string> = { overview: m.zoom_overview, list: m.zoom_list };
	const ZOOM_CODEC = codecs.enum<Zoom>(ZOOMS, 'overview');

	type SortKey = 'name' | 'sets' | 'created';
	type Filters = {
		zoom: Zoom;
		noSets: boolean;
		createdStart: string;
		createdEnd: string;
		updatedStart: string;
		updatedEnd: string;
	};
	type DayKey = 'createdStart' | 'createdEnd' | 'updatedStart' | 'updatedEnd';

	// Date bounds are stored as ISO YYYY-MM-DD strings, not DateValue objects:
	// the list state passes $state.snapshot(filters) to the request builder, and a
	// snapshot strips class prototypes — a stored CalendarDate would arrive
	// without .toDate(). A malformed param degrades to "no bound".
	function toDay(value: string): DateValue | undefined {
		if (!value) return undefined;
		try {
			return parseDate(value);
		} catch (err) {
			console.warn(err);
			return undefined;
		}
	}

	function dayRange(field: string, start: string, end: string): SearchDateFilter | undefined {
		const from = toDay(start);
		const to = toDay(end);
		if (!from && !to) return undefined;
		const tz = getLocalTimeZone();
		return {
			field,
			start: from ? BigInt(Math.floor(from.toDate(tz).getTime() / 1000)) : 0n,
			// Inclusive end-of-day: the selected day plus one.
			end: to ? BigInt(Math.floor(to.toDate(tz).getTime() / 1000) + 86400) : 0n
		};
	}

	const table = createSearchListState<Definition, SortKey, Filters>({
		scope: SearchScope.DEFINITIONS,
		adapter: searchResultToDefinition,
		sortKeys: ['name', 'sets', 'created'],
		defaultSort: 'created',
		sortFieldMap: {
			name: SortField.NAME,
			sets: SortField.MEMBER_COUNT,
			created: SortField.CREATED_AT
		},
		// Timestamps read newest-first, both on a bare link and when switched to.
		defaultSortDir: 'desc',
		sortDir: (key) => (key === 'created' ? 'desc' : 'asc'),
		filters: {
			zoom: { key: 'zoom', codec: ZOOM_CODEC },
			noSets: { key: 'noSets', codec: codecs.bool(false) },
			createdStart: { key: 'createdStart', codec: codecs.string('') },
			createdEnd: { key: 'createdEnd', codec: codecs.string('') },
			updatedStart: { key: 'updatedStart', codec: codecs.string('') },
			updatedEnd: { key: 'updatedEnd', codec: codecs.string('') }
		},
		// member_count is indexed; "no assigned action sets" is an exact zero match.
		filterToTags: (f) => (f.noSets ? { member_count: '0' } : undefined),
		// Ranges have no tag equivalent — tag matching is exact-value.
		dateFilters: (f) =>
			[
				dayRange('created_at', f.createdStart, f.createdEnd),
				dayRange('updated_at', f.updatedStart, f.updatedEnd)
			].filter((r): r is SearchDateFilter => r !== undefined),
		// The overview does not render rows, so it must not spend a Search RPC.
		paused: (f) => f.zoom !== 'list'
	});

	// ── the overview snapshot: one bounded sweep, loaded lazily ────────────────
	// Every value on a tile is a field the list RPC really returned
	// (Definition.memberCount — contained sets — and Definition.schedule).
	let overview = $state<Definition[] | null>(null);
	let sweeping = $state(false);
	let sweepError = $state<string | null>(null);
	// A plain guard, deliberately NOT $state: the effect below must depend on the
	// zoom alone. A reactive flag would make that effect read what it writes.
	let swept = false;

	async function sweep() {
		swept = true;
		sweeping = true;
		sweepError = null;
		try {
			overview = await fetchAllPages<Definition>(async (size, token) => {
				const resp = await apiClient.listDefinitions(size, token);
				return { items: resp.definitions, nextPageToken: resp.nextPageToken };
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

	/** Refresh whichever level is on screen. */
	function refresh() {
		if (table.filters.zoom === 'overview') void sweep();
		else table.refresh();
	}


	// Both bounds belong to one operator gesture: seed the start directly and let
	// the end's setFilter do the single page-reset + URL commit for the pair.
	function setRange(startKey: DayKey, endKey: DayKey, v: { start?: DateValue; end?: DateValue }) {
		table.filters[startKey] = v.start?.toString() ?? '';
		table.setFilter(endKey, v.end?.toString() ?? '');
	}

	const anyFilterActive = $derived(
		table.query.length > 0 ||
			table.filters.noSets ||
			table.filters.createdStart !== '' ||
			table.filters.createdEnd !== '' ||
			table.filters.updatedStart !== '' ||
			table.filters.updatedEnd !== ''
	);

	// The query lives in the pill now: ⌘K opens search already on this facet and
	// its keystrokes land on the same setSearch the removed input drove.
	$effect(() =>
		registerPageSearch({
			scope: SearchScope.DEFINITIONS,
			label: m.nav_definitions,
			get query() {
				return table.query;
			},
			setQuery: (value) => table.setSearch(value),
			clear: () => table.setSearch('')
		})
	);

	let deleteDialogOpen = $state(false);
	let defToDelete = $state<Definition | null>(null);
	// Creation lives on /definitions/new, a pill-committed route: a modal cannot
	// be stashed, so a half-written definition could never be parked.

	let pickerOpen = $state(false);
	let pickerDefId = $state<string | null>(null);
	let availableSets = $state<ActionSet[]>([]);

	// Headerless rows: the sort keys that were column headers now ride the row
	// list's sort bar, reusing the same labels.
	const sortOptions = [
		{ key: 'name' as const, label: m.actions_table_name() },
		{ key: 'sets' as const, label: m.definitions_table_sets() },
		{ key: 'created' as const, label: m.actions_table_created() }
	];

	function confirmDelete(def: Definition) {
		defToDelete = def;
		deleteDialogOpen = true;
	}

	async function deleteDefinition() {
		if (!defToDelete) return;
		try {
			await apiClient.deleteDefinition((defToDelete.id?.value ?? ''));
			toast.success(m.definitions_deleted());
			table.patchRows((rows) => rows.filter((d) => d.id !== defToDelete!.id));
			table.refresh();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			deleteDialogOpen = false;
			defToDelete = null;
		}
	}

	// The picker offers only sets the definition doesn't already carry, so the
	// authoritative member list comes from the definition itself, not the row.
	async function openAddSetPicker(defId: string) {
		pickerDefId = defId;
		try {
			const [current, sets] = await Promise.all([
				apiClient.getDefinition(defId),
				apiClient.listActionSets()
			]);
			const existingIds = (current.members ?? []).map((mem) => mem.actionSetId?.value ?? '');
			availableSets = sets.sets.filter((s) => !existingIds.includes((s.id?.value ?? '')));
			pickerOpen = true;
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function addSets(setIds: string[]) {
		if (!pickerDefId || setIds.length === 0) return;
		const defId = pickerDefId;
		try {
			const current = await apiClient.getDefinition(defId);
			const startOrder = (current.members ?? []).length;
			for (let i = 0; i < setIds.length; i++) {
				await apiClient.addActionSetToDefinition(defId, setIds[i], startOrder + i);
			}
			toast.success(m.definition_detail_sets_added({ count: setIds.length }));
			table.patchRows((rows) =>
				rows.map((d) => ((d.id?.value ?? '') === defId ? { ...d, memberCount: d.memberCount + setIds.length } : d))
			);
			pickerOpen = false;
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}
</script>

<PageShell contentClass="space-y-4">
	<!-- The header band keeps only what acts on the page itself. The search box is
	     gone — ⌘K is the search, already scoped to this page. -->
	{#snippet header()}
		<div class="flex flex-wrap items-center gap-x-3 gap-y-2">
			<div>
				<h1 class="text-2xl font-bold">{m.definitions_title()}</h1>
				<p class="text-muted-foreground">{m.definitions_subtitle()}</p>
			</div>
			<div
				role="group"
				aria-label={m.zoom_label()}
				class="inline-flex overflow-hidden rounded-lg border font-mono text-[0.68rem]"
			>
				{#each ZOOMS as z (z)}
					<button
						type="button"
						data-testid="definitions-zoom-{z}"
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
				<!-- The list's filters ride IN the list's own toolbar, next to sort:
				     narrowing a list is one act, so it reads as one bar. The page band
				     keeps only what acts on the page itself. -->
				<Button
					onclick={refresh}
					variant="outline"
					disabled={table.filters.zoom === 'overview' ? sweeping : table.loading}
				>
					<span
						class="mr-2 h-4 w-4"
						class:animate-spin={table.filters.zoom === 'overview' ? sweeping : table.loading}
					>
						<RefreshCw class="h-4 w-4" />
					</span>
					{m.common_refresh()}
				</Button>
				<Button onclick={() => goto('/definitions/new')}>
					<Plus class="mr-2 h-4 w-4" />
					{m.definitions_create()}
				</Button>
			</div>
		</div>
	{/snippet}

	{#if table.filters.zoom === 'overview'}
		<!-- The landing level: one card tile per definition — name, its schedule
		     summary and contained-set count, straight off ListDefinitions.
		     Clicking opens the existing detail. -->
		{#if sweepError}
			<div class="rounded-xl border border-crit/50 bg-crit-soft p-4 text-sm text-crit">
				{sweepError}
			</div>
		{:else if overview !== null && overview.length === 0}
			<div
				class="flex flex-col items-center justify-center rounded-xl border bg-surface px-6 py-12 text-center"
			>
				<FolderTree class="mb-4 h-10 w-10 text-faint" />
				<h3 class="font-semibold">{m.definitions_empty()}</h3>
				<p class="text-muted-foreground">{m.definitions_empty_hint()}</p>
			</div>
		{:else}
			<div data-testid="definitions-overview" class="space-y-2 rounded-xl border bg-sunken p-3">
				<div class="font-mono text-[0.62rem] uppercase tracking-[0.08em] text-faint">
					{m.definitions_overview_caption()}
				</div>
				<div class="grid grid-cols-[repeat(auto-fill,minmax(180px,1fr))] gap-2">
					{#each overview ?? [] as definition (definition.id)}
						<button
							type="button"
							data-testid="overview-tile"
							data-entity-id={definition.id}
							onclick={() => goto(`/definitions/${definition.id}`)}
							class="flex flex-col gap-1.5 rounded-[10px] border bg-surface p-2.5 text-left hover:border-border-strong focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
						>
							<span class="flex min-w-0 items-center gap-1.5">
								<FolderTree class="h-3.5 w-3.5 shrink-0 text-accent-ink" />
								<span class="truncate font-mono text-[0.75rem] font-semibold">{definition.name}</span>
							</span>
							<!-- Schedule summary in the display component's own words:
							     the cron string when one is set, the interval otherwise. -->
							<span class="truncate font-mono text-[0.66rem] text-muted-foreground">
								{#if definition.schedule?.cron}
									{m.actions_display_cron()}: {definition.schedule.cron}
								{:else if definition.schedule}
									{m.actions_display_interval({ hours: definition.schedule.intervalHours || 8 })}
								{:else}
									{m.actions_display_interval_default()}
								{/if}
							</span>
							<span class="flex flex-wrap items-center gap-1.5">
								<Chip
									tone={definition.memberCount > 0 ? 'ok' : 'idle'}
									label={m.definitions_count({ count: definition.memberCount })}
								/>
							</span>
						</button>
					{/each}
				</div>
			</div>
		{/if}
	{:else}
	<!-- Same row grammar as the actions list: container tile, name over its ULID,
	     set-count chip, right-aligned stamp. -->
	<div data-tour="library-list">
	<RowList
		{table}
		{sortOptions}
		rowKey={(d) => (d.id?.value ?? '')}
		href={(d) => `${base}/definitions/${d.id}`}
	>
		{#snippet filters()}
			<DateRangePicker
				start={toDay(table.filters.createdStart)}
				end={toDay(table.filters.createdEnd)}
				onChange={(v) => setRange('createdStart', 'createdEnd', v)}
				placeholder={m.common_date_filter_created()}
				class="w-52"
			/>
			<DateRangePicker
				start={toDay(table.filters.updatedStart)}
				end={toDay(table.filters.updatedEnd)}
				onChange={(v) => setRange('updatedStart', 'updatedEnd', v)}
				placeholder={m.common_date_filter_updated()}
				class="w-52"
			/>
			<label class="flex cursor-pointer select-none items-center gap-2 text-sm">
				<Checkbox
					checked={table.filters.noSets}
					onCheckedChange={(checked) => table.setFilter('noSets', checked === true)}
				/>
				{m.definitions_filter_no_sets()}
			</label>
		{/snippet}
		{#snippet row(def)}
			<div class="grid h-6 w-6 shrink-0 place-items-center rounded-md bg-accent-soft">
				<FolderTree class="h-3.5 w-3.5 text-accent-ink" />
			</div>
			<span class="min-w-0">
				<span class="block truncate text-sm font-semibold">{def.name}</span>
				<span class="flex min-w-0 items-baseline gap-2">
					<span class="shrink-0 font-mono text-[0.66rem] text-faint">{def.id}</span>
					<span class="truncate text-xs text-muted-foreground">
						{def.description || m.common_no_description()}
					</span>
				</span>
			</span>
			<span class="shrink-0">
				<Chip
					tone={def.memberCount === 0 ? 'warn' : 'idle'}
					label={m.definitions_count({ count: def.memberCount })}
				/>
			</span>
			<!-- Created is this page's only timestamp sort key, so it is the stamp. -->
			<span class="ml-auto shrink-0 font-mono text-xs tabular-nums text-muted-foreground">
				{formatTimestampDateTime(def.createdAt)}
			</span>
		{/snippet}

		{#snippet rowEnd(def)}
			<DropdownMenu.Root>
				<DropdownMenu.Trigger>
					{#snippet child({ props })}
						<Button variant="ghost" size="icon" aria-label={m.common_actions()} {...props}>
							<MoreHorizontal class="h-4 w-4" />
						</Button>
					{/snippet}
				</DropdownMenu.Trigger>
				<DropdownMenu.Content align="end">
					<DropdownMenu.Item onclick={() => openAddSetPicker((def.id?.value ?? ''))}>
						<Plus class="mr-2 h-4 w-4" />
						{m.definitions_action_sets()}
					</DropdownMenu.Item>
					<DropdownMenu.Separator />
					<DropdownMenu.Item onclick={() => confirmDelete(def)} class="text-destructive">
						<Trash2 class="mr-2 h-4 w-4" />
						{m.common_delete()}
					</DropdownMenu.Item>
				</DropdownMenu.Content>
			</DropdownMenu.Root>
		{/snippet}

		{#snippet empty()}
			<div class="flex flex-col items-center justify-center px-6 py-12 text-center">
				<FolderTree class="mb-4 h-10 w-10 text-faint" />
				<h3 class="font-semibold">{m.definitions_empty()}</h3>
				<p class="text-muted-foreground">
					{anyFilterActive ? m.common_try_different_search() : m.definitions_empty_hint()}
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
	title={m.definitions_delete_dialog_title()}
	description={m.definitions_delete_dialog_description({ name: defToDelete?.name ?? '' })}
	onconfirm={deleteDefinition}
/>

<AddSetsDialog bind:open={pickerOpen} {availableSets} onadd={addSets} />
