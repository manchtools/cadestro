<script lang="ts" module>
	import * as m from '$lib/paraglide/messages';

	export function formatInventoryInterval(minutes: number): string {
		if (minutes === 0) return m.inventory_interval_inherit();
		if (minutes < 1440) return `${minutes / 60} h`;
		if (minutes === 1440) return m.inventory_interval_1day();
		return `${minutes / 1440} ${m.inventory_interval_days_suffix()}`;
	}
</script>

<script lang="ts">
	import { untrack } from 'svelte';
	import { Button } from '$lib/components/ui/button';
	import { Label } from '$lib/components/ui/label';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Select from '$lib/components/ui/select';

	interface Props {
		open: boolean;
		currentMinutes: number;
		title: string;
		description: string;
		onsave: (minutes: number) => void;
	}

	let { open = $bindable(), currentMinutes, title, description, onsave }: Props = $props();

	let selectedValue = $state('0');

	function reset() {
		selectedValue = currentMinutes.toString();
	}

	$effect(() => {
		if (open) untrack(reset);
	});

	function handleSave() {
		onsave(parseInt(selectedValue, 10));
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content>
		<Dialog.Header>
			<Dialog.Title>{title}</Dialog.Title>
			<Dialog.Description>
				{description}
			</Dialog.Description>
		</Dialog.Header>
		<div class="space-y-4">
			<div class="space-y-2">
				<Label for="inventoryInterval">{m.inventory_interval_label()}</Label>
				<Select.Root type="single" bind:value={selectedValue}>
					<Select.Trigger class="w-full">
						{formatInventoryInterval(parseInt(selectedValue, 10))}
					</Select.Trigger>
					<Select.Content>

						<Select.Item value="0">{m.inventory_interval_inherit()}</Select.Item>
						<Select.Item value="120">2 h</Select.Item>
						<Select.Item value="240">4 h</Select.Item>
						<Select.Item value="360">6 h</Select.Item>
						<Select.Item value="720">12 h</Select.Item>
						<Select.Item value="1440">{m.inventory_interval_1day()}</Select.Item>
						<Select.Item value="2880">2 {m.inventory_interval_days_suffix()}</Select.Item>
						<Select.Item value="10080">7 {m.inventory_interval_days_suffix()}</Select.Item>
					</Select.Content>
				</Select.Root>
			</div>
		</div>
		<Dialog.Footer>
			<Button variant="outline" onclick={() => (open = false)}>{m.common_cancel()}</Button>
			<Button onclick={handleSave}>{m.common_save()}</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
