<script lang="ts">
 import { untrack } from 'svelte';
 import { toast } from 'svelte-sonner';
 import { api } from '$lib/api';
 import { Permission, type ManagedAction } from '$contract/cadestro/v1/control_pb';
 import { consoleContext } from '$lib/console-context.svelte';
 import { getLocalizedError } from '$lib/errors';
 import * as m from '$lib/paraglide/messages';
 import { Input } from '$lib/components/ui/input';
 import { Label } from '$lib/components/ui/label';
 import { Chip } from '$lib/components/fleet';
 import { Button } from '$lib/components/ui/button';
 import * as Dialog from '$lib/components/ui/dialog';
 import type { PillAction } from '$lib/shell/shell.svelte';
 import DesiredStateToggle from './desired-state-toggle.svelte';
 import PackageParamsForm from './forms/PackageParamsForm.svelte';
 import ShellParamsForm from './forms/ShellParamsForm.svelte';
 import FormSection from '$lib/components/create/form-section.svelte';
 import IdentityRow from '$lib/components/create/identity-row.svelte';
 import { actionChoice, getActionTypeInfoByValue } from './action-type';
 import { bindBuilderContext } from './pipeline/builder-pill.svelte';
 import { draftFromAction, draftErrors, hydrate, serialize } from '../../../routes/(app)/actions/new/draft';
 let { action, entityActions = [], onsaved }: { action: ManagedAction; entityActions?: PillAction[]; onsaved: (action: ManagedAction) => void } = $props();
 const { can } = consoleContext();
 let step = $state(untrack(() => draftFromAction(action)));
 let baseline = $state(untrack(() => serialize(step)));
 let saving = $state(false);
 let parked = $state(false);
 let scheduleOpen = $state(false);
 const dirty = $derived(serialize(step) !== baseline);
 const issues = $derived({ fields: draftErrors(step) });
 const info = $derived(getActionTypeInfoByValue(actionChoice(action)));
 const Icon = $derived(info.icon);
 const shownTypeLabel = $derived(info.label);
 const claimed = bindBuilderContext(untrack(() => `action:${action.id?.value ?? ''}`), () => {
  if (saving || parked) return null;
  return { route: `/actions/${action.id?.value ?? ''}`, title: step.name || action.name, dirty, valid: !Object.keys(issues.fields).length,
   commitLabel: m.common_save(), subtext: Object.values(issues.fields)[0] ?? m.action_detail_params_ready(),
   stashPayload: () => serialize($state.snapshot(step)), stashSubtitle: m.action_detail_params_stash_subtitle(),
   extraActions: [{ id: 'schedule', label: m.container_schedule_title(), onRun: () => { scheduleOpen = true; } }, ...entityActions],
   onCommit: () => void commit(), onCancel: () => { step = draftFromAction(action); }, onStash: () => { parked = true; }, onRestore: () => { parked = false; }
  };
 });
 step = hydrate(claimed) ?? untrack(() => step);
 export function hasUnsavedChanges() { return dirty; }
 async function commit() {
  if (!dirty || Object.keys(issues.fields).length) return;
  saving = true;
  let updated = action;
  try {
   if (step.name.trim() !== updated.name && can(Permission.RENAME_ACTION)) updated = (await api.renameAction({ id: updated.id, name: step.name.trim() })).action ?? updated;
   if (step.description.trim() !== updated.description && can(Permission.UPDATE_ACTION_DESCRIPTION)) updated = (await api.setActionDescription({ id: updated.id, description: step.description.trim() })).action ?? updated;
   if (can(Permission.UPDATE_ACTION_PARAMS)) updated = (await api.configureAction({ id: updated.id, desiredState: step.desiredState, timeoutSeconds: step.timeoutSeconds, schedule: step.schedule, params: step.params })).action ?? updated;
   step = draftFromAction(updated); baseline = serialize(step); onsaved(updated); toast.success(m.action_detail_params_updated());
  } catch (error) { onsaved(updated); baseline = serialize(draftFromAction(updated)); toast.error(getLocalizedError(error)); } finally { saving = false; }
 }
</script>
{#if step}

	<div class="mb-4 flex flex-wrap items-center gap-2 border-b border-hair pb-3">
		<div class="grid h-7 w-7 shrink-0 place-items-center rounded-md bg-accent-soft">
			<Icon class="h-4 w-4 text-accent-ink" />
		</div>
		<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">
			{m.action_detail_title()}
		</span>
		<Chip tone="info" label={shownTypeLabel} />
		<span class="ml-auto">

			<fieldset disabled={!can(Permission.UPDATE_ACTION_PARAMS)}><DesiredStateToggle bind:value={step.desiredState} supportsAbsent={step.params.case === 'package'} /></fieldset>
		</span>
	</div>

	<div class="grid gap-4 lg:grid-cols-[minmax(0,1fr)_17rem]">
		<div class="min-w-0 space-y-4">

			<IdentityRow
				nameDisabled={!can(Permission.RENAME_ACTION)} descriptionDisabled={!can(Permission.UPDATE_ACTION_DESCRIPTION)} idPrefix="action"
				nameLabel={m.common_name()}
				namePlaceholder={m.action_detail_name_placeholder()}
				bind:name={step.name}
				nameError={issues.fields.name}
				descriptionLabel={m.common_description()}
				descriptionPlaceholder={m.action_detail_description_placeholder()}
				bind:description={step.description}
			/>

			<FormSection title={m.action_detail_parameters()} lead>
                <fieldset disabled={!can(Permission.UPDATE_ACTION_PARAMS)}>
                {#if step.params.case === 'package'}<PackageParamsForm bind:params={step.params.value} errors={issues.fields} />{:else if step.params.case === 'shell'}<ShellParamsForm bind:params={step.params.value} errors={issues.fields} complianceOnly={step.params.value.isCompliance} />{:else}<p class="text-sm text-muted-foreground">Install available system updates during desired policy sync.</p>{/if}
                </fieldset>
			</FormSection>
		</div>

		<aside class="space-y-4 lg:border-l lg:border-hair lg:pl-4">
			<div class="space-y-1.5">
				<Label for="action-timeout">{m.action_detail_timeout_label()}</Label>
				<Input
					disabled={!can(Permission.UPDATE_ACTION_PARAMS)} id="action-timeout"
					type="number"
					min="1"
					max="3600"
					bind:value={step.timeoutSeconds}
				/>
			</div>

		</aside>
	</div>

	<Dialog.Root bind:open={scheduleOpen}>
		<Dialog.Content class="sm:max-w-2xl">
			<Dialog.Header>
				<Dialog.Title>{m.action_detail_schedule_title()}</Dialog.Title>
				<Dialog.Description>{m.action_detail_schedule_description()}</Dialog.Description>
			</Dialog.Header>
			<div class="py-2">
				{#if step.schedule}<Label for="scheduleInterval">{m.actions_schedule_interval()}</Label><Input disabled={!can(Permission.UPDATE_ACTION_PARAMS)} id="scheduleInterval" type="number" min="1" max="8760" bind:value={step.schedule.intervalHours} />{/if}
			</div>
			<Dialog.Footer>
				<Button variant="outline" onclick={() => (scheduleOpen = false)}>
					{m.common_done()}
				</Button>
			</Dialog.Footer>
		</Dialog.Content>
	</Dialog.Root>
{/if}
