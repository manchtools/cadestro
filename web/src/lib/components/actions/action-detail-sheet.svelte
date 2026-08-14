<script lang="ts" module>
	import { pushState } from '$lib/navigation';
	import { getLocalizedError } from '$lib/errors';

	/**
	 * Open the action detail sheet via shallow routing.
	 * Call from any page that includes <ActionDetailSheet />.
	 */
	export function openActionSheet(actionId: string) {
		pushState(`/actions/${actionId}`, { actionSheet: actionId });
	}
</script>

<script lang="ts">
	import { page } from '$app/state';
	import { toast } from 'svelte-sonner';
	import { apiClient, type ManagedAction } from '$lib/sdk';
	import { DesiredState } from '$contract/cadestro/v1/common_pb';
	import { ActionType } from '$contract/cadestro/v1/actions_pb';
	import { Button } from '$lib/components/ui/button';
	import { Label } from '$lib/components/ui/label';
	import { Badge } from '$lib/components/ui/badge';
	import * as Sheet from '$lib/components/ui/sheet';
	import EditNameDialog from '$lib/components/edit-name-dialog.svelte';
	import EditDescriptionDialog from '$lib/components/edit-description-dialog.svelte';
	import EditParamsDialog from './edit-params-dialog.svelte';
	import { supportsAbsent } from './edit-params-dialog.svelte';
	import { createFormValidation } from '$lib/forms';
	import { editNameSchema } from '$lib/forms/schemas/common';
	import { RefreshCw, Pencil, Clock, ExternalLink } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import {
		ActionTypeBadge,
		ActionParamsDisplay,
		ActionScheduleDisplay,
		getActionTypeIcon,
		getActionTypeInfoByValue
	} from '$lib/components/actions';

	import type { Snippet } from 'svelte';

	interface Props {
		onupdated?: () => void;
		children?: Snippet;
	}

	let { onupdated, children }: Props = $props();

	// Derive state from shallow routing
	let actionId = $derived(page.state.actionSheet);
	let sheetOpen = $derived(!!actionId);

	let action = $state<ManagedAction | null>(null);
	let loading = $state(false);
	let editNameDialogOpen = $state(false);
	let editDescDialogOpen = $state(false);
	let editParamsDialogOpen = $state(false);
	let newName = $state('');
	let newDescription = $state('');

	const nameValidation = createFormValidation(editNameSchema);

	$effect(() => {
		if (sheetOpen && actionId) {
			loadAction();
		}
		if (!sheetOpen) {
			action = null;
		}
	});

	async function loadAction() {
		if (!actionId) return;
		loading = true;
		try {
			action = (await apiClient.getAction(actionId)) ?? null;
			if (action) {
				newName = action.name;
				newDescription = action.description;
			}
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			loading = false;
		}
	}

	async function updateName() {
		if (!actionId) return;
		const valid = nameValidation.validate({ name: newName.trim() });
		if (!valid) return;
		try {
			action = (await apiClient.renameAction(actionId, newName.trim())) ?? null;
			editNameDialogOpen = false;
			toast.success(m.action_detail_name_updated());
			onupdated?.();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function updateDescription() {
		if (!actionId) return;
		try {
			action = (await apiClient.updateActionDescription(actionId, newDescription.trim())) ?? null;
			editDescDialogOpen = false;
			toast.success(m.action_detail_desc_updated());
			onupdated?.();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	function handleOpenChange(isOpen: boolean) {
		if (!isOpen) {
			history.back();
		}
	}
</script>

<Sheet.Root open={sheetOpen} onOpenChange={handleOpenChange}>
	<Sheet.Content class="sm:max-w-2xl w-full !p-0 !gap-0 flex flex-col">
		<Sheet.Header class="px-6 pt-6 pb-4 border-b shrink-0">
			{#if action}
				{@const isCompliance = action.type === ActionType.SHELL && action.params.case === 'shell' && action.params.value.isCompliance}
				{@const Icon = isCompliance ? getActionTypeInfoByValue('COMPLIANCE_CHECK').icon : getActionTypeIcon(action.type)}
				<div class="flex items-center gap-3">
					<div class="rounded-md bg-primary/10 p-2 shrink-0">
						<Icon class="h-5 w-5 text-primary" />
					</div>
					<div class="min-w-0 flex-1">
						<Sheet.Title class="truncate">{action.name}</Sheet.Title>
						<Sheet.Description class="truncate text-xs">{actionId}</Sheet.Description>
					</div>
				</div>
			{:else}
				<Sheet.Title>{m.common_loading()}</Sheet.Title>
			{/if}
		</Sheet.Header>

		{#if loading && !action}
			<div class="flex items-center justify-center py-12">
				<RefreshCw class="h-6 w-6 animate-spin text-muted-foreground" />
			</div>
		{:else if action}
			{@const isCompliance = action.type === ActionType.SHELL && action.params.case === 'shell' && action.params.value.isCompliance}
			<div class="flex-1 overflow-y-auto px-6 py-6 space-y-6">
				<!-- Details Section -->
				<div class="space-y-3">
					<h3 class="text-sm font-semibold text-muted-foreground uppercase tracking-wide">{m.action_detail_title()}</h3>
					<div class="space-y-3">
						<div>
							<div class="flex items-center justify-between">
								<Label class="text-muted-foreground">{m.common_name()}</Label>
								<Button
									variant="ghost"
									size="sm"
									class="h-6 w-6 p-0"
									aria-label={m.edit_name_title()}
									onclick={() => {
										newName = action?.name ?? '';
										nameValidation.clearErrors();
										editNameDialogOpen = true;
									}}
								>
									<Pencil class="h-3 w-3" />
								</Button>
							</div>
							<p class="font-medium text-sm">{action.name}</p>
						</div>
						<div>
							<div class="flex items-center justify-between">
								<Label class="text-muted-foreground">{m.common_description()}</Label>
								<Button
									variant="ghost"
									size="sm"
									class="h-6 w-6 p-0"
									aria-label={m.edit_description_title()}
									onclick={() => {
										newDescription = action?.description ?? '';
										editDescDialogOpen = true;
									}}
								>
									<Pencil class="h-3 w-3" />
								</Button>
							</div>
							<p class="text-sm">{action.description || m.common_no_description()}</p>
						</div>
						<div>
							<Label class="text-muted-foreground">{m.action_detail_type()}</Label>
							<div class="mt-1 flex items-center gap-2">
								<ActionTypeBadge type={action.type} {isCompliance} />
								{#if supportsAbsent(action.type)}
									<Badge variant={action.desiredState === DesiredState.ABSENT ? 'destructive' : 'default'}>
										{action.desiredState === DesiredState.ABSENT ? m.desired_state_absent() : m.desired_state_present()}
									</Badge>
								{/if}
							</div>
						</div>
						<div>
							<Label class="text-muted-foreground">{m.action_detail_timeout()}</Label>
							<p class="mt-1 text-sm font-medium">{m.action_detail_timeout_value({ seconds: action.timeoutSeconds.toString() })}</p>
						</div>
					</div>
				</div>

				<hr />

				<!-- Parameters Section -->
				<div class="space-y-3">
					<div class="flex items-center justify-between">
						<h3 class="text-sm font-semibold text-muted-foreground uppercase tracking-wide">{m.action_detail_parameters()}</h3>
						<Button variant="ghost" size="sm" onclick={() => (editParamsDialogOpen = true)}>
							<Pencil class="h-3 w-3 mr-1" />
							{m.common_edit()}
						</Button>
					</div>
					<ActionParamsDisplay params={action.params} />
				</div>

				<hr />

				<!-- Schedule Section -->
				<div class="space-y-3">
					<div class="flex items-center justify-between">
						<h3 class="text-sm font-semibold text-muted-foreground uppercase tracking-wide flex items-center gap-2">
							<Clock class="h-3.5 w-3.5" />
							{m.action_detail_schedule()}
						</h3>
						<Button variant="ghost" size="sm" onclick={() => (editParamsDialogOpen = true)}>
							<Pencil class="h-3 w-3 mr-1" />
							{m.common_edit()}
						</Button>
					</div>
					<ActionScheduleDisplay schedule={action.schedule} />
				</div>

				{#if children}
					<hr />
					{@render children()}
				{/if}
			</div>

			<!-- Footer -->
			<div class="px-6 py-4 border-t shrink-0">
				<Button variant="outline" class="w-full" href="/actions/{actionId}">
					<ExternalLink class="h-4 w-4 mr-2" />
					{m.action_detail_open_full_page()}
				</Button>
			</div>
		{/if}
	</Sheet.Content>
</Sheet.Root>

<EditNameDialog
	bind:open={editNameDialogOpen}
	bind:value={newName}
	placeholder={m.common_name()}
	onsave={updateName}
	error={nameValidation.errors.name}
	onclearerror={() => nameValidation.clearFieldError('name')}
/>

<EditDescriptionDialog
	bind:open={editDescDialogOpen}
	bind:value={newDescription}
	onsave={updateDescription}
/>

<EditParamsDialog
	bind:open={editParamsDialogOpen}
	{action}
	onsaved={(updated) => {
		action = updated;
		onupdated?.();
	}}
/>
