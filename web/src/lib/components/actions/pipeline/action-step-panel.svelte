<script lang="ts">
	// The selected step's full configuration. This is where the 16+ existing
	// per-type param forms are EMBEDDED, unchanged: ActionParamsFormDispatch owns
	// the FormKey ladder, so this panel renders one component for every action
	// type and none of the forms were rewritten for the builder.
	//
	// The field set is exactly the one edit-params-dialog offers (timeout,
	// desired state, params, schedule) plus the step's own name/description,
	// which the dialog could not reach — so the builder is a superset, not a
	// reduction, of what the operator could edit before.
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import { FieldError } from '$lib/components/ui/field-error';
	import * as Select from '$lib/components/ui/select';
	import * as m from '$lib/paraglide/messages';
	import ActionParamsFormDispatch from '../action-params/ActionParamsFormDispatch.svelte';
	import { ACTION_REGISTRY } from '../registry';
	import DesiredStateToggle from '../desired-state-toggle.svelte';
	import { Chip } from '$lib/components/fleet';
	import { base } from '$app/paths';
	import { SquareArrowOutUpRight } from '@lucide/svelte';
	import { getActionTypeInfo } from '../action-type';
	import type { StepDraft } from './step-draft';

	// `errors` is DERIVED live from the step's registry schema on every keystroke,
	// so there is no `onclearerror` seam here: a field stops being an error the
	// moment its value validates, with no imperative clearing to get out of sync.
	// `step` is $bindable because the per-type params forms bind INTO it; without
	// the declared binding Svelte flags the write as crossing an ownership
	// boundary it never agreed to.
	let {
		step = $bindable(),
		index,
		errors
	}: {
		step: StepDraft;
		index: number;
		errors: Record<string, string>;
	} = $props();

	const adapter = $derived(ACTION_REGISTRY[step.formKey]);
	const info = $derived(getActionTypeInfo(step.actionType));
	// COMPLIANCE_CHECK validates against its own schema but shares the SHELL
	// params bucket — the dispatch component keys on the FormKey either way.
</script>

<div data-testid="step-panel" class="rounded-xl border border-hair bg-surface">
	<div class="flex items-baseline gap-2 border-b px-3 py-2">
		<span class="font-mono text-[0.62rem] uppercase tracking-[0.1em] text-faint">
			{m.action_set_detail_builder_step_n({ n: index + 1 })}
		</span>
		<span class="truncate text-sm font-semibold">{info.label}</span>
		<span class="ml-auto shrink-0 font-mono text-[0.62rem] text-faint">
			{step.actionId || m.action_set_detail_builder_unsaved()}
		</span>
	</div>

	<div class="max-h-[36rem] space-y-4 overflow-y-auto p-3">
		{#if step.isNew}
			<!-- A step being AUTHORED here: the action does not exist yet, so this is
			     the only place to give it its shape. -->
			<div class="space-y-1.5">
				<Label for="step-name-{step.key}">{m.common_name()}</Label>
				<Input id="step-name-{step.key}" bind:value={step.name} aria-invalid={!!errors.name} />
				<FieldError error={errors.name} />
			</div>

			<div class="space-y-1.5">
				<Label for="step-desc-{step.key}">{m.common_description()}</Label>
				<Textarea id="step-desc-{step.key}" bind:value={step.description} rows={2} />
			</div>

			<div class="grid gap-3 sm:grid-cols-2">
				<div class="space-y-1.5">
					<Label for="step-timeout-{step.key}">{m.action_detail_timeout_label()}</Label>
					<Input
						id="step-timeout-{step.key}"
						type="number"
						min="1"
						max="3600"
						bind:value={step.timeoutSeconds}
					/>
				</div>
				<div class="space-y-1.5">
					<Label>{m.action_detail_desired_state()}</Label>
					<DesiredStateToggle
						bind:value={step.desiredState}
						supportsAbsent={adapter.supportsAbsent}
					/>
				</div>
			</div>

			<div class="space-y-3 border-t pt-3">
				<h4 class="text-sm font-medium">{m.action_detail_parameters()}</h4>
				<ActionParamsFormDispatch formKey={step.formKey} bind:params={step.params} {errors} />
			</div>
		{:else}
			<!-- An EXISTING action is a REFERENCE here, never an edit surface.
			     This panel used to write name, description, params and desired state
			     straight onto the shared action: flipping a step to REMOVE inside one
			     set silently armed an uninstall everywhere that action was assigned.
			     `ActionSetMember` carries only action_id and sort_order, so there is
			     nowhere set-local to hold an override — which makes "edit it here"
			     necessarily a global edit, and an accidental one.
			     The set owns membership and order; the action owns itself. -->
			<div class="space-y-1.5">
				<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">
					{m.common_name()}
				</span>
				<p class="truncate text-sm font-medium" data-testid="step-ref-name">{step.name}</p>
			</div>

			{#if step.description}
				<p class="text-sm text-muted-foreground">{step.description}</p>
			{/if}

			<div class="flex flex-wrap items-center gap-2">
				<Chip tone="info" label={info.label} />
				<Chip
					tone={step.desiredState === 1 ? 'crit' : 'ok'}
					label={step.desiredState === 1 ? m.desired_state_absent() : m.desired_state_present()}
				/>
			</div>

			<p class="text-xs text-muted-foreground" data-testid="step-ref-note">
				{m.action_set_builder_member_readonly()}
			</p>

			<a
				href="{base}/actions/{step.actionId}"
				data-testid="step-ref-link"
				class="inline-flex items-center gap-1.5 rounded-lg border border-hair px-2.5 py-1.5 text-xs hover:bg-accent/50"
			>
				<SquareArrowOutUpRight class="h-3.5 w-3.5" />
				{m.action_set_builder_open_action()}
			</a>
		{/if}
	</div>
</div>
