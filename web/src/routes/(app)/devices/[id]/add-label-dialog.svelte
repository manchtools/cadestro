<script lang="ts">
	import { createFormValidation } from '$lib/forms';
	import { addLabelSchema } from '$lib/forms/schemas/devices';
	import { FieldError } from '$lib/components/ui/field-error';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as m from '$lib/paraglide/messages';

	interface Props {
		open: boolean;
		onadd: (key: string, value: string) => void;

		title?: string;
		description?: string;
		confirmLabel?: string;
	}

	let { open = $bindable(), onadd, title, description, confirmLabel }: Props = $props();

	let newLabelKey = $state('');
	let newLabelValue = $state('');
	const labelValidation = createFormValidation(addLabelSchema);

	$effect(() => {
		if (open) {
			labelValidation.clearErrors();
		}
	});

	function handleAdd() {
		labelValidation.handleSubmit({ key: newLabelKey, value: newLabelValue }, async () => {
			onadd(newLabelKey, newLabelValue);
			newLabelKey = '';
			newLabelValue = '';
		});
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content>
		<Dialog.Header>
			<Dialog.Title>{title ?? m.device_detail_add_label_dialog_title()}</Dialog.Title>
			<Dialog.Description>
				{description ?? m.device_detail_add_label_dialog_description()}
			</Dialog.Description>
		</Dialog.Header>
		<div class="space-y-4">
			<div class="space-y-2">
				<Label for="labelKey">{m.device_detail_label_key()}</Label>
				<Input id="labelKey" placeholder={m.device_detail_label_key_placeholder()} bind:value={newLabelKey} aria-invalid={!!labelValidation.errors.key} />
				<FieldError error={labelValidation.errors.key} />
			</div>
			<div class="space-y-2">
				<Label for="labelValue">{m.device_detail_label_value()}</Label>
				<Input id="labelValue" placeholder={m.device_detail_label_value_placeholder()} bind:value={newLabelValue} aria-invalid={!!labelValidation.errors.value} />
				<FieldError error={labelValidation.errors.value} />
			</div>
		</div>
		<Dialog.Footer>
			<Button variant="outline" onclick={() => (open = false)}>{m.common_cancel()}</Button>
			<Button data-testid="add-label-confirm" onclick={handleAdd}>
				{confirmLabel ?? m.device_detail_add_label()}
			</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
