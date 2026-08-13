<script lang="ts" generics="Row, K extends string">
	import { Button } from '$lib/components/ui/button';
	import * as Select from '$lib/components/ui/select';
	import { ChevronLeft, ChevronRight } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import type { TableView } from './list-state.svelte';

	let { table }: { table: TableView<Row, K> } = $props();
</script>

{#if table.total > 0}
	<div class="flex items-center justify-between">
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

		<div class="flex items-center gap-4">
			<span class="text-sm text-muted-foreground">
				{m.pagination_showing({
					from: String(table.showingFrom),
					to: String(table.showingTo),
					total: String(table.total)
				})}
			</span>
			<div class="flex items-center gap-1">
				<Button
					variant="outline"
					size="icon"
					class="h-8 w-8"
					onclick={() => table.gotoPage(table.page - 1)}
					disabled={table.page <= 1}
				>
					<ChevronLeft class="h-4 w-4" />
				</Button>
				<span class="text-sm px-2"
					>{m.pagination_page({ page: String(table.page), pages: String(table.totalPages) })}</span
				>
				<Button
					variant="outline"
					size="icon"
					class="h-8 w-8"
					onclick={() => table.gotoPage(table.page + 1)}
					disabled={table.page >= table.totalPages}
				>
					<ChevronRight class="h-4 w-4" />
				</Button>
			</div>
		</div>
	</div>
{/if}
