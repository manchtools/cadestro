<script lang="ts">
	// Action-set detail = B1's pipeline builder. The page itself is a thin shell:
	// it loads the set, its members and the action library, hands them to
	// <ActionSetBuilder>, and keeps the surfaces the builder does NOT own — the
	// container schedule, assignments and dispatch.
	//
	// There is deliberately no Save button anywhere on this page: the builder
	// publishes its commit to the context pill (⌘S / Stash / Esc), and the SET's
	// own action — Delete — rides the same pill instead of a danger zone below
	// three cards.
	import { onMount } from 'svelte';
	import { getLocalizedError } from '$lib/errors';
	import { goto } from '$lib/navigation';
	import { page } from '$app/state';
	import { toast } from 'svelte-sonner';
	import { apiClient, type ActionSet, type ManagedAction } from '$lib/sdk';
	import type { ActionSetMember } from '$contract/cadestro/v1/control_pb';
	import { Button } from '$lib/components/ui/button';
	import ConfirmDeleteDialog from '$lib/components/confirm-delete-dialog.svelte';
	import PageShell from '$lib/components/page-shell.svelte';
	import AssignmentsCard from '$lib/components/assignments-card.svelte';
	import ContainerScheduleCard from '$lib/components/container-schedule-card.svelte';
	import DispatchToDeviceDialog from '$lib/components/dispatch-to-device-dialog.svelte';
	import ActionSetBuilder from '$lib/components/actions/pipeline/action-set-builder.svelte';
	import { Chip } from '$lib/components/fleet';
	import type { ActionSchedule } from '$contract/cadestro/v1/actions_pb';
	import { AssignmentSourceType } from '$contract/cadestro/v1/common_pb';
	import { ArrowLeft, RefreshCw } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import type { PillAction } from '$lib/shell/shell.svelte';

	let actionSet = $state<ActionSet | null>(null);
	let members = $state<ActionSetMember[]>([]);
	let library = $state<ManagedAction[]>([]);
	let loading = $state(true);
	let deleteDialogOpen = $state(false);
	let dispatchDialogOpen = $state(false);
	// Bumped after every reload so the builder remounts on a fresh baseline
	// instead of trying to reconcile a committed draft against new server state.
	let revision = $state(0);

	const setId = $derived(page.params.id ?? '');

	let scheduleOpen = $state(false);
	let assignOpen = $state(false);

	// The set's OWN actions, published on the builder's context so they share the
	// pill with the commit: schedule and assignment act on the whole set, not on
	// anything inside the builder. Delete keeps its confirm dialog — the pill is a
	// shorter route to the gate, not a way past it — and is marked destructive so
	// it can never be mistaken for the two beside it.
	const entityActions: PillAction[] = [
		{
			id: 'schedule',
			label: m.container_schedule_title(),
			onRun: () => (scheduleOpen = true)
		},
		{
			id: 'assignments',
			label: m.common_assign(),
			onRun: () => (assignOpen = true)
		},
		{
			id: 'delete',
			label: m.action_sets_delete_action_set(),
			tone: 'danger',
			onRun: () => (deleteDialogOpen = true)
		}
	];

	onMount(() => {
		if (setId) loadData();
	});

	async function loadData() {
		loading = true;
		try {
			const response = await apiClient.getActionSet(setId);
			actionSet = response.set ?? null;
			members = response.members ?? [];
			const actionsResponse = await apiClient.listActions();
			library = actionsResponse.actions ?? [];
			revision++;
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			loading = false;
		}
	}

	async function deleteSet() {
		if (!setId) return;
		try {
			await apiClient.deleteActionSet(setId);
			toast.success(m.action_sets_deleted());
			goto('/action-sets');
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function updateSchedule(next: ActionSchedule) {
		if (!setId) return;
		try {
			actionSet = (await apiClient.updateActionSetSchedule(setId, next)) ?? null;
			toast.success(m.container_schedule_updated());
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
			throw error;
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
				<h1 class="truncate text-2xl font-bold">{actionSet?.name ?? m.common_loading()}</h1>
				<p class="font-mono text-xs text-faint">{setId}</p>
			</div>
			{#if actionSet}
				<Chip tone="idle" label={m.action_sets_count({ count: members.length })} />
			{/if}
			<Button variant="outline" onclick={() => (dispatchDialogOpen = true)} disabled={!actionSet}>
				{m.dispatch_to_device_run_button()}
			</Button>
			<Button variant="outline" onclick={loadData} disabled={loading}>
				<span class="mr-2 h-4 w-4" class:animate-spin={loading}>
					<RefreshCw class="h-4 w-4" />
				</span>
				{m.common_refresh()}
			</Button>
		</div>
	{/snippet}

	{#if loading && !actionSet}
		<div
			class="flex items-center justify-center rounded-xl border border-hair bg-surface py-12 shadow-plate"
		>
			<RefreshCw class="h-6 w-6 animate-spin text-muted-foreground" />
		</div>
	{:else if actionSet}
		{#key revision}
			<ActionSetBuilder
				{setId}
				set={actionSet}
				{members}
				{library}
				{entityActions}
				onsaved={loadData}
			/>
		{/key}

		<ContainerScheduleCard
			schedule={actionSet.schedule}
			bind:editOpen={scheduleOpen}
			onsave={updateSchedule}
		/>

		<AssignmentsCard
			bind:assignOpen={assignOpen}
			sourceType={AssignmentSourceType.ACTION_SET}
			sourceId={setId}
			title={m.assignments_title()}
			subtitle={m.assignments_subtitle_action_set()}
			assignTitle={m.assignments_assign_action_set()}
			assignDescription={m.assignments_assign_description_action_set()}
		/>
	{/if}
</PageShell>

<ConfirmDeleteDialog
	bind:open={deleteDialogOpen}
	title={m.action_sets_delete_dialog_title()}
	description={m.action_sets_delete_dialog_description({ name: actionSet?.name ?? '' })}
	onconfirm={deleteSet}
/>

<DispatchToDeviceDialog
	bind:open={dispatchDialogOpen}
	title={m.dispatch_to_device_action_set_title()}
	actionSetId={setId}
/>
