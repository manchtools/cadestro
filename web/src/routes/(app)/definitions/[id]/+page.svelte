<script lang="ts">
	// Definition detail = the same B1 pipeline builder, one level up: the ordered
	// steps are action SETS, and the palette is round 2's Movement C set picker.
	// Commit lives in the context pill; this page carries no Save button, and the
	// definition's own action — Delete — rides the same pill instead of a danger
	// zone below the builder.
	import { onMount } from 'svelte';
	import { getLocalizedError } from '$lib/errors';
	import { goto } from '$lib/navigation';
	import { page } from '$app/state';
	import { toast } from 'svelte-sonner';
	import { apiClient, type Definition, type ActionSet } from '$lib/sdk';
	import * as m from '$lib/paraglide/messages';
	import { Button } from '$lib/components/ui/button';
	import ConfirmDeleteDialog from '$lib/components/confirm-delete-dialog.svelte';
	import PageShell from '$lib/components/page-shell.svelte';
	import AssignmentsCard from '$lib/components/assignments-card.svelte';
	import ContainerScheduleCard from '$lib/components/container-schedule-card.svelte';
	import DefinitionBuilder from '$lib/components/actions/pipeline/definition-builder.svelte';
	import { Chip } from '$lib/components/fleet';
	import type { ActionSchedule } from '$sdk/powermanage/v1/actions_pb';
	import { AssignmentSourceType } from '$sdk/powermanage/v1/common_pb';
	import { ArrowLeft, RefreshCw } from '@lucide/svelte';
	import type { PillAction } from '$lib/shell/shell.svelte';

	let definition = $state<Definition | null>(null);
	let members = $state<{ actionSetId: string; sortOrder: number; actionSetName: string }[]>([]);
	let library = $state<ActionSet[]>([]);
	let loading = $state(true);
	let deleteDialogOpen = $state(false);
	// Remount key: a fresh load must give the builder a fresh baseline.
	let revision = $state(0);

	const defId = $derived(page.params.id ?? '');

	// The definition's own action, published on the builder's context so it shares
	// the pill with the commit. It keeps its confirm dialog.
	let scheduleOpen = $state(false);
	let assignOpen = $state(false);

	// The definition's OWN actions: schedule and assignment act on the whole
	// definition, not on anything inside the builder, so they belong beside the
	// commit rather than in cards below it. Delete keeps its confirm dialog and is
	// marked destructive so it reads apart from the two beside it.
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
			label: m.definitions_delete_definition(),
			tone: 'danger',
			onRun: () => (deleteDialogOpen = true)
		}
	];

	onMount(() => {
		if (defId) loadData();
	});

	async function loadData() {
		loading = true;
		try {
			const response = await apiClient.getDefinition(defId);
			definition = response.definition ?? null;
			members = response.members ?? [];
			const setsResponse = await apiClient.listActionSets();
			library = setsResponse.sets ?? [];
			revision++;
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			loading = false;
		}
	}

	async function deleteDef() {
		if (!defId) return;
		try {
			await apiClient.deleteDefinition(defId);
			toast.success(m.definitions_deleted());
			goto('/definitions');
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function updateSchedule(next: ActionSchedule) {
		if (!defId) return;
		try {
			definition = (await apiClient.updateDefinitionSchedule(defId, next)) ?? null;
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
				<h1 class="truncate text-2xl font-bold">{definition?.name ?? m.common_loading()}</h1>
				<p class="font-mono text-xs text-faint">{defId}</p>
			</div>
			{#if definition}
				<Chip tone="idle" label={m.definitions_count({ count: members.length })} />
			{/if}
			<Button variant="outline" onclick={loadData} disabled={loading}>
				<span class="mr-2 h-4 w-4" class:animate-spin={loading}>
					<RefreshCw class="h-4 w-4" />
				</span>
				{m.common_refresh()}
			</Button>
		</div>
	{/snippet}

	{#if loading && !definition}
		<div
			class="flex items-center justify-center rounded-xl border border-hair bg-surface py-12 shadow-plate"
		>
			<RefreshCw class="h-6 w-6 animate-spin text-muted-foreground" />
		</div>
	{:else if definition}
		{#key revision}
			<DefinitionBuilder
				{defId}
				{definition}
				{members}
				{library}
				{entityActions}
				onsaved={loadData}
			/>
		{/key}

		<ContainerScheduleCard
			schedule={definition.schedule}
			bind:editOpen={scheduleOpen}
			onsave={updateSchedule}
		/>

		<AssignmentsCard
			bind:assignOpen={assignOpen}
			sourceType={AssignmentSourceType.DEFINITION}
			sourceId={defId}
			title={m.assignments_title()}
			subtitle={m.assignments_subtitle_definition()}
			assignTitle={m.assignments_assign_definition()}
			assignDescription={m.assignments_assign_description_definition()}
		/>
	{/if}
</PageShell>

<ConfirmDeleteDialog
	bind:open={deleteDialogOpen}
	title={m.definitions_delete_dialog_title()}
	description={m.definitions_delete_dialog_description({ name: definition?.name ?? '' })}
	onconfirm={deleteDef}
/>
