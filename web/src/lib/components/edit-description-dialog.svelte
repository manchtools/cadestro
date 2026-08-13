<script lang="ts">
	import { Input } from '$lib/components/ui/input';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import * as m from '$lib/paraglide/messages';

	interface Props {
		open: boolean;
		value: string;
		onsave: () => void;
	}

	let { open = $bindable(), value = $bindable(), onsave }: Props = $props();
</script>

<AlertDialog.Root bind:open>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{m.edit_description_title()}</AlertDialog.Title>
		</AlertDialog.Header>
		<div class="py-4">
			<Input bind:value placeholder={m.common_description()} />
		</div>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>{m.common_cancel()}</AlertDialog.Cancel>
			<AlertDialog.Action onclick={onsave}>{m.common_save()}</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
