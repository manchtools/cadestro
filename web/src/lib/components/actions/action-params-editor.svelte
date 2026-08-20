<script lang="ts">
	// Inline, pill-committed parameter editor for ONE action — the surface the
	// action detail page uses instead of a modal with its own Save button.
	//
	// It shares every per-type mechanism with `edit-params-dialog.svelte` (which
	// the device-tab action sheet still uses): ACTION_REGISTRY for defaults,
	// schema and proto conversion, and ActionParamsFormDispatch for the 19-arm
	// form ladder. Neither file hand-lists action types, so the two cannot drift
	// the way the pre-registry orchestrators did.
	import type { Component } from 'svelte';
	import { toast } from 'svelte-sonner';
	import { apiClient, type ManagedAction } from '$lib/sdk';
	import { getLocalizedError } from '$lib/errors';
	import * as m from '$lib/paraglide/messages';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Chip } from '$lib/components/fleet';
	import { Button } from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import type { PillAction } from '$lib/shell/shell.svelte';
	import { setUserLoaders, apiUserLoaders } from './forms/user-loader-context.svelte';
	import ActionParamsFormDispatch from './action-params/ActionParamsFormDispatch.svelte';
	import ActionScheduleForm from './forms/ActionScheduleForm.svelte';
	import DesiredStateToggle from './desired-state-toggle.svelte';
	import FormSection from '$lib/components/create/form-section.svelte';
	import IdentityRow from '$lib/components/create/identity-row.svelte';
	import { scheduleFormToProto } from './forms/types';
	import { ACTION_REGISTRY } from './registry';
	import { ActionType } from '$contract/cadestro/v1/actions_pb';
	import {
		getActionTypeIcon,
		getActionTypeLabel,
		getActionTypeInfoByValue
	} from './action-type';
	import { bindBuilderContext } from './pipeline/builder-pill.svelte';
	import { stepFromAction, validateStep, type StepDraft } from './pipeline/step-draft';

	let {
		action,
		typeLabel = undefined,
		typeIcon = undefined,
		entityActions = [],
		onsaved
	}: {
		action: ManagedAction;
		/** The strip that names the action's type lives HERE, with the draft, so
		 *  its state chip can be the live toggle instead of a read-only echo of a
		 *  value the operator then had to change somewhere else. Derived from the
		 *  action when the caller does not override it. */
		typeLabel?: string;
		typeIcon?: Component;
		/** The page's own actions for this entity (assign, delete). They ride THIS
		 *  context because the pill has one slot: a page that entered its own
		 *  context alongside this editor would be overwritten by it, then dropped
		 *  when the editor went clean — the pill flickering back to nav. */
		entityActions?: PillAction[];
		onsaved: (updated: ManagedAction) => void;
	} = $props();

	let scheduleOpen = $state(false);

	setUserLoaders(apiUserLoaders);

	// The body as the SERVER holds it. Mutable, because a successful save makes a
	// new one true: the detail page does not remount this editor when a save
	// returns, so a construction-time constant would keep describing the action as
	// it was before the save — and `dirty` would stay true over stored work,
	// leaving the pill saying "something changed" and Save armed to re-send it.
	// svelte-ignore state_referenced_locally
	let base = $state<StepDraft | null>(stepFromAction(action, 0));

	// SCRIPT_RUN has no editable params shape at all; the page
	// renders the read-only display instead of this editor.
	// svelte-ignore state_referenced_locally
	let step = $state<StepDraft | null>(base ? (JSON.parse(JSON.stringify(base)) as StepDraft) : null);
	// svelte-ignore state_referenced_locally
	let baseline = $state(base ? JSON.stringify($state.snapshot(base)) : '');

	let committing = $state(false);
	let parked = $state(false);

	const dirty = $derived(!!step && JSON.stringify($state.snapshot(step)) !== baseline);
	const issues = $derived(
		step ? validateStep(step, m.action_set_detail_builder_name_required()) : { fields: {}, first: null }
	);
	const adapter = $derived(step ? ACTION_REGISTRY[step.formKey] : null);
	/** Capitalised binding so the markup can render it; `{@const}` is illegal
	 *  as a direct child of a plain element. */
	const compliance = $derived(
		action.type === ActionType.SHELL &&
			action.params.case === 'shell' &&
			action.params.value.isCompliance
	);
	const Icon = $derived(
		typeIcon ??
			(compliance ? getActionTypeInfoByValue('COMPLIANCE_CHECK').icon : getActionTypeIcon(action.type))
	);
	const shownTypeLabel = $derived(
		typeLabel ??
			(compliance
				? getActionTypeInfoByValue('COMPLIANCE_CHECK').label
				: getActionTypeLabel(action.type))
	);

	// svelte-ignore state_referenced_locally
	// svelte-ignore state_referenced_locally
	const claimed = bindBuilderContext(`action:${action.id}`, () => {
		// Held whenever this editor is mounted, not only while dirty: the pill is
		// the action's action bar, and Schedule / Assign / Delete cannot live on a
		// context that only exists after you have typed. `parked` and `committing`
		// still end the hold so Stash sticks and a save is not double-fired.
		if (!step || committing || parked) return null;
		const blocked = issues.first !== null;
		return {
			// This editor's home. Without it the store refuses to park a draft and
			// the pill hides Stash — so an action's unsaved parameters were the one
			// buffer in the app that could not be set aside.
			route: `/actions/${action.id}`,
			stashPayload: () => $state.snapshot(step),
			title: step.name.trim() || action.name,
			dirty,
			valid: !blocked,
			extraActions: [
				{
					id: 'schedule',
					label: m.container_schedule_title(),
					onRun: () => (scheduleOpen = true)
				},
				...entityActions
			],
			commitLabel: m.common_save(),
			subtext: blocked
				? `${m.action_set_detail_builder_blocked({ count: 1 })} · ${issues.first}`
				: m.action_detail_params_ready(),
			subtextTone: blocked ? ('warn' as const) : ('neutral' as const),
			stashSubtitle: m.action_detail_params_stash_subtitle(),
			onCommit: commit,
			onCancel: discard,
			onStash: () => (parked = true),
			onRestore: () => (parked = false)
		};
	});

	// A draft parked on the stage outranks the loaded body: it is the operator's
	// unsaved work and the ONLY copy of it — this editor's buffer is component
	// state that the unmount destroyed. The baseline stays the SERVER's body, so
	// `dirty` still means "diverges from what is stored", not "from the card".
	if (claimed) step = claimed as StepDraft;

	function discard() {
		step = base ? (JSON.parse(JSON.stringify(base)) as StepDraft) : null;
	}

	async function commit() {
		const current = step;
		if (!current || !adapter) return;
		committing = true;
		try {
			if (current.name.trim() !== base?.name) {
				await apiClient.renameAction(action.id, current.name.trim());
			}
			if (current.description.trim() !== base?.description) {
				await apiClient.updateActionDescription(action.id, current.description.trim());
			}
			const updated = await apiClient.updateActionParams({
				id: action.id,
				desiredState: adapter.supportsAbsent ? current.desiredState : 0,
				timeoutSeconds: current.timeoutSeconds,
				schedule: scheduleFormToProto(current.schedule),
				params: {
					case: adapter.paramsCase,
					value: adapter.formToProto(current.params)
				} as Parameters<typeof apiClient.updateActionParams>[0]['params']
			});
			toast.success(m.action_detail_params_updated());
			if (updated) {
				// Rebase on what the server now holds. The operator's buffer is NOT
				// overwritten: a keystroke that landed while the save was in flight is
				// unstored work, and it stays dirty rather than being swallowed. The
				// client-side key is identity, not state, so it carries over.
				const saved = stepFromAction(updated, 0);
				if (saved) {
					if (step) saved.key = step.key;
					base = saved;
					baseline = JSON.stringify(saved);
				}
				onsaved(updated);
			}
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			committing = false;
		}
	}
