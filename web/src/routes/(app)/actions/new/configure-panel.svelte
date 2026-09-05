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
	import DesiredStateToggle from '$lib/components/actions/desired-state-toggle.svelte';
	import { getActionTypeInfoByValue } from '$lib/components/actions/action-type';
	import PackageParamsForm from '$lib/components/actions/forms/PackageParamsForm.svelte';
	import ShellParamsForm from '$lib/components/actions/forms/ShellParamsForm.svelte';
	import type { ActionDraft } from './draft';

	let {
		draft = $bindable(),
		typeValue,
		errors,
		scheduleOpen = $bindable(false),
		onback
	}: {
		draft: ActionDraft;
		typeValue: string;
		errors: Record<string, string>;

		scheduleOpen?: boolean;
		onback: () => void;
	} = $props();


	const info = $derived(getActionTypeInfoByValue(typeValue));
	const supportsAbsent = $derived(draft.params.case === 'package');

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
					{#if draft.params.case === 'package'}<PackageParamsForm bind:params={draft.params.value} {errors} />{:else if draft.params.case === 'shell'}<ShellParamsForm bind:params={draft.params.value} {errors} complianceOnly={typeValue === 'COMPLIANCE_CHECK'} />{:else}<p class="text-sm text-muted-foreground">Install available system updates during desired policy sync.</p>{/if}
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

				<Button variant="outline" onclick={() => (scheduleOpen = true)}>{m.container_schedule_title()} · {draft.schedule?.intervalHours ?? 24} h</Button>
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
			{#if draft.schedule}<Label for="scheduleInterval">{m.actions_schedule_interval()}</Label><Input id="scheduleInterval" type="number" min="1" max="8760" bind:value={draft.schedule.intervalHours} />{/if}
		</div>
		<Dialog.Footer>
			<Button variant="outline" onclick={() => (scheduleOpen = false)}>{m.common_done()}</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
