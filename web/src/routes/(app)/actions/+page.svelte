<script lang="ts">
	import { base } from '$app/paths';
	import { goto } from '$lib/navigation';
	import { toast } from 'svelte-sonner';
	import { apiClient, type ManagedAction, formatTimestampDateTime } from '$lib/sdk';
	import { ActionType } from '$contract/cadestro/v1/actions_pb';
	import { DesiredState, SearchScope, SortField } from '$contract/cadestro/v1/common_pb';
	import { Button } from '$lib/components/ui/button';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Chip, Stat, TONE_FILL } from '$lib/components/fleet';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { MultiSelectCombobox } from '$lib/components/ui/multi-select';
	import ConfirmDeleteDialog from '$lib/components/confirm-delete-dialog.svelte';
	import PageShell from '$lib/components/page-shell.svelte';
	import { DateRangePicker } from '$lib/components/ui/date-range-picker';
	import { type DateValue, getLocalTimeZone, parseDate } from '@internationalized/date';
	import { Zap, Plus, MoreHorizontal, RefreshCw, Trash2 } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { getLocalizedError } from '$lib/errors';
	import { registerPageSearch } from '$lib/shell/page-search.svelte';
	import { codecs } from '$lib/url-state';
	import { searchResultToManagedAction } from '$lib/search-adapters';
	import {
		RowList,
		DataTablePagination,
		createSearchListState,
		type SearchDateFilter
	} from '$lib/components/data-table';
	import {
		getActionTypeLabel,
		getActionTypeIcon,
		getActionTypeOptions,
		getActionTypeInfoByValue
	} from '$lib/components/actions';
	import LibraryOverview from './library-overview.svelte';
	import { buildBubbles, summarize, COMPLIANCE_BUCKET } from './library-model';
	import { loadLibrary, type LibrarySnapshot } from './library-data';

	type SortKey = 'name' | 'type' | 'created' | 'updated';

	type Zoom = 'list' | 'overview';
	type Filters = {
		zoom: Zoom;
		types: string[];
		unassigned: boolean;
		createdStart: string;
		createdEnd: string;
		updatedStart: string;
		updatedEnd: string;
	};
	type DayKey = 'createdStart' | 'createdEnd' | 'updatedStart' | 'updatedEnd';

	const ZOOMS = ['list', 'overview'] as const;
	const ZOOM_LABEL: Record<Zoom, () => string> = {
		list: m.actions_zoom_list,
		overview: m.actions_zoom_overview
	};

	const ZOOM_CODEC = codecs.enum<Zoom>(ZOOMS, 'overview');

	const COMPLIANCE_FILTER_ID = COMPLIANCE_BUCKET;

	const TYPE_SLUGS: [string, number][] = getActionTypeOptions()
		.filter((o) => o.value !== 'COMPLIANCE_CHECK')
		.map((o) => [o.value.toLowerCase(), o.type]);
	const TYPE_SLUG_TO_NUM = new Map<string, number>(TYPE_SLUGS);

	const SLUG_BY_TYPE = new Map<number, string>(TYPE_SLUGS.map(([slug, num]) => [num, slug]));

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

			end: to ? BigInt(Math.floor(to.toDate(tz).getTime() / 1000) + 86400) : 0n
		};
	}

	const table = createSearchListState<ManagedAction, SortKey, Filters>({
		scope: SearchScope.ACTIONS,
		adapter: searchResultToManagedAction,
		sortKeys: ['name', 'type', 'created', 'updated'],
		defaultSort: 'created',
		sortFieldMap: {
			name: SortField.NAME,
			type: SortField.TYPE,
			created: SortField.CREATED_AT,
			updated: SortField.UPDATED_AT
		},

		defaultSortDir: 'desc',
		sortDir: (key) => (key === 'created' || key === 'updated' ? 'desc' : 'asc'),
		filters: {
			zoom: { key: 'zoom', codec: ZOOM_CODEC },
			types: { key: 'types', codec: codecs.stringArray([]) },
			unassigned: { key: 'unassigned', codec: codecs.bool(false) },
			createdStart: { key: 'createdStart', codec: codecs.string('') },
			createdEnd: { key: 'createdEnd', codec: codecs.string('') },
			updatedStart: { key: 'updatedStart', codec: codecs.string('') },
			updatedEnd: { key: 'updatedEnd', codec: codecs.string('') }
		},

		dateFilters: (f) =>
			[
				dayRange('created_at', f.createdStart, f.createdEnd),
				dayRange('updated_at', f.updatedStart, f.updatedEnd)
			].filter((r): r is SearchDateFilter => r !== undefined),

		filterToTags: (f) => {
			const tags: Record<string, string> = {};
			const numericTypes = f.types
				.filter((t) => t !== COMPLIANCE_FILTER_ID)
				.map((slug) => TYPE_SLUG_TO_NUM.get(slug))
				.filter((n): n is number => n !== undefined)
				.map(String);
			if (numericTypes.length > 0) tags.type = numericTypes.join('|');
			if (f.types.includes(COMPLIANCE_FILTER_ID)) tags.is_compliance = 'true';
			if (f.unassigned) tags.assigned = 'false';
			return tags;
		},

		paused: (f) => f.zoom !== 'list'
	});

	$effect(() =>
		registerPageSearch({
			scope: SearchScope.ACTIONS,
			label: m.nav_actions,
			get query() {
				return table.query;
			},
			setQuery: (value) => table.setSearch(value),
			clear: () => table.setSearch('')
		})
	);

	let snapshot = $state<LibrarySnapshot | null>(null);
	let sweeping = $state(false);
	let sweepError = $state<string | null>(null);

	let swept = false;

	async function sweep() {
		swept = true;
		sweeping = true;
		sweepError = null;
		try {
			snapshot = await loadLibrary();
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

	const bubbles = $derived(buildBubbles(snapshot?.actions ?? [], SLUG_BY_TYPE));
	const summary = $derived(summarize(snapshot?.actions ?? []));
	const overviewEmpty = $derived(snapshot !== null && snapshot.actions.length === 0);

	function focusType(bucket: string) {
		table.filters.types = [bucket];
		table.setFilter('zoom', 'list');
	}

	function refresh() {
		if (table.filters.zoom === 'overview') void sweep();
		else table.refresh();
	}

	let deleteDialogOpen = $state(false);
	let actionToDelete = $state<ManagedAction | null>(null);

	const typeFilterItems = $derived.by(() => {
		const seen = new Set<number>();
		const items = getActionTypeOptions()
			.filter((o) => {
				if (o.value === 'COMPLIANCE_CHECK') return false;
				if (seen.has(o.type)) return false;
				seen.add(o.type);
				return true;
			})
			.map((o) => ({ id: o.value.toLowerCase(), label: o.label }));
		items.push({ id: COMPLIANCE_FILTER_ID, label: m.actions_type_compliance_check() });
		return items;
	});

	function setRange(startKey: DayKey, endKey: DayKey, v: { start?: DateValue; end?: DateValue }) {
		table.filters[startKey] = v.start?.toString() ?? '';
		table.setFilter(endKey, v.end?.toString() ?? '');
	}

	const anyFilterActive = $derived(
		table.query.length > 0 ||
			table.filters.types.length > 0 ||
			table.filters.unassigned ||
			table.filters.createdStart !== '' ||
			table.filters.createdEnd !== '' ||
			table.filters.updatedStart !== '' ||
			table.filters.updatedEnd !== ''
	);

	const sortOptions = [
		{ key: 'name' as const, label: m.actions_table_name() },
		{ key: 'type' as const, label: m.actions_table_type() },
		{ key: 'created' as const, label: m.actions_table_created() },
		{ key: 'updated' as const, label: m.actions_table_updated() }
	];

	function isComplianceAction(action: ManagedAction): boolean {
		return (
			action.type === ActionType.SHELL &&
			action.params.case === 'shell' &&
			action.params.value.isCompliance
		);
	}

	function getDisplayInfo(action: ManagedAction) {
		if (isComplianceAction(action)) {
			return getActionTypeInfoByValue('COMPLIANCE_CHECK');
		}
		return { label: getActionTypeLabel(action.type), icon: getActionTypeIcon(action.type) };
	}

	function confirmDelete(action: ManagedAction) {
		actionToDelete = action;
		deleteDialogOpen = true;
	}

	async function deleteAction() {
		if (!actionToDelete) return;
		try {
			await apiClient.deleteAction((actionToDelete.id?.value ?? ''));
			toast.success(m.actions_deleted());

			table.patchRows((rows) => rows.filter((a) => a.id?.value !== actionToDelete!.id?.value));
			table.refresh();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			deleteDialogOpen = false;
			actionToDelete = null;
		}
	}
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
				<Button onclick={() => goto('/actions/new')}>
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
		href={(a) => `${base}/actions/${a.id?.value ?? ''}`}
	>
		{#snippet filters()}
			<MultiSelectCombobox
				items={typeFilterItems}
				selected={table.filters.types}
				onSelectedChange={(next) => table.setFilter('types', next)}
				placeholder={m.actions_filter_all_types()}
				searchPlaceholder={m.common_search()}
				class="w-44"
			/>
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
					checked={table.filters.unassigned}
					onCheckedChange={(checked) => table.setFilter('unassigned', checked === true)}
				/>
				{m.actions_filter_unassigned()}
			</label>
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
					<DropdownMenu.Item onclick={() => confirmDelete(action)} class="text-destructive">
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
