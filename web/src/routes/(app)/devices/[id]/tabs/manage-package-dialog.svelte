<script lang="ts">
	import { toast } from 'svelte-sonner';
	import { apiClient, fetchAllPages, type ManagedAction } from '$lib/sdk';
	import { ActionType } from '$sdk/powermanage/v1/actions_pb';
	import { AssignmentSourceType, AssignmentTargetType } from '$sdk/powermanage/v1/common_pb';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Badge } from '$lib/components/ui/badge';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Select from '$lib/components/ui/select';
	import { Check, Plus, Package } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import {
		packageFormToProto,
		defaultScheduleForm,
		scheduleFormToProto
	} from '$lib/components/actions';
	import { getLocalizedError } from '$lib/errors';

	interface Props {
		open: boolean;
		packageName: string;
		packageVersion: string;
		deviceId: string;
		/** Set of action IDs already assigned to this device */
		assignedActionIds: Set<string>;
		onassigned: () => void;
	}

	let {
		open = $bindable(),
		packageName,
		packageVersion,
		deviceId,
		assignedActionIds,
		onassigned
	}: Props = $props();

	let desiredState = $state<string>('0');
	let version = $state('');
	let actionName = $state('');
	let matchingActions = $state<ManagedAction[]>([]);
	let selectedActionId = $state('');
	let loading = $state(false);
	let saving = $state(false);

	// Reset state and load matching actions when dialog opens
	$effect(() => {
		if (open) {
			desiredState = '0';
			version = packageVersion;
			actionName = packageName;
			selectedActionId = '';
			saving = false;
			loadMatchingActions();
		}
	});

	async function loadMatchingActions() {
		loading = true;
		try {
			// F023: page through all PACKAGE actions instead of capping at 100.
			const allActions = await fetchAllPages<ManagedAction>(async (size, token) => {
				const r = await apiClient.listActions(size, token, ActionType.PACKAGE);
				return { items: r.actions, nextPageToken: r.nextPageToken };
			});
			const nameLower = packageName.toLowerCase();
			matchingActions = allActions.filter((a) => {
				if (a.params.case !== 'package') return false;
				const p = a.params.value;
				return (
					p.name.toLowerCase() === nameLower ||
					p.aptName.toLowerCase() === nameLower ||
					p.dnfName.toLowerCase() === nameLower ||
					p.pacmanName.toLowerCase() === nameLower ||
					p.zypperName.toLowerCase() === nameLower
				);
			});
		} catch (error) {
			console.error('Failed to load actions:', error);
			matchingActions = [];
		} finally {
			loading = false;
		}
	}

	function isAssigned(actionId: string): boolean {
		return assignedActionIds.has(actionId);
	}

	async function assignExisting() {
		if (!selectedActionId) return;
		saving = true;
		try {
			await apiClient.createAssignment(AssignmentSourceType.ACTION, selectedActionId, AssignmentTargetType.DEVICE, deviceId);
			toast.success(m.software_manage_success());
			open = false;
			onassigned();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			saving = false;
		}
	}

	async function createAndAssign() {
		saving = true;
		try {
			const action = await apiClient.createAction({
				name: actionName.trim(),
				description: '',
				type: ActionType.PACKAGE,
				desiredState: parseInt(desiredState),
				timeoutSeconds: 300,
				schedule: scheduleFormToProto(defaultScheduleForm()),
				params: {
					case: 'package',
					value: packageFormToProto({
						name: packageName,
						version: desiredState === '0' ? version : '',
						allowDowngrade: false,
						pin: false,
						aptName: '',
						dnfName: '',
						pacmanName: '',
						zypperName: ''
					})
				}
			});
			if (action) {
				await apiClient.createAssignment(AssignmentSourceType.ACTION, action.id, AssignmentTargetType.DEVICE, deviceId);
				toast.success(m.software_manage_success());
				open = false;
				onassigned();
			}
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			saving = false;
		}
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content class="max-w-lg">
		<Dialog.Header>
			<Dialog.Title>{m.software_manage_title()}</Dialog.Title>
			<Dialog.Description>
				{m.software_manage_description({ name: packageName })}
			</Dialog.Description>
		</Dialog.Header>

		<div class="space-y-4">
			<!-- Desired State -->
			<div class="space-y-1.5">
				<Label>{m.software_manage_desired_state()}</Label>
				<Select.Root type="single" bind:value={desiredState}>
					<Select.Trigger class="w-full">
						{desiredState === '0' ? m.desired_state_present() : m.desired_state_absent()}
					</Select.Trigger>
					<Select.Content>
						<Select.Item value="0">{m.desired_state_present()}</Select.Item>
						<Select.Item value="1">{m.desired_state_absent()}</Select.Item>
					</Select.Content>
				</Select.Root>
			</div>

			<!-- Version (only when Present) -->
			{#if desiredState === '0'}
				<div class="space-y-1.5">
					<Label>{m.software_manage_version()}</Label>
					<Input bind:value={version} placeholder={packageVersion} />
					<p class="text-xs text-muted-foreground">{m.software_manage_version_hint()}</p>
				</div>
			{/if}

			<!-- Existing matching actions -->
			<div class="space-y-2">
				<Label>{m.software_manage_existing_actions()}</Label>
				{#if loading}
					<p class="text-sm text-muted-foreground">{m.common_loading()}</p>
				{:else if matchingActions.length === 0}
					<p class="text-sm text-muted-foreground">{m.software_manage_no_matching()}</p>
				{:else}
					<div class="space-y-1 max-h-40 overflow-y-auto rounded-md border p-2">
						{#each matchingActions as action}
							{@const assigned = isAssigned(action.id)}
							<button
								type="button"
								class="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm transition-colors {selectedActionId === action.id ? 'bg-primary/10 border border-primary' : 'hover:bg-muted'}"
								onclick={() => { if (!assigned) selectedActionId = action.id; }}
								disabled={assigned}
							>
								<Package class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
								<span class="flex-1 truncate font-medium">{action.name}</span>
								{#if assigned}
									<Badge variant="secondary" class="text-xs shrink-0">
										<Check class="mr-1 h-3 w-3" />
										{m.software_manage_already_assigned()}
									</Badge>
								{/if}
								{#if selectedActionId === action.id}
									<Check class="h-4 w-4 text-primary shrink-0" />
								{/if}
							</button>
						{/each}
					</div>
					{#if selectedActionId && !isAssigned(selectedActionId)}
						<Button
							class="w-full"
							onclick={assignExisting}
							disabled={saving}
						>
							{m.software_manage_assign()}
						</Button>
					{/if}
				{/if}
			</div>

			<!-- Divider -->
			{#if matchingActions.length > 0}
				<div class="relative">
					<div class="absolute inset-0 flex items-center">
						<span class="w-full border-t"></span>
					</div>
					<div class="relative flex justify-center text-xs uppercase">
						<span class="bg-background px-2 text-muted-foreground">{m.common_or()}</span>
					</div>
				</div>
			{/if}

			<!-- Create new -->
			<div class="space-y-2">
				<div class="space-y-1.5">
					<Label>{m.software_manage_action_name()}</Label>
					<Input bind:value={actionName} placeholder={packageName} />
				</div>
				<Button
					class="w-full"
					onclick={createAndAssign}
					disabled={saving || !actionName.trim()}
				>
					<Plus class="mr-2 h-4 w-4" />
					{saving ? m.software_manage_creating() : m.software_manage_create_assign()}
				</Button>
			</div>
		</div>
	</Dialog.Content>
</Dialog.Root>
