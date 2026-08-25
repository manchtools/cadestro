<script lang="ts" generics="Row, K extends string">
	import type { Snippet } from 'svelte';
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { ArrowUp, ArrowDown, ArrowUpDown } from '@lucide/svelte';
	import type { TableView } from './list-state.svelte';

	interface Column {
		id: string;
		label: string;

		sortKey?: K;
		class?: string;
	}

	let {
		table,
		columns,
		row,
		rowKey,
		empty,
		loading: loadingSnippet,
		loadingRows = 5
	}: {
		table: TableView<Row, K>;
		columns: Column[];

		row: Snippet<[Row]>;
		rowKey: (row: Row) => string;

		empty?: Snippet;

		loading?: Snippet;
		loadingRows?: number;
	} = $props();
</script>

{#snippet headerRow()}
	<Table.Header>
		<Table.Row>
			{#each columns as col (col.id)}
				<Table.Head class={col.class}>
					{#if col.sortKey}
						{@const key = col.sortKey}
						<button
							class="flex items-center gap-1 hover:text-foreground"
							onclick={() => table.toggleSort(key)}
						>
							{col.label}
							{#if table.sortKey === key}
								{#if table.sortDir === 'asc'}<ArrowUp class="h-3 w-3" />{:else}<ArrowDown
										class="h-3 w-3"
									/>{/if}
							{:else}
								<ArrowUpDown class="h-3 w-3 opacity-50" />
							{/if}
						</button>
					{:else}
						{col.label}
					{/if}
				</Table.Head>
			{/each}
		</Table.Row>
	</Table.Header>
{/snippet}

<Card.Root>
	{#if table.loading && table.rows.length === 0}
		<Table.Root>
			{@render headerRow()}
			<Table.Body>
				{#if loadingSnippet}
					{@render loadingSnippet()}
				{:else}
					{#each Array(loadingRows) as _, i (i)}
						<Table.Row>
							{#each columns as col (col.id)}
								<Table.Cell><Skeleton class="h-4 w-full max-w-32" /></Table.Cell>
							{/each}
						</Table.Row>
					{/each}
				{/if}
			</Table.Body>
		</Table.Root>
	{:else if table.rows.length === 0}
		{@render empty?.()}
	{:else}
		<Table.Root>
			{@render headerRow()}
			<Table.Body>
				{#each table.rows as r (rowKey(r))}
					<Table.Row>
						{@render row(r)}
					</Table.Row>
				{/each}
			</Table.Body>
		</Table.Root>
	{/if}
</Card.Root>
