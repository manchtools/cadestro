<script lang="ts" module>
	import { pushState } from '$lib/navigation';

	/**
	 * Open the compliance policy detail sheet via shallow routing.
	 * Call from any page that includes <CompliancePolicyDetailSheet />.
	 */
	export function openCompliancePolicySheet(policyId: string) {
		pushState(`/compliance-policies/${policyId}`, { compliancePolicySheet: policyId });
	}
</script>

<script lang="ts">
	// Compliance reads as left-explanation / right-control section blocks: the
	// left column says what the rule checks, the right column is the control that
	// changes it. Grace periods are edited inline and committed together through
	// the shell's context pill — a committable edit state, not a one-shot dialog.
	// Add / remove stay one-shot, because they are.
	import { page } from '$app/state';
	import { toast } from 'svelte-sonner';
	import { getLocalizedError } from '$lib/errors';
	import {
		apiClient,
		fetchAllPages,
		type CompliancePolicy,
		type ManagedAction
	} from '$lib/sdk';
	import { ActionType } from '$contract/cadestro/v1/actions_pb';
	import { Button } from '$lib/components/ui/button';
	import { Label } from '$lib/components/ui/label';
	import { Input } from '$lib/components/ui/input';
	import { Chip } from '$lib/components/fleet';
	import * as Sheet from '$lib/components/ui/sheet';
	import * as Table from '$lib/components/ui/table';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { ActionCreateForm } from '$lib/components/actions';
	import EditNameDialog from '$lib/components/edit-name-dialog.svelte';
	import EditDescriptionDialog from '$lib/components/edit-description-dialog.svelte';
	import { createFormValidation } from '$lib/forms';
	import { editNameSchema } from '$lib/forms/schemas/common';
	import { enterContext, exitContext, shell, updateContext } from '$lib/shell/shell.svelte';
	import {
		RefreshCw,
		Pencil,
		ShieldCheck,
		ExternalLink,
		Plus,
		Trash2,
		Search
	} from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { onDestroy, untrack, type Snippet } from 'svelte';

	interface Props {
		onupdated?: () => void;
	}

	let { onupdated }: Props = $props();

	/** UpdateCompliancePolicyRuleRequest validates grace_period_hours 0..8760. */
	const GRACE_MAX_HOURS = 8760;

	// Derive state from shallow routing
	let policyId = $derived(page.state.compliancePolicySheet);
	let sheetOpen = $derived(!!policyId);

	let policy = $state<CompliancePolicy | null>(null);
	let loading = $state(false);
	let editNameDialogOpen = $state(false);
	let editDescDialogOpen = $state(false);
	let newName = $state('');
	let newDescription = $state('');

	const nameValidation = createFormValidation(editNameSchema);

	// Add rule state
	let addRuleDialogOpen = $state(false);
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

	// Inline grace-period edit: action id → the operator's draft value.
	let graceDraft = $state<Record<string, number>>({});
	let savingGrace = $state(false);

	const contextId = $derived(`compliance-policy:${policyId ?? ''}`);
	const dirtyRules = $derived(
		(policy?.rules ?? []).filter(
			(rule) =>
				graceDraft[rule.actionId?.value ?? ''] !== undefined &&
				graceDraft[rule.actionId?.value ?? ''] !== rule.gracePeriodHours
		)
	);
	const graceValid = $derived(
		Object.values(graceDraft).every(
			(hours) => Number.isInteger(hours) && hours >= 0 && hours <= GRACE_MAX_HOURS
		)
	);

	function seedGraceDraft(next: CompliancePolicy | null) {
		const seeded: Record<string, number> = {};
		for (const rule of next?.rules ?? []) seeded[rule.actionId?.value ?? ''] = rule.gracePeriodHours;
		graceDraft = seeded;
	}

	$effect(() => {
		if (sheetOpen && policyId) {
			loadPolicy();
		}
		if (!sheetOpen) {
			policy = null;
			graceDraft = {};
		}
	});

	// The pill carries the commit only while there is something to commit, and
	// this sheet only ever owns its own context — never someone else's.
	//
	// The shell reads and writes are untracked on purpose: enterContext /
	// updateContext mutate the very state a tracked read would subscribe to, and
	// the callbacks below are fresh closures on every run, so a tracked version
	// re-triggers itself forever.
	let ownedContextId = $state('');
	$effect(() => {
		const active = sheetOpen && dirtyRules.length > 0;
		const count = dirtyRules.length;
		const valid = graceValid;
		const title = policy?.name ?? m.compliance_policy_detail_rules_label();
		const subtext = valid
			? m.compliance_policy_grace_dirty({ count })
			: m.compliance_policy_grace_invalid({ max: GRACE_MAX_HOURS });
		const id = contextId;

		untrack(() => {
			if (active) {
				const patch = {
					title,
					valid,
					subtext,
					subtextTone: valid ? ('neutral' as const) : ('warn' as const)
				};
				if (shell.pill.context?.id === id) {
					updateContext(patch);
				} else {
					enterContext({
						id,
						dirty: true,
						commitLabel: m.common_save(),
						onCommit: saveGracePeriods,
						onCancel: () => seedGraceDraft(policy),
						...patch
					});
				}
				ownedContextId = id;
			} else if (shell.pill.context?.id === id) {
				exitContext();
				ownedContextId = '';
			}
		});
	});

	// Navigating away while an edit is pending must not strand the pill.
	onDestroy(() => {
		if (ownedContextId && shell.pill.context?.id === ownedContextId) exitContext();
	});

	async function loadPolicy() {
		if (!policyId) return;
		loading = true;
		try {
			policy = (await apiClient.getCompliancePolicy(policyId)) ?? null;
			if (policy) {
				newName = policy.name;
				newDescription = policy.description;
			}
			seedGraceDraft(policy);
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			loading = false;
		}
	}

	async function updateName() {
		if (!policyId) return;
		const valid = nameValidation.validate({ name: newName.trim() });
		if (!valid) return;
		try {
			policy = (await apiClient.renameCompliancePolicy(policyId, newName.trim())) ?? null;
			editNameDialogOpen = false;
			toast.success(m.compliance_policy_detail_name_updated());
			onupdated?.();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function updateDescription() {
		if (!policyId) return;
		try {
			policy =
				(await apiClient.updateCompliancePolicyDescription(policyId, newDescription.trim())) ?? null;
			editDescDialogOpen = false;
			toast.success(m.compliance_policy_detail_desc_updated());
			onupdated?.();
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
			const existingActionIds = policy?.rules.map((r) => r.actionId?.value ?? '') ?? [];
			complianceActions = allActions.filter((a) => {
				if (existingActionIds.includes((a.id?.value ?? ''))) return false;
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
			policy =
				(await apiClient.addCompliancePolicyRule(policyId, selectedActionId, gracePeriodHours)) ??
				null;
			seedGraceDraft(policy);
			addRuleDialogOpen = false;
			toast.success(m.compliance_policy_detail_rule_added());
			onupdated?.();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			addingRule = false;
		}
	}

	function handleActionCreatedForRule(action: ManagedAction) {
		complianceActions = [action, ...complianceActions];
		selectedActionId = (action.id?.value ?? '');
		addRuleStep = 'select';
	}

	async function removeRule(actionId: string) {
		if (!policyId) return;
		try {
			policy = (await apiClient.removeCompliancePolicyRule(policyId, actionId)) ?? null;
			seedGraceDraft(policy);
			toast.success(m.compliance_policy_detail_rule_removed());
			onupdated?.();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	/** Commit every changed grace period; the pill is the only trigger. */
	async function saveGracePeriods() {
		if (!policyId || savingGrace || !graceValid) return;
		const pending = dirtyRules.map((rule) => ({
			actionId: rule.actionId?.value ?? '',
			hours: graceDraft[rule.actionId?.value ?? '']
		}));
		if (pending.length === 0) return;
		savingGrace = true;
		try {
			let updated: CompliancePolicy | null = policy;
			for (const change of pending) {
				updated =
					(await apiClient.updateCompliancePolicyRule(policyId, change.actionId, change.hours)) ??
					updated;
			}
			policy = updated;
			seedGraceDraft(updated);
			toast.success(m.compliance_policy_grace_saved());
			onupdated?.();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			savingGrace = false;
		}
	}

	function graceChip(hours: number): { tone: 'ok' | 'warn'; label: string } {
		return hours === 0
			? { tone: 'ok', label: m.compliance_policy_detail_no_grace_period() }
			: { tone: 'warn', label: m.compliance_policy_detail_grace_period_hours({ count: hours }) };
	}

	function handleOpenChange(isOpen: boolean) {
		if (!isOpen) {
			history.back();
		}
	}
</script>

{#snippet block(title: string, explanation: string, control: Snippet)}
	<section class="grid gap-4 border-b border-hair pb-6 last:border-b-0 md:grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)]">
		<div>
			<h3 class="text-sm font-semibold">{title}</h3>
			<p class="mt-1 text-xs leading-relaxed text-muted-foreground">{explanation}</p>
		</div>
		<div>{@render control()}</div>
	</section>
{/snippet}

<Sheet.Root open={sheetOpen} onOpenChange={handleOpenChange}>
	<Sheet.Content class="sm:max-w-2xl w-full !p-0 !gap-0 flex flex-col">
		<Sheet.Header class="px-6 pt-6 pb-4 border-b shrink-0">
			{#if policy}
				<div class="flex items-center gap-3">
					<div class="rounded-md bg-accent-soft p-2 shrink-0">
						<ShieldCheck class="h-5 w-5 text-accent-ink" />
					</div>
					<div class="min-w-0 flex-1">
						<Sheet.Title class="truncate">{policy.name}</Sheet.Title>
						<Sheet.Description class="truncate font-mono text-xs">{policyId}</Sheet.Description>
					</div>
				</div>
			{:else}
				<Sheet.Title>{m.common_loading()}</Sheet.Title>
			{/if}
		</Sheet.Header>

		{#if loading && !policy}
			<div class="flex items-center justify-center py-12">
				<RefreshCw class="h-6 w-6 animate-spin text-muted-foreground" />
			</div>
		{:else if policy}
			<div class="flex-1 space-y-6 overflow-y-auto px-6 py-6" data-tour="compliance-sections">
				{#snippet detailsControl()}
					<!-- A snippet body is its own scope, so the enclosing
					     `{:else if policy}` narrowing does not reach in here. -->
					{#if policy}
					<div class="space-y-3">
						<div>
							<div class="flex items-center justify-between">
								<Label class="text-muted-foreground">{m.common_name()}</Label>
								<Button
									variant="ghost"
									size="sm"
									class="h-6 w-6 p-0"
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
							<p class="text-sm font-medium">{policy.name}</p>
						</div>
						<div>
							<div class="flex items-center justify-between">
								<Label class="text-muted-foreground">{m.common_description()}</Label>
								<Button
									variant="ghost"
									size="sm"
									class="h-6 w-6 p-0"
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
							<p class="mt-1 text-sm font-medium">
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
							<Button variant="ghost" size="sm" onclick={openAddRuleDialog}>
								<Plus class="h-3 w-3 mr-1" />
								{m.compliance_policy_detail_add_rule()}
							</Button>
						</div>
						{#if policy.rules.length === 0}
							<p class="py-4 text-center text-sm text-muted-foreground">
								{m.compliance_policy_detail_no_rules()}
							</p>
						{:else}
							<ul class="divide-y divide-hair rounded-xl border border-hair bg-surface">
								{#each policy.rules as rule (rule.actionId?.value ?? '')}
									{@const chip = graceChip(rule.gracePeriodHours)}
									<li class="flex flex-wrap items-center gap-2 px-3 py-2.5">
										<ShieldCheck class="h-4 w-4 shrink-0 text-muted-foreground" />
										<span class="min-w-0 flex-1 truncate text-sm font-medium">
											{rule.actionName}
										</span>
										<Chip tone={chip.tone} label={chip.label} />
										<label class="flex items-center gap-1.5 text-xs text-muted-foreground">
											<span class="sr-only"
												>{m.compliance_policy_detail_grace_period_label()}</span
											>
											<Input
												type="number"
												min="0"
												max={GRACE_MAX_HOURS}
												class="h-7 w-20 font-mono text-xs"
												aria-label="{m.compliance_policy_detail_grace_period_label()} — {rule.actionName}"
								value={graceDraft[rule.actionId?.value ?? ''] ?? rule.gracePeriodHours}
												oninput={(e) =>
													(graceDraft = {
														...graceDraft,
									[rule.actionId?.value ?? '']: e.currentTarget.valueAsNumber
													})}
											/>
										</label>
										<Button
											variant="ghost"
											size="icon"
											class="h-7 w-7"
											aria-label={m.common_delete()}
							onclick={() => removeRule(rule.actionId?.value ?? '')}
										>
											<Trash2 class="h-3.5 w-3.5 text-destructive" />
										</Button>
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
			</div>
		{/if}

		{#if policy}
			<!-- Footer -->
			<div class="px-6 py-4 border-t shrink-0">
				<Button variant="outline" class="w-full" href="/compliance-policies/{policyId}">
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

<!-- Add Rule Dialog -->
<Dialog.Root bind:open={addRuleDialogOpen}>
	<Dialog.Content
		class={addRuleStep === 'create-action'
			? 'sm:max-w-4xl max-h-[90vh] overflow-hidden flex flex-col'
			: 'sm:max-w-lg'}
	>
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
						<p class="text-muted-foreground text-sm">
							{m.compliance_policy_detail_no_compliance_actions()}
						</p>
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
							{#each filteredComplianceActions as action (action.id)}
								<Table.Row
									class="cursor-pointer"
									onclick={() =>
										(selectedActionId = selectedActionId === (action.id?.value ?? '') ? '' : (action.id?.value ?? ''))}
								>
									<Table.Cell>
										<Checkbox
											checked={selectedActionId === (action.id?.value ?? '')}
											onCheckedChange={() =>
												(selectedActionId = selectedActionId === (action.id?.value ?? '') ? '' : (action.id?.value ?? ''))}
											onclick={(e: MouseEvent) => e.stopPropagation()}
										/>
									</Table.Cell>
									<Table.Cell>
										<div class="flex items-center gap-2">
											<ShieldCheck class="h-4 w-4 text-muted-foreground shrink-0" />
											<div>
												<span class="font-medium">{action.name}</span>
												{#if action.description}
													<p class="text-xs text-muted-foreground line-clamp-1">
														{action.description}
													</p>
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
					<Input type="number" min="0" max={GRACE_MAX_HOURS} bind:value={gracePeriodHours} />
					<p class="text-xs text-muted-foreground">
						{m.compliance_policy_detail_edit_grace_period_description()}
					</p>
				</div>
			{/if}

			<Dialog.Footer>
				<Button variant="outline" onclick={() => (addRuleDialogOpen = false)}>
					{m.common_cancel()}
				</Button>
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
