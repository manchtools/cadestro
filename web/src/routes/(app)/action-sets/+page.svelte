<script lang="ts">
	import { base } from '$app/paths';
	import { goto } from '$lib/navigation';
	import { toast } from 'svelte-sonner';
	import {
		apiClient,
		fetchAllPages,
		type ActionSet,
		type ManagedAction,
		formatTimestampDateTime
	} from '$lib/sdk';
	import { SearchScope, SortField } from '$contract/cadestro/v1/common_pb';
	import { Button } from '$lib/components/ui/button';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Chip } from '$lib/components/fleet';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import ConfirmDeleteDialog from '$lib/components/confirm-delete-dialog.svelte';
	import PageShell from '$lib/components/page-shell.svelte';
	import ActionPickerWithCreate from '$lib/components/action-picker-with-create.svelte';
	import { DateRangePicker } from '$lib/components/ui/date-range-picker';
	import { type DateValue, getLocalTimeZone, parseDate } from '@internationalized/date';
	import { Layers, Plus, MoreHorizontal, RefreshCw, Trash2 } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { getLocalizedError } from '$lib/errors';
	import { registerPageSearch } from '$lib/shell/page-search.svelte';
	import { codecs } from '$lib/url-state';
	import { searchResultToActionSet } from '$lib/search-adapters';
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

	type SortKey = 'name' | 'actions' | 'created' | 'updated';
	type Filters = {
		zoom: Zoom;
		noActions: boolean;
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

	const table = createSearchListState<ActionSet, SortKey, Filters>({
		scope: SearchScope.ACTION_SETS,
		adapter: searchResultToActionSet,
		sortKeys: ['name', 'actions', 'created', 'updated'],
		defaultSort: 'created',
		sortFieldMap: {
			name: SortField.NAME,
			actions: SortField.MEMBER_COUNT,
			created: SortField.CREATED_AT,
			updated: SortField.UPDATED_AT
		},
		// Timestamps read newest-first, both on a bare link and when switched to.
		defaultSortDir: 'desc',
		sortDir: (key) => (key === 'created' || key === 'updated' ? 'desc' : 'asc'),
		filters: {
			zoom: { key: 'zoom', codec: ZOOM_CODEC },
			noActions: { key: 'noActions', codec: codecs.bool(false) },
			createdStart: { key: 'createdStart', codec: codecs.string('') },
			createdEnd: { key: 'createdEnd', codec: codecs.string('') },
			updatedStart: { key: 'updatedStart', codec: codecs.string('') },
			updatedEnd: { key: 'updatedEnd', codec: codecs.string('') }
		},
		// member_count is indexed; "no assigned actions" is an exact zero match.
		filterToTags: (f) => (f.noActions ? { member_count: '0' } : undefined),
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
	// Every number on a tile is a field the list RPC really returned
	// (ActionSet.memberCount — the set's contained actions). How many assignments
	// reference a set is OMITTED: no list response carries that rollup, and
	// counting it client-side across ListAssignments pages would fabricate it.
	let overview = $state<ActionSet[] | null>(null);
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
			overview = await fetchAllPages<ActionSet>(async (size, token) => {
				const resp = await apiClient.listActionSets(size, token);
				return { items: resp.sets, nextPageToken: resp.nextPageToken };
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
			table.filters.noActions ||
			table.filters.createdStart !== '' ||
			table.filters.createdEnd !== '' ||
			table.filters.updatedStart !== '' ||
			table.filters.updatedEnd !== ''
	);

	// The query lives in the pill now: ⌘K opens search already on this facet and
	// its keystrokes land on the same setSearch the removed input drove.
	$effect(() =>
		registerPageSearch({
			scope: SearchScope.ACTION_SETS,
			label: m.nav_action_sets,
			get query() {
				return table.query;
			},
			setQuery: (value) => table.setSearch(value),
			clear: () => table.setSearch('')
		})
	);

	let deleteDialogOpen = $state(false);
	let setToDelete = $state<ActionSet | null>(null);
	// Creation lives on /action-sets/new, a pill-committed route: naming a set and
	// picking its actions is real unfinished work a modal could never park.
	let pickerOpen = $state(false);
	let pickerSetId = $state<string | null>(null);
	let availableActions = $state<ManagedAction[]>([]);

	// Headerless rows: the sort keys that were column headers now ride the row
	// list's sort bar, reusing the same labels.
	const sortOptions = [
		{ key: 'name' as const, label: m.actions_table_name() },
		{ key: 'actions' as const, label: m.action_sets_table_actions() },
		{ key: 'created' as const, label: m.actions_table_created() },
		{ key: 'updated' as const, label: m.actions_table_updated() }
	];

	function confirmDelete(set: ActionSet) {
		setToDelete = set;
		deleteDialogOpen = true;
	}

	async function deleteActionSet() {
		if (!setToDelete) return;
		try {
			await apiClient.deleteActionSet((setToDelete.id?.value ?? ''));
			toast.success(m.action_sets_deleted());
			table.patchRows((rows) => rows.filter((s) => s.id !== setToDelete!.id));
			table.refresh();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			deleteDialogOpen = false;
			setToDelete = null;
		}
	}

	// The picker offers only actions the set doesn't already carry, so the
	// authoritative member list comes from the set itself, not the search row.
	async function openAddActionPicker(setId: string) {
		pickerSetId = setId;
		try {
			const [current, actions] = await Promise.all([
				apiClient.getActionSet(setId),
				apiClient.listActions()
			]);
			const existingIds = (current.members ?? []).map((mem) => mem.actionId?.value ?? '');
			availableActions = actions.actions.filter((a) => !existingIds.includes((a.id?.value ?? '')));
			pickerOpen = true;
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	function patchMemberCount(setId: string, delta: number) {
		table.patchRows((rows) =>
			rows.map((s) =>
				(s.id?.value ?? '') === setId ? { ...s, memberCount: Math.max(0, s.memberCount + delta) } : s
			)
		);
	}

	async function addActions(actionIds: string[]) {
		if (!pickerSetId || actionIds.length === 0) return;
		const setId = pickerSetId;
		try {
			const current = await apiClient.getActionSet(setId);
			const startOrder = (current.members ?? []).length;
			for (let i = 0; i < actionIds.length; i++) {
				await apiClient.addActionToSet(setId, actionIds[i], startOrder + i);
			}
			toast.success(m.action_set_detail_actions_added({ count: actionIds.length }));
			patchMemberCount(setId, actionIds.length);
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
				<h1 class="text-2xl font-bold">{m.action_sets_title()}</h1>
				<p class="text-muted-foreground">{m.action_sets_subtitle()}</p>
			</div>
			<div
				role="group"
				aria-label={m.zoom_label()}
				class="inline-flex overflow-hidden rounded-lg border font-mono text-[0.68rem]"
			>
				{#each ZOOMS as z (z)}
					<button
						type="button"
						data-testid="action-sets-zoom-{z}"
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
				<Button onclick={() => goto('/action-sets/new')}>
					<Plus class="mr-2 h-4 w-4" />
					{m.action_sets_create()}
				</Button>
			</div>
		</div>
	{/snippet}

	{#if table.filters.zoom === 'overview'}
		<!-- The landing level: one card tile per set — name and its step count,
		     straight off ListActionSets. Clicking opens the existing detail. -->
		{#if sweepError}
			<div class="rounded-xl border border-crit/50 bg-crit-soft p-4 text-sm text-crit">
				{sweepError}
			</div>
		{:else if overview !== null && overview.length === 0}
			<div
				class="flex flex-col items-center justify-center rounded-xl border bg-surface px-6 py-12 text-center"
			>
				<Layers class="mb-4 h-10 w-10 text-faint" />
				<h3 class="font-semibold">{m.action_sets_empty()}</h3>
				<p class="text-muted-foreground">{m.action_sets_empty_hint()}</p>
			</div>
		{:else}
			<div data-testid="action-sets-overview" class="space-y-2 rounded-xl border bg-sunken p-3">
				<div class="font-mono text-[0.62rem] uppercase tracking-[0.08em] text-faint">
					{m.action_sets_overview_caption()}
				</div>
				<div class="grid grid-cols-[repeat(auto-fill,minmax(180px,1fr))] gap-2">
					{#each overview ?? [] as set (set.id)}
						<button
							type="button"
							data-testid="overview-tile"
							data-entity-id={set.id}
							onclick={() => goto(`/action-sets/${set.id}`)}
							class="flex flex-col gap-1.5 rounded-[10px] border bg-surface p-2.5 text-left hover:border-border-strong focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
						>
							<span class="flex min-w-0 items-center gap-1.5">
								<Layers class="h-3.5 w-3.5 shrink-0 text-accent-ink" />
								<span class="truncate font-mono text-[0.75rem] font-semibold">{set.name}</span>
							</span>
							<span class="flex flex-wrap items-center gap-1.5">
								<Chip
									tone={set.memberCount > 0 ? 'ok' : 'idle'}
									label={m.action_sets_count({ count: set.memberCount })}
								/>
							</span>
						</button>
					{/each}
				</div>
			</div>
		{/if}
	{:else}
	<!-- Same row grammar as the actions list: container tile, name over its ULID,
	     step-count chip, right-aligned stamp. -->
	<div data-tour="library-list">
	<RowList
		{table}
		{sortOptions}
		rowKey={(s) => (s.id?.value ?? '')}
		href={(s) => `${base}/action-sets/${s.id}`}
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
					checked={table.filters.noActions}
					onCheckedChange={(checked) => table.setFilter('noActions', checked === true)}
				/>
				{m.action_sets_filter_no_actions()}
			</label>
		{/snippet}
		{#snippet row(set)}
			<div class="grid h-6 w-6 shrink-0 place-items-center rounded-md bg-accent-soft">
				<Layers class="h-3.5 w-3.5 text-accent-ink" />
			</div>
			<span class="min-w-0">
				<span class="block truncate text-sm font-semibold">{set.name}</span>
				<span class="flex min-w-0 items-baseline gap-2">
					<span class="shrink-0 font-mono text-[0.66rem] text-faint">{set.id}</span>
					<span class="truncate text-xs text-muted-foreground">
						{set.description || m.common_no_description()}
					</span>
				</span>
			</span>
			<span class="shrink-0">
				<Chip
					tone={set.memberCount === 0 ? 'warn' : 'idle'}
					label={m.action_sets_count({ count: set.memberCount })}
				/>
			</span>
			<span
				class="ml-auto shrink-0 font-mono text-xs tabular-nums text-muted-foreground"
				title="{m.actions_table_created()}: {formatTimestampDateTime(
					set.createdAt
				)} · {m.actions_table_updated()}: {formatTimestampDateTime(set.updatedAt)}"
			>
				{formatTimestampDateTime(set.updatedAt)}
			</span>
		{/snippet}

		{#snippet rowEnd(set)}
			<DropdownMenu.Root>
				<DropdownMenu.Trigger>
					{#snippet child({ props })}
						<Button variant="ghost" size="icon" aria-label={m.common_actions()} {...props}>
							<MoreHorizontal class="h-4 w-4" />
						</Button>
					{/snippet}
				</DropdownMenu.Trigger>
				<DropdownMenu.Content align="end">
					<DropdownMenu.Item onclick={() => openAddActionPicker((set.id?.value ?? ''))}>
						<Plus class="mr-2 h-4 w-4" />
						{m.action_picker_title()}
					</DropdownMenu.Item>
					<DropdownMenu.Separator />
					<DropdownMenu.Item onclick={() => confirmDelete(set)} class="text-destructive">
						<Trash2 class="mr-2 h-4 w-4" />
						{m.common_delete()}
					</DropdownMenu.Item>
				</DropdownMenu.Content>
			</DropdownMenu.Root>
		{/snippet}

		{#snippet empty()}
			<div class="flex flex-col items-center justify-center px-6 py-12 text-center">
				<Layers class="mb-4 h-10 w-10 text-faint" />
				<h3 class="font-semibold">{m.action_sets_empty()}</h3>
				<p class="text-muted-foreground">
					{anyFilterActive ? m.common_try_different_search() : m.action_sets_empty_hint()}
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
	title={m.action_sets_delete_dialog_title()}
	description={m.action_sets_delete_dialog_description({ name: setToDelete?.name ?? '' })}
	onconfirm={deleteActionSet}
/>


<ActionPickerWithCreate
	bind:open={pickerOpen}
	{availableActions}
	onSelect={addActions}
	onCreate={(action) => addActions([(action.id?.value ?? '')])}
	onClose={() => {
		pickerSetId = null;
	}}
/>
