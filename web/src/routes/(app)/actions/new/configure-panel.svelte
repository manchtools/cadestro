<script lang="ts">

	import { ArrowLeft } from '@lucide/svelte';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Button } from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import { FieldError } from '$lib/components/ui/field-error';
	import FormSection from '$lib/components/create/form-section.svelte';
	import IdentityRow from '$lib/components/create/identity-row.svelte';
	import * as m from '$lib/paraglide/messages';
	import ActionParamsFormDispatch from '$lib/components/actions/action-params/ActionParamsFormDispatch.svelte';
	import ActionScheduleForm from '$lib/components/actions/forms/ActionScheduleForm.svelte';
	import DesiredStateToggle from '$lib/components/actions/desired-state-toggle.svelte';
	import ScheduleSummary from '$lib/components/actions/schedule-summary.svelte';
	import { scheduleFormToProto } from '$lib/components/actions/forms/types';
	import { ACTION_REGISTRY, type FormKey } from '$lib/components/actions/registry';
	import { getActionTypeInfoByValue } from '$lib/components/actions/action-type';
	import type { ActionDraft } from './draft';

	let {
		draft = $bindable(),
		typeValue,
		formKey,
		errors,
		scheduleOpen = $bindable(false),
		onback
	}: {
		draft: ActionDraft;
		typeValue: string;
		formKey: FormKey;
		errors: Record<string, string>;

		scheduleOpen?: boolean;
		onback: () => void;
	} = $props();

	const scheduleProto = $derived(scheduleFormToProto(draft.schedule));

	const info = $derived(getActionTypeInfoByValue(typeValue));
	const supportsAbsent = $derived(ACTION_REGISTRY[formKey].supportsAbsent);

</script>

<div class="mx-auto flex w-full max-w-5xl flex-col gap-3" data-testid="action-configure">
	<div class="flex items-center gap-2">
		<button
			type="button"
			data-testid="action-configure-back"
			onclick={onback}
			class="flex items-center gap-1.5 rounded-full border px-2.5 py-1.5 text-xs text-muted-foreground hover:bg-accent/50"
		>
			<ArrowLeft class="h-3.5 w-3.5" />
			{m.actions_new_change_type()}
		</button>
	</div>

	<div class="rounded-xl border border-hair bg-surface shadow-plate">
		<div class="flex items-center gap-2.5 border-b px-3 py-2.5">
			{#if info}
				{@const Icon = info.icon}
				<span class="grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-accent-soft text-accent-ink">
					<Icon class="h-4 w-4" />
				</span>
			{/if}
			<div class="min-w-0 flex-1">
				<h1 class="truncate text-sm font-semibold">
					{m.action_detail_new_action({ type: info.label })}
				</h1>
				<p class="truncate text-xs text-muted-foreground">{info.description}</p>
			</div>

			<DesiredStateToggle bind:value={draft.desiredState} {supportsAbsent} />
		</div>

		<div class="grid gap-4 p-4 lg:grid-cols-[minmax(0,1fr)_17rem]">
			<div class="min-w-0 space-y-4">
				<IdentityRow
					idPrefix="action"
					nameLabel={m.common_name()}
					namePlaceholder={m.action_detail_name_placeholder()}
					bind:name={draft.name}
					nameError={errors.name}
					descriptionLabel={m.common_description()}
					descriptionPlaceholder={m.action_detail_description_placeholder()}
					bind:description={draft.description}
				/>

				<FormSection title={m.action_detail_parameters()}>
					<ActionParamsFormDispatch {formKey} bind:params={draft.params} {errors} />
				</FormSection>
			</div>

			<aside class="space-y-4 lg:border-l lg:border-hair lg:pl-4">
				<div class="space-y-1.5 lg:max-w-none">
					<Label for="action-timeout">{m.action_detail_timeout_label()}</Label>
					<Input
						id="action-timeout"
						type="number"
						min="1"
						max="3600"
						bind:value={draft.timeoutSeconds}
						aria-invalid={!!errors.timeoutSeconds}
					/>
					<FieldError error={errors.timeoutSeconds} />
				</div>

				<ScheduleSummary schedule={scheduleProto} onedit={() => (scheduleOpen = true)} />
			</aside>
		</div>
	</div>
</div>

<Dialog.Root bind:open={scheduleOpen}>
	<Dialog.Content class="sm:max-w-2xl">
		<Dialog.Header>
			<Dialog.Title>{m.action_detail_schedule_title()}</Dialog.Title>
			<Dialog.Description>{m.action_detail_schedule_description()}</Dialog.Description>
		</Dialog.Header>
		<div class="py-2">
			<ActionScheduleForm bind:params={draft.schedule} />
		</div>
		<Dialog.Footer>
			<Button variant="outline" onclick={() => (scheduleOpen = false)}>{m.common_done()}</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
