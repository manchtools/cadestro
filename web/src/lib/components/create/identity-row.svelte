<script lang="ts">
	// The identity row every create/edit surface opens with: a name beside a
	// description.
	//
	// It is a COMPONENT rather than a copy-pasted pair of divs because that
	// copy-paste is exactly how these forms drifted apart — the pipeline builders
	// paired them, every other form stacked them full-width, and the field
	// wrappers picked up three different spacings along the way. One name is
	// never worth the whole plate's width, and a stack of two full-width inputs
	// is the shape that made the create surfaces read as a questionnaire.
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
		/** A description that is genuinely prose gets a textarea and its own row;
		 *  a one-line summary sits beside the name. */
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
