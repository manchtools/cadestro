<script lang="ts">
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Table from '$lib/components/ui/table';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import ItemTablePicker from '$lib/components/item-table-picker.svelte';
	import { ActionCreateForm, getActionTypeLabel } from '$lib/components/actions';
	import type { ManagedAction } from '$lib/sdk';
	import { Plus, ArrowLeft } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';

	interface Props {
		/** Whether the dialog is open */
		open?: boolean;
		/** Available actions to pick from */
		availableActions: ManagedAction[];
		/** Called when actions are selected (existing actions) */
		onSelect: (actionIds: string[]) => void;
		/** Called when a new action is created */
		onCreate: (action: ManagedAction) => void;
		/** Called when dialog is closed */
		onClose: () => void;
	}

	let {
		open = $bindable(false),
		availableActions = [],
		onSelect,
		onCreate,
		onClose
	}: Props = $props();

	let showCreateForm = $state(false);
	let selectedActionIds = $state<string[]>([]);

	// Reset state when dialog opens
	$effect(() => {
		if (open) {
			showCreateForm = false;
			selectedActionIds = [];
		}
	});

	function handleAddSelected() {
		if (selectedActionIds.length > 0) {
			onSelect(selectedActionIds);
			selectedActionIds = [];
			open = false;
		}
	}

	function handleCreated(action: ManagedAction) {
		onCreate(action);
		showCreateForm = false;
		open = false;
	}

	function handleClose() {
		open = false;
		onClose();
	}
</script>

<Dialog.Root bind:open onOpenChange={(isOpen) => !isOpen && onClose()}>
	<Dialog.Content class="sm:max-w-4xl max-h-[90vh] overflow-hidden flex flex-col">
		{#if showCreateForm}
			<div class="flex-1 overflow-y-auto p-1 m-2">
				<ActionCreateForm
					compact
					onCancel={() => (showCreateForm = false)}
					onCreated={handleCreated}
				/>
			</div>
		{:else}
			<Dialog.Header>
				<Dialog.Title>{m.action_picker_title()}</Dialog.Title>
				<Dialog.Description>
					{m.action_set_detail_add_actions_description()}
				</Dialog.Description>
			</Dialog.Header>

			<div class="flex justify-end mb-2">
				<Button variant="outline" size="sm" onclick={() => (showCreateForm = true)}>
					<Plus class="h-4 w-4 mr-2" />
					{m.action_picker_create_new()}
				</Button>
			</div>

			<div class="flex-1 overflow-y-auto">
				{#if availableActions.length === 0}
					<div class="py-8 text-center">
						<p class="text-muted-foreground">{m.action_set_detail_no_actions_available()}</p>
						<Button variant="outline" class="mt-4" onclick={() => (showCreateForm = true)}>
							<Plus class="h-4 w-4 mr-2" />
							{m.action_picker_create_new()}
						</Button>
					</div>
				{:else}
					<ItemTablePicker
						items={availableActions}
						bind:selected={selectedActionIds}
						searchPlaceholder={m.action_set_detail_search_actions()}
						emptyMessage={m.action_set_detail_no_actions_available()}
						searchFilter={(item, query) => {
							const a = item as ManagedAction;
							const q = query.toLowerCase();
							return a.name.toLowerCase().includes(q) || a.description.toLowerCase().includes(q);
						}}
					>
						{#snippet headerRow()}
							<Table.Head>{m.common_name()}</Table.Head>
							<Table.Head>{m.common_type()}</Table.Head>
						{/snippet}
						{#snippet itemRow(item)}
							{@const a = item as ManagedAction}
							<Table.Cell>
								<div>
									<span class="font-medium">{a.name}</span>
									{#if a.description}
										<p class="text-xs text-muted-foreground line-clamp-1">{a.description}</p>
									{/if}
								</div>
							</Table.Cell>
							<Table.Cell>
								<Badge variant="outline">{getActionTypeLabel(a.type)}</Badge>
							</Table.Cell>
						{/snippet}
					</ItemTablePicker>
				{/if}
			</div>

			<Dialog.Footer class="mt-4">
				<Button variant="outline" onclick={handleClose}>{m.common_cancel()}</Button>
				<Button onclick={handleAddSelected} disabled={selectedActionIds.length === 0}>
					{m.action_picker_add_selected()}
				</Button>
			</Dialog.Footer>
		{/if}
	</Dialog.Content>
</Dialog.Root>
