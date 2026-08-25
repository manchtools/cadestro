<script lang="ts">

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
