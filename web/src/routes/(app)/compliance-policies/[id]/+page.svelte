<script lang="ts">
	import { onMount, onDestroy, untrack } from 'svelte';
	import { base } from '$app/paths';
	import { shell, enterContext, exitContext, type PillAction } from '$lib/shell/shell.svelte';
	import { getLocalizedError } from '$lib/errors';
	import { goto } from '$lib/navigation';
	import { page } from '$app/state';
	import { toast } from 'svelte-sonner';
	import { apiClient, fetchAllPages, type CompliancePolicy, type CompliancePolicyRule, type ManagedAction } from '$lib/sdk';
	import { ActionType } from '$contract/cadestro/v1/actions_pb';
	import {
		AssignmentSourceType,
		AssignmentTargetType,
		ComplianceStatus
	} from '$contract/cadestro/v1/common_pb';
	import { Chip } from '$lib/components/fleet';
	import type { FleetTone } from '$lib/components/fleet';
	import { createFormValidation } from '$lib/forms';
	import { editNameSchema } from '$lib/forms/schemas/common';
	import { Button } from '$lib/components/ui/button';
	import { Label } from '$lib/components/ui/label';
	import { Input } from '$lib/components/ui/input';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Table from '$lib/components/ui/table';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { ActionCreateForm } from '$lib/components/actions';
	import ConfirmDeleteDialog from '$lib/components/confirm-delete-dialog.svelte';
	import EditNameDialog from '$lib/components/edit-name-dialog.svelte';
	import EditDescriptionDialog from '$lib/components/edit-description-dialog.svelte';
	import AssignmentsCard from '$lib/components/assignments-card.svelte';
	import {
		ArrowLeft,
		RefreshCw,
		Trash2,
		ShieldCheck,
		ShieldAlert,
		ShieldQuestion,
		Pencil,
		Plus,
		Clock,
		Search
	} from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import type { Snippet } from 'svelte';

	let policy = $state<CompliancePolicy | null>(null);
	let loading = $state(true);
	let deleteDialogOpen = $state(false);
	let editNameDialogOpen = $state(false);
	let editDescDialogOpen = $state(false);
	let addRuleDialogOpen = $state(false);
	let editGraceDialogOpen = $state(false);
	let newName = $state('');
	let newDescription = $state('');
	const nameValidation = createFormValidation(editNameSchema);

	// Add rule state
	let complianceActions = $state<ManagedAction[]>([]);
	let selectedActionId = $state('');
	let gracePeriodHours = $state(0);
	let addingRule = $state(false);
	let addRuleStep = $state<'select' | 'create-action'>('select');
	let addRuleSearchQuery = $state('');

	let filteredComplianceActions = $derived(
		complianceActions.filter((a) => {
			if (!addRuleSearchQuery) return true;
			const q = addRuleSearchQuery.toLowerCase();
			return a.name.toLowerCase().includes(q) || a.description.toLowerCase().includes(q);
		})
	);

	// Edit grace period state
	let editRuleActionId = $state('');
	let editGracePeriodHours = $state(0);
	let updatingRule = $state(false);

	const policyId = $derived(page.params.id ?? '');

	// The policy's OWN action. There is no editable draft on this page — every
	// field commits through its own dialog — so the context it holds is
	// permanently clean: it exists solely to be the policy's action bar, the same
	// role the action detail page's clean context plays for action types with no
	// params form. A clean context offers no Stash/Cancel and a disabled commit,
	// so Delete reads as the one thing on offer, in crit with a trash glyph.
	const entityActions: PillAction[] = [
		{
			id: 'delete',
			label: m.compliance_policy_detail_delete_policy(),
			tone: 'danger',
			onRun: () => (deleteDialogOpen = true)
		}
	];
	// Deliberately NOT `compliance-policy:<id>`: the detail SHEET on the list
	// route already owns that id for a dirty grace-period draft, and the pill has
	// one slot. Sharing the id would let this page's clean context replace that
	// draft — or the sheet's teardown drop this page's action bar — on the
	// navigation between them.
	const contextId = $derived(`compliance-policy-actions:${policyId}`);
	$effect(() => {
		const p = policy;
		if (!p) return;
		untrack(() =>
			enterContext({
				id: contextId,
				title: p.name,
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

	// Per-device results. There is no policy→devices status RPC: the retained
	// surface has GetDeviceCompliancePolicyStatus(device_id) only, so the page
	// resolves the policy's directly-assigned devices through ListAssignments and
	// reads each device's evaluation, capped so a large assignment cannot turn
	// one page load into hundreds of requests. Devices reached through a group
	// assignment are counted but not expanded — that would need group-membership
	// reads this section does not make.
	const DEVICE_RESULT_LIMIT = 25;

	type DeviceResult = {
		deviceId: string;
		hostname: string;
		status: ComplianceStatus;
		failing: string[];
	};

	let deviceResults = $state<DeviceResult[]>([]);
	let deviceResultsLoading = $state(false);
	let deviceTargetTotal = $state(0);
	let groupTargetCount = $state(0);

	onMount(() => {
		if (policyId) {
			loadData();
		}
	});

	async function loadData() {
		loading = true;
		try {
			const p = await apiClient.getCompliancePolicy(policyId);
			policy = p ?? null;
			if (policy) {
				newName = policy.name;
				newDescription = policy.description;
			}
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			loading = false;
		}
		await loadDeviceResults();
	}

	async function loadDeviceResults() {
		if (!policyId) return;
		deviceResultsLoading = true;
		try {
			const response = await apiClient.listAssignments(
				50,
				'',
				AssignmentSourceType.COMPLIANCE_POLICY,
				policyId
			);
			const deviceTargets = response.assignments.filter(
				(a) => a.targetType === AssignmentTargetType.DEVICE
			);
			deviceTargetTotal = deviceTargets.length;
			groupTargetCount = response.assignments.length - deviceTargets.length;

			const slice = deviceTargets.slice(0, DEVICE_RESULT_LIMIT);
			const settled = await Promise.allSettled(
				slice.map((a) => apiClient.getDeviceCompliancePolicyStatus(a.targetId))
			);
			deviceResults = settled.map((result, i) => {
				const target = slice[i];
				const hostname = target.targetName || target.targetId.slice(0, 8) + '...';
				if (result.status === 'rejected') {
					// A device we could not read is reported as unknown, never as passing.
					console.error('Failed to read device compliance status', result.reason);
					return {
						deviceId: target.targetId,
						hostname,
						status: ComplianceStatus.UNKNOWN,
						failing: []
					};
				}
				const evaluation = result.value.policies.find((p) => p.policyId === policyId);
				return {
					deviceId: target.targetId,
					hostname,
					status: evaluation?.status ?? ComplianceStatus.UNKNOWN,
					failing: (evaluation?.rules ?? [])
						.filter((r) => r.status !== ComplianceStatus.COMPLIANT)
						.map((r) => r.actionName)
				};
			});
		} catch (error) {
			console.error('Failed to load device compliance results', error);
			toast.error(getLocalizedError(error));
			deviceResults = [];
		} finally {
			deviceResultsLoading = false;
		}
	}

	function statusIcon(status: ComplianceStatus) {
		switch (status) {
			case ComplianceStatus.COMPLIANT:
				return ShieldCheck;
			case ComplianceStatus.NON_COMPLIANT:
				return ShieldAlert;
			case ComplianceStatus.IN_GRACE_PERIOD:
				return Clock;
			default:
				return ShieldQuestion;
		}
	}

	/** Static per-tone ink; Tailwind only emits classes it can see literally. */
	const TONE_INK: Record<FleetTone, string> = {
		ok: 'text-ok',
		warn: 'text-warn',
		crit: 'text-crit',
		info: 'text-info',
		idle: 'text-idle'
	};

	function statusTone(status: ComplianceStatus): FleetTone {
		switch (status) {
			case ComplianceStatus.COMPLIANT:
				return 'ok';
			case ComplianceStatus.NON_COMPLIANT:
				return 'crit';
			case ComplianceStatus.IN_GRACE_PERIOD:
				return 'warn';
			default:
				return 'idle';
		}
	}

	function statusLabel(status: ComplianceStatus): string {
		switch (status) {
			case ComplianceStatus.COMPLIANT:
				return m.compliance_status_compliant();
			case ComplianceStatus.NON_COMPLIANT:
				return m.compliance_status_non_compliant();
			case ComplianceStatus.IN_GRACE_PERIOD:
				return m.compliance_status_in_grace_period();
			default:
				return m.compliance_status_unknown();
		}
	}

	async function deletePolicy() {
		if (!policyId) return;
		try {
			await apiClient.deleteCompliancePolicy(policyId);
			toast.success(m.compliance_policies_deleted());
			goto('/compliance-policies');
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function updateName() {
		if (!policyId) return;
		await nameValidation.handleSubmit({ name: newName }, async () => {
			try {
				policy = (await apiClient.renameCompliancePolicy(policyId, newName.trim())) ?? null;
				editNameDialogOpen = false;
				toast.success(m.compliance_policy_detail_name_updated());
			} catch (error) {
				toast.error(getLocalizedError(error));
				console.error(error);
			}
		});
	}

	async function updateDescription() {
		if (!policyId) return;
		try {
			policy = (await apiClient.updateCompliancePolicyDescription(policyId, newDescription.trim())) ?? null;
			editDescDialogOpen = false;
			toast.success(m.compliance_policy_detail_desc_updated());
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function openAddRuleDialog() {
		try {
			// F023: page through all SHELL actions instead of capping at 200.
			const allActions = await fetchAllPages<ManagedAction>(async (size, token) => {
				const r = await apiClient.listActions(size, token, ActionType.SHELL);
				return { items: r.actions, nextPageToken: r.nextPageToken };
			});
			// Filter to only compliance check actions and exclude already-added ones
			const existingActionIds = policy?.rules.map((r) => r.actionId) ?? [];
			complianceActions = allActions.filter((a) => {
				if (existingActionIds.includes(a.id)) return false;
				if (a.type !== ActionType.SHELL) return false;
				if (a.params.case === 'shell') {
					return a.params.value.isCompliance;
				}
				return false;
			});
		} catch (error) {
			console.error('Failed to load compliance actions', error);
			complianceActions = [];
		}
		selectedActionId = '';
		gracePeriodHours = 0;
		addRuleStep = 'select';
		addRuleSearchQuery = '';
		addRuleDialogOpen = true;
	}

	async function addRule() {
		if (!policyId || !selectedActionId) return;
		addingRule = true;
		try {
			policy = (await apiClient.addCompliancePolicyRule(policyId, selectedActionId, gracePeriodHours)) ?? null;
			addRuleDialogOpen = false;
			toast.success(m.compliance_policy_detail_rule_added());
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			addingRule = false;
		}
	}

	function handleActionCreatedForRule(action: ManagedAction) {
		complianceActions = [action, ...complianceActions];
		selectedActionId = action.id;
		addRuleStep = 'select';
	}

	async function removeRule(actionId: string) {
		if (!policyId) return;
		try {
			policy = (await apiClient.removeCompliancePolicyRule(policyId, actionId)) ?? null;
			toast.success(m.compliance_policy_detail_rule_removed());
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	function openEditGracePeriod(rule: CompliancePolicyRule) {
		editRuleActionId = rule.actionId;
		editGracePeriodHours = rule.gracePeriodHours;
		editGraceDialogOpen = true;
	}

	async function updateGracePeriod() {
		if (!policyId || !editRuleActionId) return;
		updatingRule = true;
		try {
			policy = (await apiClient.updateCompliancePolicyRule(policyId, editRuleActionId, editGracePeriodHours)) ?? null;
			editGraceDialogOpen = false;
			toast.success(m.compliance_policy_detail_rule_updated());
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			updatingRule = false;
		}
	}

	function formatGracePeriod(hours: number): string {
		if (hours === 0) return m.compliance_policy_detail_no_grace_period();
		return m.compliance_policy_detail_grace_period_hours({ count: hours });
	}
</script>

{#snippet block(title: string, explanation: string, control: Snippet)}
	<section
		class="grid gap-4 border-b border-hair pb-6 last:border-b-0 md:grid-cols-[minmax(0,1fr)_minmax(0,1.6fr)]"
	>
		<div>
			<h3 class="text-sm font-semibold">{title}</h3>
			<p class="mt-1 max-w-prose text-xs leading-relaxed text-muted-foreground">{explanation}</p>
		</div>
		<div>{@render control()}</div>
	</section>
{/snippet}

<div class="flex-1 overflow-auto p-4 md:p-6">
<div class="space-y-6">
	<div class="flex items-center gap-4">
		<Button variant="ghost" size="icon" onclick={() => history.back()}>
			<ArrowLeft class="h-4 w-4" />
		</Button>
		<div class="flex-1">
			<h1 class="text-2xl font-bold">{policy?.name ?? m.common_loading()}</h1>
			<p class="font-mono text-xs text-faint">{policyId}</p>
		</div>
		<Button variant="outline" onclick={loadData} disabled={loading}>
			<span class="mr-2 h-4 w-4" class:animate-spin={loading}>
				<RefreshCw class="h-4 w-4" />
			</span>
			{m.common_refresh()}
		</Button>
	</div>

	{#if loading && !policy}
		<div
			class="flex items-center justify-center rounded-xl border border-hair bg-surface py-12 shadow-plate"
		>
			<RefreshCw class="h-6 w-6 animate-spin text-muted-foreground" />
		</div>
	{:else if policy}
		<!-- Left column explains what the rule checks; right column is the control
		     that changes it. Same grammar as the sheet. -->
		<section
			data-tour="compliance-sections"
			class="space-y-6 rounded-xl border border-hair bg-surface p-4 shadow-plate"
		>
			{#snippet detailsControl()}
				{#if policy}
				<div class="space-y-4">
					<div>
						<div class="flex items-center justify-between">
							<Label class="text-muted-foreground">{m.common_name()}</Label>
							<Button
								variant="ghost"
								size="sm"
								aria-label={m.common_name()}
								onclick={() => {
									newName = policy?.name ?? '';
									nameValidation.clearErrors();
									editNameDialogOpen = true;
								}}
							>
								<Pencil class="h-3 w-3" />
							</Button>
						</div>
						<p class="font-medium">{policy.name}</p>
					</div>
					<div>
						<div class="flex items-center justify-between">
							<Label class="text-muted-foreground">{m.common_description()}</Label>
							<Button
								variant="ghost"
								size="sm"
								aria-label={m.common_description()}
								onclick={() => {
									newDescription = policy?.description ?? '';
									editDescDialogOpen = true;
								}}
							>
								<Pencil class="h-3 w-3" />
							</Button>
						</div>
						<p class="text-sm">{policy.description || m.common_no_description()}</p>
					</div>
					<div>
						<Label class="text-muted-foreground">{m.compliance_policy_detail_rules()}</Label>
						<p class="font-medium">
							{m.compliance_policies_rule_count({ count: policy.ruleCount })}
						</p>
					</div>
				</div>
				{/if}
			{/snippet}
			{@render block(
				m.compliance_policy_section_details_title(),
				m.compliance_policy_section_details_help(),
				detailsControl
			)}

			{#snippet rulesControl()}
				{#if policy}
				<div class="space-y-2">
					<div class="flex items-center justify-end">
						<Button size="sm" onclick={openAddRuleDialog}>
							<Plus class="h-4 w-4 mr-1" />
							{m.compliance_policy_detail_add_rule()}
						</Button>
					</div>
					{#if policy.rules.length === 0}
						<p class="text-muted-foreground text-sm text-center py-4">
							{m.compliance_policy_detail_no_rules()}
						</p>
					{:else}
						<ul class="divide-y divide-hair rounded-xl border border-hair bg-sunken">
							{#each policy.rules as rule (rule.actionId)}
								<li class="flex items-center justify-between gap-2 px-3 py-2.5">
									<div class="flex items-center gap-2 min-w-0 flex-1">
										<ShieldCheck class="h-4 w-4 text-muted-foreground shrink-0" />
										<span class="font-medium truncate text-sm">{rule.actionName}</span>
									</div>
									<div class="flex items-center gap-2 shrink-0">
										<button
											class="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground cursor-pointer"
											onclick={() => openEditGracePeriod(rule)}
										>
											<Clock class="h-3 w-3" />
											{formatGracePeriod(rule.gracePeriodHours)}
										</button>
										<Button
											variant="ghost"
											size="icon"
											class="h-7 w-7"
											aria-label={m.common_delete()}
											onclick={() => removeRule(rule.actionId)}
										>
											<Trash2 class="h-3.5 w-3.5 text-destructive" />
										</Button>
									</div>
								</li>
							{/each}
						</ul>
					{/if}
				</div>
				{/if}
			{/snippet}
			{@render block(
				m.compliance_policy_section_rules_title(),
				m.compliance_policy_section_rules_help(),
				rulesControl
			)}

			{#snippet devicesControl()}
				<div class="space-y-2" data-testid="compliance-device-results">
					{#if deviceResultsLoading}
						<p class="py-4 text-center text-sm text-muted-foreground">
							{m.compliance_policy_devices_loading()}
						</p>
					{:else if deviceResults.length === 0}
						<p class="py-4 text-center text-sm text-muted-foreground">
							{m.compliance_policy_devices_empty()}
						</p>
					{:else}
						<ul class="divide-y divide-hair rounded-xl border border-hair bg-sunken">
							{#each deviceResults as result (result.deviceId)}
								{@const StatusIcon = statusIcon(result.status)}
								<li class="flex flex-wrap items-center gap-2 px-3 py-2.5">
									<StatusIcon class="h-4 w-4 shrink-0 {TONE_INK[statusTone(result.status)]}" />
									<a
										href="{base}/devices/{result.deviceId}"
										class="min-w-0 flex-1 truncate font-mono text-sm hover:underline"
									>
										{result.hostname}
									</a>
									{#if result.failing.length > 0}
										<span class="truncate text-xs text-muted-foreground">
											{result.failing.join(', ')}
										</span>
									{/if}
									<Chip tone={statusTone(result.status)} label={statusLabel(result.status)} />
								</li>
							{/each}
						</ul>
					{/if}
					{#if deviceTargetTotal > deviceResults.length && !deviceResultsLoading}
						<p class="text-xs text-muted-foreground">
							{m.compliance_policy_devices_truncated({
								shown: deviceResults.length,
								total: deviceTargetTotal
							})}
						</p>
					{/if}
					{#if groupTargetCount > 0}
						<p class="text-xs text-muted-foreground">
							{m.compliance_policy_devices_groups_note({ count: groupTargetCount })}
						</p>
					{/if}
				</div>
			{/snippet}
			{@render block(
				m.compliance_policy_section_devices_title(),
				m.compliance_policy_section_devices_help(),
				devicesControl
			)}
		</section>

		<AssignmentsCard
			sourceType={AssignmentSourceType.COMPLIANCE_POLICY}
			sourceId={policyId}
			title={m.assignments_title()}
			subtitle={m.assignments_subtitle_compliance_policy()}
			assignTitle={m.assignments_assign_compliance_policy()}
			assignDescription={m.assignments_assign_description_compliance_policy()}
		/>

	{/if}
</div>

<ConfirmDeleteDialog
	bind:open={deleteDialogOpen}
	title={m.compliance_policies_delete_dialog_title()}
	description={m.compliance_policies_delete_dialog_description({ name: policy?.name ?? '' })}
	onconfirm={deletePolicy}
/>

<EditNameDialog
	bind:open={editNameDialogOpen}
	bind:value={newName}
	placeholder={m.common_name()}
	onsave={updateName}
	error={nameValidation.errors.name}
	onclearerror={() => nameValidation.clearFieldError('name')}
/>

<EditDescriptionDialog bind:open={editDescDialogOpen} bind:value={newDescription} onsave={updateDescription} />

<!-- Add Rule Dialog -->
<Dialog.Root bind:open={addRuleDialogOpen}>
	<Dialog.Content class={addRuleStep === 'create-action' ? 'sm:max-w-4xl max-h-[90vh] overflow-hidden flex flex-col' : 'sm:max-w-lg'}>
		{#if addRuleStep === 'select'}
			<Dialog.Header>
				<Dialog.Title>{m.compliance_policy_detail_add_rule_title()}</Dialog.Title>
				<Dialog.Description>
					{m.compliance_policy_detail_add_rule_description()}
				</Dialog.Description>
			</Dialog.Header>

			<div class="flex items-center justify-between gap-2 py-2">
				<div class="relative flex-1">
					<Search class="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
					<Input
						placeholder={m.compliance_policy_detail_select_action()}
						class="pl-9"
						bind:value={addRuleSearchQuery}
					/>
				</div>
				<Button variant="outline" size="sm" onclick={() => (addRuleStep = 'create-action')}>
					<Plus class="h-4 w-4 mr-1" />
					{m.compliance_policy_add_rule_create_new()}
				</Button>
			</div>

			<div class="max-h-[40vh] overflow-y-auto">
				{#if filteredComplianceActions.length === 0}
					<div class="py-8 text-center">
						<p class="text-muted-foreground text-sm">{m.compliance_policy_detail_no_compliance_actions()}</p>
						<Button variant="outline" class="mt-4" onclick={() => (addRuleStep = 'create-action')}>
							<Plus class="h-4 w-4 mr-1" />
							{m.compliance_policy_add_rule_create_new()}
						</Button>
					</div>
				{:else}
					<Table.Root>
						<Table.Header>
							<Table.Row>
								<Table.Head class="w-10"></Table.Head>
								<Table.Head>{m.common_name()}</Table.Head>
							</Table.Row>
						</Table.Header>
						<Table.Body>
							{#each filteredComplianceActions as action}
								<Table.Row
									class="cursor-pointer"
									onclick={() => (selectedActionId = selectedActionId === action.id ? '' : action.id)}
								>
									<Table.Cell>
										<Checkbox
											checked={selectedActionId === action.id}
											onCheckedChange={() => (selectedActionId = selectedActionId === action.id ? '' : action.id)}
											onclick={(e: MouseEvent) => e.stopPropagation()}
										/>
									</Table.Cell>
									<Table.Cell>
										<div class="flex items-center gap-2">
											<ShieldCheck class="h-4 w-4 text-muted-foreground shrink-0" />
											<div>
												<span class="font-medium">{action.name}</span>
												{#if action.description}
													<p class="text-xs text-muted-foreground line-clamp-1">{action.description}</p>
												{/if}
											</div>
										</div>
									</Table.Cell>
								</Table.Row>
							{/each}
						</Table.Body>
					</Table.Root>
				{/if}
			</div>

			{#if selectedActionId}
				<div class="border-t pt-3 mt-2 space-y-2">
					<Label>{m.compliance_policy_detail_grace_period_label()}</Label>
					<Input type="number" min="0" bind:value={gracePeriodHours} />
					<p class="text-xs text-muted-foreground">
						{m.compliance_policy_detail_edit_grace_period_description()}
					</p>
				</div>
			{/if}

			<Dialog.Footer>
				<Button variant="outline" onclick={() => (addRuleDialogOpen = false)}>{m.common_cancel()}</Button>
				<Button onclick={addRule} disabled={addingRule || !selectedActionId}>
					{addingRule ? m.common_creating() : m.common_add()}
				</Button>
			</Dialog.Footer>
		{:else if addRuleStep === 'create-action'}
			<div class="flex-1 overflow-y-auto p-1 m-2">
				<ActionCreateForm
					compact
					initialType="COMPLIANCE_CHECK"
					onCancel={() => (addRuleStep = 'select')}
					onCreated={handleActionCreatedForRule}
				/>
			</div>
		{/if}
	</Dialog.Content>
</Dialog.Root>

<!-- Edit Grace Period Dialog -->
<Dialog.Root bind:open={editGraceDialogOpen}>
	<Dialog.Content>
		<Dialog.Header>
			<Dialog.Title>{m.compliance_policy_detail_edit_grace_period()}</Dialog.Title>
			<Dialog.Description>
				{m.compliance_policy_detail_edit_grace_period_description()}
			</Dialog.Description>
		</Dialog.Header>
		<div class="space-y-4 py-4">
			<div class="space-y-2">
				<Label>{m.compliance_policy_detail_grace_period_label()}</Label>
				<Input type="number" min="0" bind:value={editGracePeriodHours} />
			</div>
		</div>
		<Dialog.Footer>
			<Button variant="outline" onclick={() => (editGraceDialogOpen = false)}>{m.common_cancel()}</Button>
			<Button onclick={updateGracePeriod} disabled={updatingRule}>
				{updatingRule ? m.common_saving() : m.common_save()}
			</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

</div>
