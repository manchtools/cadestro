<script lang="ts">
	// Step 2 of /actions/new: one plate carrying the whole action.
	//
	// B1's rule — the plate has NO button bar. Save, Cancel and Stash are the
	// context pill's, and the validation rollup is the pill's subtext, so nothing
	// here duplicates a commit affordance.
	//
	// The per-type parameter forms are EMBEDDED, unchanged, through the same two
	// components the pipeline's step panel uses: ActionParamsFormDispatch owns the
	// FormKey ladder and ActionScheduleForm owns the schedule, so no per-type form
	// was rewritten for this surface.
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

	// `draft` is $bindable because the per-type params forms bind INTO it; without
	// the declared binding Svelte flags the write as crossing an ownership
	// boundary it never agreed to. `errors` is DERIVED live from the registry
	// schema by the page, so a field stops being an error the moment its value
	// validates — there is no imperative clearing to fall out of sync.
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
		/** Opened by the page's pill Schedule action — the schedule is the action's,
		 *  not one more box to scroll past on the way to the parameters. */
		scheduleOpen?: boolean;
		onback: () => void;
	} = $props();

	/** The draft's schedule as the display component reads it. */
	const scheduleProto = $derived(scheduleFormToProto(draft.schedule));

	const info = $derived(getActionTypeInfoByValue(typeValue));
	const supportsAbsent = $derived(ACTION_REGISTRY[formKey].supportsAbsent);

</script>

<!-- Wider than a create PLATE because this is not a plate: it is the pipeline
     builders' working-surface grammar — a main column carrying the substance and
     a rail carrying what qualifies it. Capped so a one-word field like "Package
     Name" still never spans the viewport. -->
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

	<!-- A working surface, not a questionnaire: what the action DOES fills the
		     main column, and the settings that merely qualify it (how long it may
		     run, when it repeats) sit in a rail beside it. Stacked full width, the
		     parameters — the only part the operator came here to write — were three
		     scrolls down past two boxes they rarely touch. -->
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

				<!-- The schedule STATES itself here and is edited from the pill, exactly
					     as it is on an existing action. It rides the same draft, so it
					     still commits with everything else from one ⌘S. -->
				<ScheduleSummary schedule={scheduleProto} onedit={() => (scheduleOpen = true)} />
			</aside>
		</div>
	</div>
</div>

<!-- Opened by the pill's Schedule action — one commit path, no second Save. -->
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
