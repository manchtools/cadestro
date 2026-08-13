<script lang="ts">
	import { Input } from '$lib/components/ui/input';
	import { FieldError } from '$lib/components/ui/field-error';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import * as m from '$lib/paraglide/messages';

	interface Props {
		open: boolean;
		value: string;
		placeholder?: string;
		onsave: () => void;
		error?: string;
		onclearerror?: () => void;
	}

	let { open = $bindable(), value = $bindable(), placeholder, onsave, error, onclearerror }: Props =
		$props();
</script>

<AlertDialog.Root bind:open>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{m.edit_name_title()}</AlertDialog.Title>
		</AlertDialog.Header>
		<div class="py-4 space-y-2">
			<Input bind:value {placeholder} aria-invalid={!!error} oninput={() => onclearerror?.()} />
			<FieldError {error} />
		</div>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>{m.common_cancel()}</AlertDialog.Cancel>
			<AlertDialog.Action onclick={onsave}>{m.common_save()}</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
