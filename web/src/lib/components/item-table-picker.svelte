<script lang="ts">
	import * as Table from '$lib/components/ui/table';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Input } from '$lib/components/ui/input';
	import { Search } from '@lucide/svelte';
	import type { Snippet } from 'svelte';
	import * as m from '$lib/paraglide/messages';

	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	type Item = Record<string, any> & { id: string };

	let {
		items = [],
		selected = $bindable<string[]>([]),
		searchPlaceholder = m.common_search(),
		emptyMessage = '',
		searchFilter,
		headerRow,
		itemRow
	}: {
		items: Item[];
		selected: string[];
		searchPlaceholder?: string;
		emptyMessage?: string;
		searchFilter: (item: Item, query: string) => boolean;
		headerRow: Snippet;
		itemRow: Snippet<[Item, boolean]>;
	} = $props();

	let searchQuery = $state('');

	const filtered = $derived(
		searchQuery ? items.filter((item) => searchFilter(item, searchQuery)) : items
	);

	const allSelected = $derived(
		filtered.length > 0 && filtered.every((item) => selected.includes(item.id))
	);
	const someSelected = $derived(
		filtered.some((item) => selected.includes(item.id)) && !allSelected
	);

	function toggle(id: string) {
		if (selected.includes(id)) {
			selected = selected.filter((s) => s !== id);
		} else {
			selected = [...selected, id];
		}
	}

	function toggleAll() {
		if (allSelected) {
			const filteredIds = new Set(filtered.map((item) => item.id));
			selected = selected.filter((id) => !filteredIds.has(id));
		} else {
			const existing = new Set(selected);
			for (const item of filtered) {
				existing.add(item.id);
			}
			selected = [...existing];
		}
	}
</script>

<div class="space-y-3">
	<div class="relative">
		<Search class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
		<Input
			placeholder={searchPlaceholder}
			bind:value={searchQuery}
			class="pl-9"
		/>
	</div>

	{#if filtered.length === 0}
		<p class="py-6 text-center text-sm text-muted-foreground">
			{searchQuery ? m.common_no_results_search() : emptyMessage}
		</p>
	{:else}
		<div class="max-h-64 overflow-y-auto rounded-md border">
			<Table.Root>
				<Table.Header>
					<Table.Row>
						<Table.Head class="w-10">
							<Checkbox
								checked={allSelected}
								indeterminate={someSelected}
								onCheckedChange={toggleAll}
							/>
						</Table.Head>
						{@render headerRow()}
					</Table.Row>
				</Table.Header>
				<Table.Body>
					{#each filtered as item (item.id)}
						{@const isSelected = selected.includes(item.id)}
						<Table.Row
							data-state={isSelected ? 'selected' : undefined}
							class="cursor-pointer"
							onclick={() => toggle(item.id)}
						>
							<Table.Cell>
								<Checkbox
									checked={isSelected}
									onCheckedChange={() => toggle(item.id)}
									onclick={(e: MouseEvent) => e.stopPropagation()}
								/>
							</Table.Cell>
							{@render itemRow(item, isSelected)}
						</Table.Row>
					{/each}
				</Table.Body>
			</Table.Root>
		</div>
		<p class="text-xs text-muted-foreground">
			{m.picker_selected({ count: selected.length })}
		</p>
	{/if}
</div>