</script>

{#if step && adapter}
	<!-- The SAME working surface as /actions/new: what the action does fills the
	     main column, what merely qualifies it sits in a rail. Editing an action
	     and creating one were two different layouts for the same object. -->
	<div class="mb-4 flex flex-wrap items-center gap-2 border-b border-hair pb-3">
		<div class="grid h-7 w-7 shrink-0 place-items-center rounded-md bg-accent-soft">
			<Icon class="h-4 w-4 text-accent-ink" />
		</div>
		<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">
			{m.action_detail_title()}
		</span>
		<Chip tone="info" label={shownTypeLabel} />
		<span class="ml-auto">
			<!-- The state IS the control. It used to be a read-only chip here and a
			     dropdown in the rail — the operator clicked the chip and nothing
			     happened. -->
			<DesiredStateToggle bind:value={step.desiredState} supportsAbsent={adapter.supportsAbsent} />
		</span>
	</div>

	<div class="grid gap-4 lg:grid-cols-[minmax(0,1fr)_17rem]">
		<div class="min-w-0 space-y-4">
			<!-- The same opening row as the create surface. The description used to
			     be a paragraph on the page with its own pencil and its own dialog and
			     its own RPC — one more commit path beside the pill, for the one field
			     create asks for in the same breath as the name. -->
			<IdentityRow
				idPrefix="action"
				nameLabel={m.common_name()}
				namePlaceholder={m.action_detail_name_placeholder()}
				bind:name={step.name}
				nameError={issues.fields.name}
				descriptionLabel={m.common_description()}
				descriptionPlaceholder={m.action_detail_description_placeholder()}
				bind:description={step.description}
			/>

			<FormSection title={m.action_detail_parameters()} lead>
				<ActionParamsFormDispatch
					formKey={step.formKey}
					bind:params={step.params}
					errors={issues.fields}
				/>
			</FormSection>
		</div>

		<aside class="space-y-4 lg:border-l lg:border-hair lg:pl-4">
			<div class="space-y-1.5">
				<Label for="action-timeout">{m.action_detail_timeout_label()}</Label>
				<Input
					id="action-timeout"
					type="number"
					min="1"
					max="3600"
					bind:value={step.timeoutSeconds}
				/>
			</div>

		</aside>
	</div>

	<!-- The schedule is the ACTION's, not a parameter of it, so it is one of the
	     entity's pill actions rather than a section the operator scrolls past on
	     the way to Save. It edits the same draft, so it commits with everything
	     else from the one pill. -->
	<Dialog.Root bind:open={scheduleOpen}>
		<Dialog.Content class="sm:max-w-2xl">
			<Dialog.Header>
				<Dialog.Title>{m.action_detail_schedule_title()}</Dialog.Title>
				<Dialog.Description>{m.action_detail_schedule_description()}</Dialog.Description>
			</Dialog.Header>
			<div class="py-2">
				<ActionScheduleForm bind:params={step.schedule} />
			</div>
			<Dialog.Footer>
				<Button variant="outline" onclick={() => (scheduleOpen = false)}>
					{m.common_done()}
				</Button>
			</Dialog.Footer>
		</Dialog.Content>
	</Dialog.Root>
{/if}
