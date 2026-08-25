<script lang="ts" module>
	import * as m from '$lib/paraglide/messages';

	export function formatSyncInterval(minutes: number): string {
		if (minutes === 0) return m.sync_default();
		if (minutes < 60) return `${minutes} min`;
		if (minutes === 60) return m.sync_1hour();
		if (minutes < 1440) return `${minutes / 60} hours`;
		if (minutes === 1440) return m.sync_1day();
		return `${minutes / 1440} days`;
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
				<Label for="syncInterval">{m.sync_label()}</Label>
				<Select.Root type="single" bind:value={selectedValue}>
					<Select.Trigger class="w-full">
						{formatSyncInterval(parseInt(selectedValue, 10))}
					</Select.Trigger>
					<Select.Content>
						<Select.Item value="0">{m.sync_default()}</Select.Item>
						<Select.Item value="5">{m.sync_5min()}</Select.Item>
						<Select.Item value="10">{m.sync_10min()}</Select.Item>
						<Select.Item value="15">{m.sync_15min()}</Select.Item>
						<Select.Item value="30">{m.sync_30min()}</Select.Item>
						<Select.Item value="60">{m.sync_1hour()}</Select.Item>
						<Select.Item value="120">{m.sync_2hours()}</Select.Item>
						<Select.Item value="360">{m.sync_6hours()}</Select.Item>
						<Select.Item value="720">{m.sync_12hours()}</Select.Item>
						<Select.Item value="1440">{m.sync_1day()}</Select.Item>
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
