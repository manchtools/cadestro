<script lang="ts">
	// /compliance-policies/new — policy creation as a pill-committed surface.
	//
	// The modal it replaces was a three-step wizard behind a dialog footer: name
	// and description, then a compliance-check picker, then an inline check create.
	// None of it could be parked. On the route the wizard collapses into two plates
	// on one page — there is no "Next", because the commit is the pill's.
	//
	// The RPC sequence is unchanged: CreateCompliancePolicy, then
	// AddCompliancePolicyRule once per selected check.
	import { toast } from 'svelte-sonner';
	import { goto } from '$lib/navigation';
	import { apiClient, fetchAllPages, useDraft, type ManagedAction } from '$lib/sdk';
	import { ActionType } from '$contract/cadestro/v1/actions_pb';
	import { nameDescriptionSchema } from '$lib/forms/schemas/common';
	import { bindBuilderContext } from '$lib/components/actions/pipeline/builder-pill.svelte';
	import CreatePlate from '$lib/components/create/create-plate.svelte';
	import IdentityRow from '$lib/components/create/identity-row.svelte';
	import { ActionCreateForm } from '$lib/components/actions';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import { FieldError } from '$lib/components/ui/field-error';
	import { ShieldCheck, Plus, Search, Check } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { getLocalizedError } from '$lib/errors';

	const CONTEXT_ID = 'compliance-policy:create';
	const ROUTE = '/compliance-policies/new';

	type PolicyRule = { actionId: string; gracePeriodHours: number };
	type PolicyDraft = { name: string; description: string; rules: PolicyRule[] };

	function emptyDraft(): PolicyDraft {
		return { name: '', description: '', rules: [] };
	}

	function hydrate(raw: unknown): PolicyDraft | null {
		if (!raw || typeof raw !== 'object') return null;
		const d = raw as Partial<PolicyDraft>;
		return {
			name: typeof d.name === 'string' ? d.name : '',
			description: typeof d.description === 'string' ? d.description : '',
			rules: Array.isArray(d.rules)
				? d.rules
						.filter((r): r is PolicyRule => !!r && typeof r === 'object')
						.filter((r) => typeof r.actionId === 'string')
						.map((r) => ({
							actionId: r.actionId,
							gracePeriodHours: typeof r.gracePeriodHours === 'number' ? r.gracePeriodHours : 0
						}))
				: []
		};
	}

	// Autosave for reload survival. The DraftType union is owned by the SDK and
	// cannot be extended from the web app, so this surface namespaces itself inside
	// the 'create-definition' bucket by id, exactly as /actions/new does.
	const persist = useDraft<PolicyDraft>('create-definition', CONTEXT_ID, emptyDraft());

	// svelte-ignore state_referenced_locally
	const claimed = bindBuilderContext(CONTEXT_ID, () => snapshot());
	// svelte-ignore state_referenced_locally
	let draft = $state<PolicyDraft>(hydrate(claimed) ?? hydrate(persist.data) ?? emptyDraft());

	/** A create surface opens EMPTY: there is nothing to save and nothing worth
	 *  parking. Saying `dirty: true` regardless is what made an untouched form
	 *  offer Save, and auto-stash itself onto the stage on the way out. */
	const PRISTINE = JSON.stringify(emptyDraft());
	const isDirty = () => JSON.stringify($state.snapshot(draft)) !== PRISTINE;

	let saving = $state(false);
	/** Parked on the stage — the pill must NOT re-enter this context. */
	let parked = $state(false);
	/** Transient view state, not part of the buffer. */
	let complianceActions = $state<ManagedAction[]>([]);
	let searchQuery = $state('');
	let creatingAction = $state(false);

	$effect(() => {
		persist.data = $state.snapshot(draft) as PolicyDraft;
	});

	// The check catalogue. F023: page through all SHELL actions instead of capping
	// at 200, then keep only the ones flagged as compliance checks.
	$effect(() => {
		let live = true;
		void (async () => {
			try {
				const all = await fetchAllPages<ManagedAction>(async (size, token) => {
					const r = await apiClient.listActions(size, token, ActionType.SHELL);
					return { items: r.actions, nextPageToken: r.nextPageToken };
				});
				const checks = all.filter((a: ManagedAction) => {
					if (a.type !== ActionType.SHELL) return false;
					return a.params.case === 'shell' ? a.params.value.isCompliance : false;
				});
				if (live) complianceActions = checks;
			} catch (error) {
				console.warn('Failed to load compliance actions', error);
				if (live) complianceActions = [];
			}
		})();
		return () => {
			live = false;
		};
	});

	const filteredActions = $derived(
		complianceActions.filter((a) => {
			if (!searchQuery) return true;
			const q = searchQuery.toLowerCase();
			return a.name.toLowerCase().includes(q) || a.description.toLowerCase().includes(q);
		})
	);

	const errors = $derived.by(() => {
		const out: Record<string, string> = {};
		const result = nameDescriptionSchema.safeParse({
			name: draft.name.trim(),
			description: draft.description.trim()
		});
		if (!result.success) {
			for (const issue of result.error.issues) {
				const field = issue.path.length ? String(issue.path[0]) : '_';
				if (!(field in out)) out[field] = issue.message;
			}
		}
		return out;
	});
	const firstError = $derived(Object.values(errors)[0] ?? null);

	function isSelected(actionId: string): boolean {
		return draft.rules.some((r) => r.actionId === actionId);
	}

	function toggleAction(actionId: string) {
		draft.rules = isSelected(actionId)
			? draft.rules.filter((r) => r.actionId !== actionId)
			: [...draft.rules, { actionId, gracePeriodHours: 0 }];
	}

	function handleActionCreated(action: ManagedAction) {
		complianceActions = [action, ...complianceActions];
		draft.rules = [...draft.rules, { actionId: action.id, gracePeriodHours: 0 }];
		creatingAction = false;
	}

	function snapshot() {
		if (saving || parked) return null;
		return {
			route: ROUTE,
			title: draft.name.trim() || m.compliance_policies_create(),
			dirty: isDirty(),
			valid: firstError === null,
			commitLabel: m.common_create(),
			subtext: firstError ?? m.picker_selected({ count: String(draft.rules.length) }),
			subtextTone: firstError ? ('warn' as const) : ('neutral' as const),
			stashSubtitle: m.common_create_stash_subtitle({
				entity: draft.name.trim() || m.compliance_policies_create()
			}),
			onCommit: () => void commit(),
			onCancel: cancel,
			onStash: () => (parked = true),
			onRestore: () => (parked = false),
			stashPayload: () => $state.snapshot(draft)
		};
	}

	function cancel() {
		void persist.clear();
		void goto('/compliance-policies');
	}

	async function commit() {
		if (firstError) return;
		saving = true;
		try {
			let policy = await apiClient.createCompliancePolicy(
				draft.name.trim(),
				draft.description.trim()
			);
			if (policy) {
				for (const rule of $state.snapshot(draft.rules)) {
					try {
						policy =
							(await apiClient.addCompliancePolicyRule(
								policy.id,
								rule.actionId,
								rule.gracePeriodHours
							)) ?? policy;
					} catch (error) {
						console.error('Failed to add rule', error);
					}
				}
				await persist.clear();
				toast.success(m.compliance_policies_created());
				void goto(`/compliance-policies/${policy.id}`);
			}
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head><title>{m.compliance_policies_create_dialog_title()}</title></svelte:head>

<div class="flex-1 space-y-3 overflow-auto p-4 md:p-6">
	<CreatePlate
		icon={ShieldCheck}
		title={m.compliance_policies_create_dialog_title()}
		description={m.compliance_policies_create_dialog_description()}
		testid="policy-create"
	>
		<IdentityRow
			idPrefix="policy"
			nameLabel={m.common_name()}
			namePlaceholder={m.compliance_policies_name_placeholder()}
			bind:name={draft.name}
			nameError={errors.name}
			descriptionLabel={m.common_description()}
			descriptionPlaceholder={m.compliance_policies_desc_placeholder()}
			bind:description={draft.description}
		/>
	</CreatePlate>

	<CreatePlate
		icon={ShieldCheck}
		title={m.compliance_policy_create_step_rules()}
		description={m.compliance_policy_create_step_rules_description()}
		testid="policy-pick"
	>
		{#if creatingAction}
			<ActionCreateForm
				compact
				initialType="COMPLIANCE_CHECK"
				onCancel={() => (creatingAction = false)}
				onCreated={handleActionCreated}
			/>
		{:else}
			<div class="flex items-center justify-between gap-2">
				<div class="relative flex-1">
					<Search class="absolute top-2.5 left-2.5 h-4 w-4 text-muted-foreground" />
					<Input
						placeholder={m.compliance_policy_detail_select_action()}
						class="pl-9"
						bind:value={searchQuery}
					/>
				</div>
				<Button variant="outline" size="sm" onclick={() => (creatingAction = true)}>
					<Plus class="mr-1 h-4 w-4" />
					{m.compliance_policy_add_rule_create_new()}
				</Button>
			</div>

			{#if filteredActions.length === 0}
				<p class="py-8 text-center text-sm text-muted-foreground">
					{m.compliance_policy_detail_no_compliance_actions()}
				</p>
			{:else}
				<ul class="max-h-[40vh] divide-y overflow-y-auto rounded-lg border">
					{#each filteredActions as action (action.id)}
						<li>
							<button
								type="button"
								data-testid="policy-rule-row"
								data-action-id={action.id}
								onclick={() => toggleAction(action.id)}
								aria-pressed={isSelected(action.id)}
								class="flex w-full items-center gap-3 px-3 py-2 text-left hover:bg-accent/50"
							>
								<!-- A drawn tick, not a Checkbox: the row IS the control, and a
								     nested interactive element inside this button would be both
								     invalid HTML and a second click target. -->
								<span
									aria-hidden="true"
									class="grid h-4 w-4 shrink-0 place-items-center rounded border {isSelected(
										action.id
									)
										? 'border-accent bg-accent-soft text-accent-ink'
										: 'border-input'}"
								>
									{#if isSelected(action.id)}<Check class="h-3 w-3" />{/if}
								</span>
								<ShieldCheck class="h-4 w-4 shrink-0 text-muted-foreground" />
								<span class="min-w-0 flex-1">
									<span class="block truncate text-sm font-medium">{action.name}</span>
									{#if action.description}
										<span class="block truncate text-xs text-muted-foreground"
											>{action.description}</span
										>
									{/if}
								</span>
							</button>
						</li>
					{/each}
				</ul>
			{/if}

			<p class="text-xs text-muted-foreground" data-testid="policy-selected-count">
				{m.picker_selected({ count: String(draft.rules.length) })}
			</p>
		{/if}
	</CreatePlate>
</div>
