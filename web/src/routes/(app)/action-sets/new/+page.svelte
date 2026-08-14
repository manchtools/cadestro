<script lang="ts">
	// /action-sets/new — set creation as a pill-committed surface.
	//
	// The modal it replaces was a three-step wizard behind a dialog footer: name
	// and description, then an action picker, then an inline action create. All of
	// that is real unfinished work an operator can be interrupted in the middle of,
	// and none of it could be parked. On the route the wizard collapses into two
	// plates on one page — there is no "Next", because the commit is the pill's.
	//
	// The RPC sequence is unchanged: CreateActionSet, then AddActionToSet once per
	// selected action, in the operator's selection order.
	import { toast } from 'svelte-sonner';
	import { goto } from '$lib/navigation';
	import { create } from '@bufbuild/protobuf';
	import { apiClient, useDraft, type ManagedAction } from '$lib/sdk';
	import { ActionScheduleSchema } from '$contract/cadestro/v1/actions_pb';
	import { OnFailure } from '$contract/cadestro/v1/agent_pb';
	import { nameDescriptionSchema } from '$lib/forms/schemas/common';
	import { bindBuilderContext } from '$lib/components/actions/pipeline/builder-pill.svelte';
	import CreatePlate from '$lib/components/create/create-plate.svelte';
	import IdentityRow from '$lib/components/create/identity-row.svelte';
	import { ActionCreateForm, getActionTypeLabel } from '$lib/components/actions';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import { Badge } from '$lib/components/ui/badge';
	import { FieldError } from '$lib/components/ui/field-error';
	import { Layers, Plus, Search, Check } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { getLocalizedError } from '$lib/errors';

	const CONTEXT_ID = 'action-set:create';
	const ROUTE = '/action-sets/new';

	type SetDraft = { name: string; description: string; selectedActionIds: string[] };

	function emptyDraft(): SetDraft {
		return { name: '', description: '', selectedActionIds: [] };
	}

	function hydrate(raw: unknown): SetDraft | null {
		if (!raw || typeof raw !== 'object') return null;
		const d = raw as Partial<SetDraft>;
		return {
			name: typeof d.name === 'string' ? d.name : '',
			description: typeof d.description === 'string' ? d.description : '',
			selectedActionIds: Array.isArray(d.selectedActionIds)
				? d.selectedActionIds.filter((id): id is string => typeof id === 'string')
				: []
		};
	}

	// Autosave for reload survival. The DraftType union is owned by the SDK and
	// cannot be extended from the web app, so this surface namespaces itself inside
	// the 'create-definition' bucket by id, exactly as /actions/new does.
	const persist = useDraft<SetDraft>('create-definition', CONTEXT_ID, emptyDraft());

	// svelte-ignore state_referenced_locally
	const claimed = bindBuilderContext(CONTEXT_ID, () => snapshot());
	// svelte-ignore state_referenced_locally
	let draft = $state<SetDraft>(hydrate(claimed) ?? hydrate(persist.data) ?? emptyDraft());

	/** A create surface opens EMPTY: there is nothing to save and nothing worth
	 *  parking. Saying `dirty: true` regardless is what made an untouched form
	 *  offer Save, and auto-stash itself onto the stage on the way out. */
	const PRISTINE = JSON.stringify(emptyDraft());
	const isDirty = () => JSON.stringify($state.snapshot(draft)) !== PRISTINE;

	let saving = $state(false);
	/** Parked on the stage — the pill must NOT re-enter this context. */
	let parked = $state(false);
	/** Transient view state, not part of the buffer. */
	let allActions = $state<ManagedAction[]>([]);
	let searchQuery = $state('');
	let creatingAction = $state(false);

	$effect(() => {
		persist.data = $state.snapshot(draft) as SetDraft;
	});

	// The picker's catalogue. Loaded once on mount instead of behind a wizard step,
	// because there is no step to be behind any more.
	$effect(() => {
		let live = true;
		void (async () => {
			try {
				const response = await apiClient.listActions();
				if (live) allActions = response.actions;
			} catch (error) {
				console.warn('Failed to load actions', error);
				if (live) allActions = [];
			}
		})();
		return () => {
			live = false;
		};
	});

	const filteredActions = $derived(
		allActions.filter((a) => {
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
		return draft.selectedActionIds.includes(actionId);
	}

	function toggleAction(actionId: string) {
		draft.selectedActionIds = isSelected(actionId)
			? draft.selectedActionIds.filter((id) => id !== actionId)
			: [...draft.selectedActionIds, actionId];
	}

	function handleActionCreated(action: ManagedAction) {
		allActions = [action, ...allActions];
		draft.selectedActionIds = [...draft.selectedActionIds, action.id];
		creatingAction = false;
	}

	function snapshot() {
		if (saving || parked) return null;
		return {
			route: ROUTE,
			title: draft.name.trim() || m.action_sets_create(),
			dirty: isDirty(),
			valid: firstError === null,
			commitLabel: m.common_create(),
			subtext:
				firstError ?? m.picker_selected({ count: String(draft.selectedActionIds.length) }),
			subtextTone: firstError ? ('warn' as const) : ('neutral' as const),
			stashSubtitle: m.common_create_stash_subtitle({
				entity: draft.name.trim() || m.action_sets_create()
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
		void goto('/action-sets');
	}

	async function commit() {
		if (firstError) return;
		saving = true;
		try {
			const set = await apiClient.createActionSet({
				name: draft.name.trim(),
				description: draft.description.trim(),
				schedule: create(ActionScheduleSchema, { intervalHours: 8 }),
				onFailure: OnFailure.CONTINUE
			});
			if (set) {
				const ids = $state.snapshot(draft.selectedActionIds);
				for (let i = 0; i < ids.length; i++) {
					try {
						await apiClient.addActionToSet(set.id, ids[i], i);
					} catch (error) {
						console.error('Failed to add action to set', error);
					}
				}
				await persist.clear();
				toast.success(m.action_sets_created());
				void goto(`/action-sets/${set.id}`);
			}
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head><title>{m.action_sets_create_dialog_title()}</title></svelte:head>

<div class="flex-1 space-y-3 overflow-auto p-4 md:p-6">
	<CreatePlate
		icon={Layers}
		title={m.action_sets_create_dialog_title()}
		description={m.action_sets_create_dialog_description()}
		testid="action-set-create"
	>
		<IdentityRow
			idPrefix="set"
			nameLabel={m.common_name()}
			namePlaceholder={m.action_sets_name_placeholder()}
			bind:name={draft.name}
			nameError={errors.name}
			descriptionLabel={m.common_description()}
			descriptionPlaceholder={m.action_sets_desc_placeholder()}
			bind:description={draft.description}
		/>
	</CreatePlate>

	<CreatePlate
		icon={Layers}
		title={m.action_sets_create_step_actions()}
		description={m.action_sets_create_step_actions_description()}
		testid="action-set-pick"
	>
		{#if creatingAction}
			<ActionCreateForm
				compact
				onCancel={() => (creatingAction = false)}
				onCreated={handleActionCreated}
			/>
		{:else}
			<div class="flex items-center justify-between gap-2">
				<div class="relative flex-1">
					<Search class="absolute top-2.5 left-2.5 h-4 w-4 text-muted-foreground" />
					<Input
						placeholder={m.action_set_detail_search_actions()}
						class="pl-9"
						bind:value={searchQuery}
					/>
				</div>
				<Button variant="outline" size="sm" onclick={() => (creatingAction = true)}>
					<Plus class="mr-1 h-4 w-4" />
					{m.action_picker_create_new()}
				</Button>
			</div>

			{#if filteredActions.length === 0}
				<p class="py-8 text-center text-sm text-muted-foreground">
					{m.action_set_detail_no_actions_available()}
				</p>
			{:else}
				<ul class="max-h-[40vh] divide-y overflow-y-auto rounded-lg border">
					{#each filteredActions as action (action.id)}
						<li>
							<button
								type="button"
								data-testid="set-action-row"
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
								<span class="min-w-0 flex-1">
									<span class="block truncate text-sm font-medium">{action.name}</span>
									{#if action.description}
										<span class="block truncate text-xs text-muted-foreground"
											>{action.description}</span
										>
									{/if}
								</span>
								<Badge variant="outline">{getActionTypeLabel(action.type)}</Badge>
							</button>
						</li>
					{/each}
				</ul>
			{/if}

			<p class="text-xs text-muted-foreground" data-testid="set-selected-count">
				{m.picker_selected({ count: String(draft.selectedActionIds.length) })}
			</p>
		{/if}
	</CreatePlate>
</div>
