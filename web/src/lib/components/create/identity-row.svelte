<script lang="ts">

	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import { FieldError } from '$lib/components/ui/field-error';

	let {
		idPrefix,
		nameLabel,
		namePlaceholder,
		name = $bindable(''),
		nameError,
		descriptionLabel,
		descriptionPlaceholder,
		description = $bindable(''),
		descriptionError,

		descriptionMultiline = false
	}: {
		idPrefix: string;
		nameLabel: string;
		namePlaceholder?: string;
		name?: string;
		nameError?: string;
		descriptionLabel: string;
		descriptionPlaceholder?: string;
		description?: string;
		descriptionError?: string;
		descriptionMultiline?: boolean;
	} = $props();
</script>

{#snippet nameField()}
	<div class="space-y-1.5">
		<Label for="{idPrefix}-name">{nameLabel}</Label>
		<Input
			id="{idPrefix}-name"
			placeholder={namePlaceholder}
			bind:value={name}
			class="font-medium"
			aria-invalid={!!nameError}
		/>
		<FieldError error={nameError} />
	</div>
{/snippet}

{#if descriptionMultiline}
	<div class="space-y-3">
		{@render nameField()}
		<div class="space-y-1.5">
			<Label for="{idPrefix}-description">{descriptionLabel}</Label>
			<Textarea
				id="{idPrefix}-description"
				placeholder={descriptionPlaceholder}
				bind:value={description}
				rows={2}
				aria-invalid={!!descriptionError}
			/>
			<FieldError error={descriptionError} />
		</div>
	</div>
{:else}
	<div class="grid gap-3 sm:grid-cols-2">
		{@render nameField()}
		<div class="space-y-1.5">
			<Label for="{idPrefix}-description">{descriptionLabel}</Label>
			<Input
				id="{idPrefix}-description"
				placeholder={descriptionPlaceholder}
				bind:value={description}
				aria-invalid={!!descriptionError}
			/>
			<FieldError error={descriptionError} />
		</div>
	</div>
{/if}
