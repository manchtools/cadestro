<script lang="ts">
	import * as Popover from '$lib/components/ui/popover';
	import * as Command from '$lib/components/ui/command';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Button } from '$lib/components/ui/button';
	import { ChevronsUpDown } from '@lucide/svelte';
	import { cn } from '$lib/utils';
	import * as m from '$lib/paraglide/messages';

	type Item = {
		id: string;
		label: string;
		description?: string;
	};

	let {
		items = [],
		selected = $bindable<string[]>([]),
		onSelectedChange,
		placeholder = 'Select items...',
		searchPlaceholder = 'Search...',
		class: className
	}: {
		items: Item[];
		selected: string[];
		/** Optional callback fired only on user-interaction toggles (not on
		 *  external state updates from a parent's bind:selected). Intended
		 *  for URL-as-state syncing where the caller must distinguish
		 *  user-driven changes from state-set-from-URL to avoid history
		 *  loops. */
		onSelectedChange?: (value: string[]) => void;
		placeholder?: string;
		searchPlaceholder?: string;
		class?: string;
	} = $props();

	let open = $state(false);
	let search = $state('');

	const selectedCount = $derived(selected.length);
	const buttonLabel = $derived(
		selectedCount === 0
			? placeholder
			: selectedCount === 1
				? items.find((i) => i.id === selected[0])?.label ?? `${selectedCount} selected`
				: `${selectedCount} selected`
	);

	function toggle(id: string) {
		const next = selected.includes(id) ? selected.filter((s) => s !== id) : [...selected, id];
		selected = next;
		onSelectedChange?.(next);
	}
</script>

<Popover.Root bind:open>
	<Popover.Trigger>
		<Button
			variant="outline"
			role="combobox"
			aria-expanded={open}
			class={cn('w-full justify-between font-normal', className)}
		>
			<span class="truncate">{buttonLabel}</span>
			<ChevronsUpDown class="ml-2 h-4 w-4 shrink-0 opacity-50" />
		</Button>
	</Popover.Trigger>
	<Popover.Content class="w-(--bits-popover-anchor-width) p-0">
		<Command.Root shouldFilter={true} bind:value={search}>
			<Command.Input placeholder={searchPlaceholder} />
			<Command.List>
				<Command.Empty>{m.common_no_results_search()}</Command.Empty>
				{#each items as item (item.id)}
					{@const isSelected = selected.includes(item.id)}
					<Command.Item
						value={item.label}
						onSelect={() => toggle(item.id)}
					>
						<Checkbox checked={isSelected} class="pointer-events-none" />
						<div class="flex flex-col">
							<span>{item.label}</span>
							{#if item.description}
								<span class="text-xs text-muted-foreground">{item.description}</span>
							{/if}
						</div>
					</Command.Item>
				{/each}
			</Command.List>
		</Command.Root>
	</Popover.Content>
</Popover.Root>
