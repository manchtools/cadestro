<script lang="ts" generics="Row, K extends string">

	import type { Snippet } from 'svelte';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { ArrowUp, ArrowDown } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import type { TableView } from './list-state.svelte';

	interface SortOption {
		key: K;

		label: string;
	}

	let {
		table,
		row,
		rowKey,
		href,
		rowEnd,
		sortOptions,
		filters,
		empty,
		loadingRows = 5
	}: {
		table: TableView<Row, K>;

		row: Snippet<[Row]>;
		rowKey: (row: Row) => string;

		href?: (row: Row) => string;

		rowEnd?: Snippet<[Row]>;

		sortOptions?: readonly SortOption[];

		filters?: Snippet;

		empty?: Snippet;
		loadingRows?: number;
	} = $props();

	const ROW_BODY = 'flex min-h-11 min-w-0 flex-1 items-center gap-3 px-3 py-2';
</script>

{#snippet rowShell(item: Row)}
	<div
		data-testid="row-list-row"
		data-row-key={rowKey(item)}
		class="flex items-stretch border-t border-hair first:border-t-0 hover:bg-sunken/50"
	>
		{#if href}
			<a
				data-testid="row-list-link"
				href={href(item)}
				class="{ROW_BODY} rounded-[10px] focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring"
			>
				{@render row(item)}
			</a>
		{:else}
			<div class={ROW_BODY}>{@render row(item)}</div>
		{/if}
		{#if rowEnd}
			<div class="flex shrink-0 items-center gap-1 pr-2">{@render rowEnd(item)}</div>
		{/if}
	</div>
{/snippet}

<div data-testid="row-list" class="overflow-hidden rounded-xl border border-border bg-surface">
	{#if filters || (sortOptions && sortOptions.length > 0)}
		<div
			data-testid="row-list-toolbar"
			class="flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-hair px-3 py-1.5"
		>
			{#if filters}
				<div data-testid="row-list-filters" class="flex min-w-0 flex-1 flex-wrap items-center gap-2">
					{@render filters()}
				</div>
			{/if}
			{#if sortOptions && sortOptions.length > 0}
				<div data-testid="row-list-sort" class="ml-auto flex items-center gap-2">
					<span class="font-mono text-[0.62rem] uppercase tracking-[0.08em] text-faint">
						{m.list_sort_label()}
					</span>
				<div
						class="inline-flex overflow-hidden rounded-lg border border-border font-mono text-[0.68rem]"
					>
						{#each sortOptions as option (option.key)}
							{@const active = table.sortKey === option.key}
							<button
								type="button"
								aria-pressed={active}
								title={active
									? table.sortDir === 'asc'
										? m.list_sort_asc()
										: m.list_sort_desc()
									: undefined}
								onclick={() => table.toggleSort(option.key)}
								class="flex items-center gap-1 border-r border-border px-2 py-1 last:border-r-0 {active
									? 'bg-accent-soft font-semibold text-accent-ink'
									: 'text-muted-foreground hover:text-foreground'}"
							>
								{option.label}
								{#if active}
									{#if table.sortDir === 'asc'}<ArrowUp class="h-3 w-3" />{:else}<ArrowDown
											class="h-3 w-3"
										/>{/if}
								{/if}
							</button>
						{/each}
					</div>
				</div>
			{/if}
		</div>
	{/if}

	{#if table.loading && table.rows.length === 0}
		<div>
			{#each Array(loadingRows) as _, i (i)}
				<div class="flex min-h-11 items-center gap-3 border-t border-hair px-3 py-2 first:border-t-0">
					<Skeleton class="h-6 w-6 shrink-0 rounded-md" />
					<Skeleton class="h-3.5 w-40" />
					<Skeleton class="ml-auto h-3 w-24" />
				</div>
			{/each}
		</div>
	{:else if table.error}
 <p role="alert" class="px-3 py-6 text-sm text-destructive">{table.error}</p>
	{:else if table.rows.length === 0}
		{@render empty?.()}
	{:else}
		<div>
			{#each table.rows as r (rowKey(r))}
				{@render rowShell(r)}
			{/each}
		</div>
	{/if}
</div>
