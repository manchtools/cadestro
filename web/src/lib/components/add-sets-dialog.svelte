<script lang="ts">
	import { type ActionSet } from '$lib/sdk';
	import { Button } from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Table from '$lib/components/ui/table';
	import ItemTablePicker from '$lib/components/item-table-picker.svelte';
	import * as m from '$lib/paraglide/messages';

	interface Props {
		open: boolean;
		availableSets: ActionSet[];
		onadd: (setIds: string[]) => void;
	}

	let { open = $bindable(), availableSets, onadd }: Props = $props();

	let selectedSetIds = $state<string[]>([]);

	$effect(() => {
		if (open) {
			selectedSetIds = [];
		}
	});

	function handleAdd() {
		onadd(selectedSetIds);
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content class="max-w-2xl">
		<Dialog.Header>
			<Dialog.Title>{m.definition_detail_add_sets_title()}</Dialog.Title>
			<Dialog.Description>
				{m.definition_detail_add_sets_description()}
			</Dialog.Description>
		</Dialog.Header>
		<div class="py-4">
			<ItemTablePicker
				items={availableSets}
				bind:selected={selectedSetIds}
				searchPlaceholder={m.definition_detail_search_sets()}
				emptyMessage={m.definition_detail_no_sets_available()}
				searchFilter={(item, query) => {
					const s = /** @type {import('$lib/sdk').ActionSet} */ (item);
					const q = query.toLowerCase();
					return s.name.toLowerCase().includes(q) || s.description.toLowerCase().includes(q);
				}}
			>
				{#snippet headerRow()}
					<Table.Head>{m.common_name()}</Table.Head>
					<Table.Head>{m.action_set_detail_members_label()}</Table.Head>
				{/snippet}
				{#snippet itemRow(item)}
					{@const s = /** @type {import('$lib/sdk').ActionSet} */ (item)}
					<Table.Cell>
						<div>
							<span class="font-medium">{s.name}</span>
							{#if s.description}
								<p class="text-xs text-muted-foreground">{s.description}</p>
							{/if}
						</div>
					</Table.Cell>
					<Table.Cell>
						<span class="text-muted-foreground">{m.action_sets_count({ count: s.memberCount })}</span>
					</Table.Cell>
				{/snippet}
			</ItemTablePicker>
		</div>
		<Dialog.Footer>
			<Button variant="outline" onclick={() => (open = false)}>{m.common_cancel()}</Button>
			<Button onclick={handleAdd} disabled={selectedSetIds.length === 0}>{m.common_add()}</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
