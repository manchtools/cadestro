<script lang="ts">
	// Action detail. The parameter form is no longer behind a modal with its own
	// Save button: it is inline, and its commit rides the context pill (⌘S /
	// Stash / Esc) like every other committable surface in the redesign.
	//
	// Action types with no registry entry (SCRIPT_RUN) have no
	// editable params shape at all — those fall back to the read-only display
	// rather than showing an empty editor.
	import { onMount, onDestroy, untrack } from 'svelte';
	import {
		shell,
		enterContext,
		exitContext,
		type PillAction
	} from '$lib/shell/shell.svelte';
	import { getLocalizedError } from '$lib/errors';
	import { goto } from '$lib/navigation';
	import { page } from '$app/state';
	import { toast } from 'svelte-sonner';
	import { apiClient, type ManagedAction } from '$lib/sdk';
	import { DesiredState, AssignmentSourceType } from '$contract/cadestro/v1/common_pb';
	import { ActionType } from '$contract/cadestro/v1/actions_pb';
	import { Button } from '$lib/components/ui/button';
	import ConfirmDeleteDialog from '$lib/components/confirm-delete-dialog.svelte';
	import PageShell from '$lib/components/page-shell.svelte';
	import AssignmentsCard from '$lib/components/assignments-card.svelte';
	import DispatchToDeviceDialog from '$lib/components/dispatch-to-device-dialog.svelte';
	import { Chip } from '$lib/components/fleet';
	import { formKeyFromActionType } from '$lib/components/actions/registry';
	import { ArrowLeft, RefreshCw, Clock } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import ActionParamsEditor from '$lib/components/actions/action-params-editor.svelte';
	import ScheduleSummary from '$lib/components/actions/schedule-summary.svelte';
	import {
		ActionParamsDisplay,
		getActionTypeIcon,
		getActionTypeInfoByValue,
		getActionTypeLabel
	} from '$lib/components/actions';

	let action = $state<ManagedAction | null>(null);
	let loading = $state(true);
	let deleteDialogOpen = $state(false);
	let dispatchDialogOpen = $state(false);
	// Remount key for the editor: a reload must rebase its draft on the new body.
	let revision = $state(0);

	const actionId = $derived(page.params.id ?? '');
	let assignOpen = $state(false);

	// The action's own actions. They are handed to the params EDITOR rather than
	// entered here: the pill has one context slot and the editor already owns
	// `action:<id>`, so a second owner on this page would be overwritten by it and
	// then dropped when it went clean — the pill flickering back to nav.
	const entityActions: PillAction[] = [
		// The header's lone ← was the last piece of page chrome competing with the
		// bar; the way out belongs with the entity's other actions.
		{ id: 'back', label: m.common_back(), onRun: () => history.back() },
		{ id: 'assignments', label: m.common_assign(), onRun: () => (assignOpen = true) },
		{
			id: 'delete',
			label: m.action_detail_delete_action(),
			tone: 'danger',
			onRun: () => (deleteDialogOpen = true)
		}
	];

	// Action types with no editable params render the read-only display instead of
	// the editor, so nothing would hold the pill. This page takes it for those.
	const contextId = $derived(`action:${actionId}`);
	$effect(() => {
		const a = action;
		if (!a || editable) return;
		untrack(() =>
			enterContext({
				id: contextId,
				title: a.name,
				dirty: false,
				valid: true,
				commitLabel: m.common_save(),
				onCommit: () => {},
				extraActions: entityActions
			})
		);
	});
	onDestroy(() => {
		if (shell.pill.context?.id === contextId) exitContext();
	});

	const isCompliance = $derived(
		!!action &&
			action.type === ActionType.SHELL &&
			action.params.case === 'shell' &&
			action.params.value.isCompliance
	);
	/** Whether this action type has an editable params form at all. */
	const editable = $derived(!!action && formKeyFromActionType(action.type) !== null);

	onMount(() => {
		if (actionId) loadActionData();
	});

	async function loadActionData() {
		if (!actionId) return;
		loading = true;
		try {
			action = (await apiClient.getAction(actionId)) ?? null;
			revision++;
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			loading = false;
		}
	}

	async function deleteAction() {
		if (!actionId) return;
		try {
			await apiClient.deleteAction(actionId);
			toast.success(m.actions_deleted());
			goto('/actions');
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}
</script>

<PageShell contentClass="space-y-6">
	{#snippet header()}
		<div class="flex items-center gap-4">
			<Button variant="ghost" size="icon" onclick={() => history.back()}>
				<ArrowLeft class="h-4 w-4" />
			</Button>
			<div class="min-w-0 flex-1">
				<h1 class="truncate text-2xl font-bold">{action?.name ?? m.common_loading()}</h1>
				<p class="font-mono text-xs text-faint">{actionId}</p>
			</div>
			<Button variant="outline" onclick={() => (dispatchDialogOpen = true)} disabled={!action}>
				{m.dispatch_to_device_run_button()}
			</Button>
			<Button variant="outline" onclick={loadActionData} disabled={loading}>
				<span class="mr-2 h-4 w-4" class:animate-spin={loading}>
					<RefreshCw class="h-4 w-4" />
				</span>
				{m.common_refresh()}
			</Button>
		</div>
	{/snippet}

	{#if loading && !action}
		<div
			class="flex items-center justify-center rounded-xl border border-hair bg-surface py-12 shadow-plate"
		>
			<RefreshCw class="h-6 w-6 animate-spin text-muted-foreground" />
		</div>
	{:else if action}
		{@const Icon = isCompliance
			? getActionTypeInfoByValue('COMPLIANCE_CHECK').icon
			: getActionTypeIcon(action.type)}
		{@const absent = action.desiredState === DesiredState.ABSENT}
		{#if !editable}
			<!-- Types with no editable params (SCRIPT_RUN) have no
			     draft to toggle, so their strip stays a statement. The editable ones
			     carry theirs inside the editor, where the state chip IS the switch. -->
			<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
				<div class="flex flex-wrap items-center gap-2">
					<div class="grid h-7 w-7 shrink-0 place-items-center rounded-md bg-accent-soft">
						<Icon class="h-4 w-4 text-accent-ink" />
					</div>
					<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">
						{m.action_detail_title()}
					</span>
					<Chip
						tone="info"
						label={isCompliance
							? getActionTypeInfoByValue('COMPLIANCE_CHECK').label
							: getActionTypeLabel(action.type)}
					/>
					<Chip
						tone={absent ? 'crit' : 'ok'}
						label={absent ? m.desired_state_absent() : m.desired_state_present()}
					/>
				</div>
			</section>
		{/if}

		<section class="rounded-xl border border-hair bg-surface p-4 shadow-plate">
			<!-- No heading here when the editor is mounted: it publishes its own
			     Parameters section, and the card was announcing it a second time. -->
			<div>
				{#if editable}
					{#key revision}
						<ActionParamsEditor
							{action}
								{entityActions}
							onsaved={(updated) => (action = updated)}
						/>
					{/key}
				{:else}
					<div class="space-y-4">
						<div class="flex items-center gap-2">
							<Clock class="h-4 w-4 text-faint" />
							<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">
								{m.action_detail_parameters()}
							</span>
						</div>
						<ActionParamsDisplay params={action.params} />
						<ScheduleSummary schedule={action.schedule} />
					</div>
				{/if}
			</div>
		</section>

		<AssignmentsCard
			bind:assignOpen={assignOpen}
			sourceType={AssignmentSourceType.ACTION}
			sourceId={actionId}
			title={m.action_detail_assignments()}
			subtitle={m.action_detail_assignments_description()}
			assignTitle={m.action_detail_assign_action()}
			assignDescription={m.action_detail_assign_description()}
		/>
	{/if}
</PageShell>

<ConfirmDeleteDialog
	bind:open={deleteDialogOpen}
	title={m.actions_delete_dialog_title()}
	description={m.actions_delete_dialog_description({ name: action?.name ?? '' })}
	onconfirm={deleteAction}
/>

<DispatchToDeviceDialog
	bind:open={dispatchDialogOpen}
	title={m.dispatch_to_device_action_title()}
	actionId={actionId}
/>
