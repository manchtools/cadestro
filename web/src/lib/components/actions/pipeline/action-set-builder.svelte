<script lang="ts">
	// B1 "Action-set builder — the full pipeline", for one action set.
	//
	// Three columns: the step-type palette (left), the ordered pipeline (centre)
	// and the selected step's configuration (right). The card carries NO button
	// bar — every exit (Save ⌘S / Stash / Cancel Esc) lives in the context pill,
	// which this component feeds through bindBuilderContext.
	//
	// The pill is held for the WHOLE visit, not only while the draft diverges: the
	// page hands down the set's own actions (`entityActions`) and they need a home
	// that does not appear only after an edit. `dirty` says whether there is
	// anything to commit, so Save/Stash/Cancel stay honest either way.
	//
	// Every pre-existing capability is preserved and routed through the same RPCs
	// the old page used: addActionToSet / removeActionFromSet /
	// reorderActionInSet / renameActionSet / updateActionSetDescription /
	// createAction / updateActionParams. The one B1 affordance deliberately
	// OMITTED is "Dry-run": no RPC previews an action set, and inventing one
	// would be UI over an API that does not exist.
	import IdentityRow from '$lib/components/create/identity-row.svelte';
	import { toast } from 'svelte-sonner';
	import { apiClient, type ActionSet, type ManagedAction } from '$lib/sdk';
	import type { ActionSetMember } from '$contract/cadestro/v1/control_pb';
	import type { ActionType } from '$contract/cadestro/v1/actions_pb';
	import { getLocalizedError } from '$lib/errors';
	import * as m from '$lib/paraglide/messages';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import { Button } from '$lib/components/ui/button';
	import { Plus } from '@lucide/svelte';
	import { setUserLoaders, apiUserLoaders } from '../forms/user-loader-context.svelte';
	import { scheduleFormToProto } from '../forms/types';
	import { ACTION_REGISTRY } from '../registry';
	import {
		getActionTypeInfo,
		getActionTypeInfoByValue,
		getActionTypeOptions
	} from '../action-type';
	import StepPalette from './step-palette.svelte';
	import PipelineRail from './pipeline-rail.svelte';
	import ActionStepPanel from './action-step-panel.svelte';
	import { bindBuilderContext } from './builder-pill.svelte';
	import type { PillAction } from '$lib/shell/shell.svelte';
	import {
		stepFromAction,
		stepFromJson,
		stepFromPalette,
		validateStep,
		type StepDraft
	} from './step-draft';
	import type { PaletteEntry, RailStep, StepState } from './types';

	let {
		setId,
		set,
		members,
		library,
		entityActions = [],
		onsaved
	}: {
		setId: string;
		set: ActionSet;
		members: ActionSetMember[];
		/** Every action the operator may pick from — the picker's source. */
		library: ManagedAction[];
		/** The SET's own actions (delete …), owned by the page and published on
		 *  this builder's context so they ride the same pill as the commit. */
		entityActions?: PillAction[];
		onsaved: () => Promise<void> | void;
	} = $props();

	// Nested UserPicker / GroupParamsForm resolve their loaders from context, the
	// same way the create-form and the params dialog wire them.
	setUserLoaders(apiUserLoaders);

	interface Body {
		name: string;
		description: string;
		steps: StepDraft[];
		removed: string[];
	}

	// Construction-time inputs on purpose: the page remounts this builder on every
	// reload (`{#key revision}`), so a live prop would fight the local draft
	// instead of rebasing it.
	// svelte-ignore state_referenced_locally
	const byId = new Map(library.map((a) => [a.id, a]));

	function serverBody(): Body {
		const steps: StepDraft[] = [];
		for (const member of [...members].sort((a, b) => a.sortOrder - b.sortOrder)) {
			const action = byId.get(member.actionId?.value ?? '');
			// A member whose action is not in the library (deleted, or outside this
			// operator's scope) is skipped rather than rendered as a broken step —
			// commit never touches it, so it stays in the set untouched.
			if (!action) continue;
			const step = stepFromAction(action, member.sortOrder);
			if (step) steps.push(step);
		}
		return { name: set.name, description: set.description, steps, removed: [] };
	}

	const base = serverBody();
	const baseline = JSON.stringify(base);
	const baseSteps = new Map(base.steps.map((s) => [s.actionId, s]));

	/** The baseline must stay pristine — it is what `dirty` and the per-step
	 *  "changed?" checks compare against — so the buffer never shares objects
	 *  with it. Every step field is JSON-shaped, so this round-trip is total. */
	function cloneSteps(source: StepDraft[]): StepDraft[] {
		return JSON.parse(JSON.stringify(source)) as StepDraft[];
	}

	// No autosave here. It wrote every keystroke to IndexedDB and could never
	// read it back — `useDraft` resolves its stored value SYNCHRONOUSLY at
	// construction, while the offline store fills its map from IndexedDB after
	// startup, so a reloaded builder always read an empty map. The caption
	// promising "a full reload survives" was false for the whole life of the
	// feature. Stash is the durable set-aside mechanism, and it is tested.
	// svelte-ignore state_referenced_locally
	let name = $state(base.name);
	// svelte-ignore state_referenced_locally
	let description = $state(base.description);
	// svelte-ignore state_referenced_locally
	let steps = $state<StepDraft[]>(cloneSteps(base.steps));
	let removed = $state<string[]>([]);
	// svelte-ignore state_referenced_locally
	let selectedKey = $state<string | null>(base.steps[0]?.key ?? null);
	let parked = $state(false);
	let committing = $state(false);

	const typeOptions = getActionTypeOptions();

	const paletteEntries: PaletteEntry[] = typeOptions.map((option) => {
		const info = getActionTypeInfoByValue(option.value);
		return { id: option.value, label: info.label, hint: option.value.toLowerCase(), icon: info.icon };
	});

	function body(): Body {
		return $state.snapshot({ name, description, steps, removed }) as Body;
	}

	const dirty = $derived(JSON.stringify(body()) !== baseline);

	// Live per-step validation. Pure schema evaluation, so an error appears and
	// disappears with the keystroke that caused it — no imperative clearing.
	const issues = $derived(
		steps.map((step) => validateStep(step, m.action_set_detail_builder_name_required()))
	);
	const badIndex = $derived(issues.findIndex((i) => i.first !== null));
	const nameError = $derived(name.trim() ? null : m.action_set_detail_builder_name_required());
	const errorCount = $derived(
		issues.filter((i) => i.first !== null).length + (nameError ? 1 : 0)
	);

	const selectedIndex = $derived(steps.findIndex((s) => s.key === selectedKey));

	const railSteps = $derived.by((): RailStep[] =>
		steps.map((step, i) => {
			const adapter = ACTION_REGISTRY[step.formKey];
			const info = getActionTypeInfo(step.actionType);
			return {
				key: step.key,
				title: step.name.trim() || m.action_set_detail_builder_untitled(),
				summary: `${info.label} · ${step.actionId || m.action_set_detail_builder_unsaved()}`,
				icon: info.icon,
				state: adapter.supportsAbsent ? (step.desiredState === 1 ? 'absent' : 'present') : 'run',
				// Switchable ONLY while the step is being authored. A step that
				// references a library action states its state but cannot change it:
				// there is no set-local override to write, so a toggle here could
				// only edit the shared action — arming an uninstall everywhere it is
				// assigned — or, once commit stopped writing it, lie about a change
				// that never saved. One option renders the group disabled.
				stateOptions:
					step.isNew && adapter.supportsAbsent
						? (['present', 'absent'] as StepState[])
						: ([adapter.supportsAbsent
								? step.desiredState === 1
									? 'absent'
									: 'present'
								: 'run'] as StepState[]),
				error: issues[i]?.first ?? undefined
			};
		})
	);

	// Actions the set does not already carry — the palette must not offer a member
	// twice, and a step queued for removal is available again.
	const availableActions = $derived(
		library.filter((a) => !steps.some((s) => s.actionId === a.id))
	);
	/** The Existing tab: the library itself, in the palette, instead of behind a
	 *  dialog button under the fold. */
	const existingEntries = $derived<PaletteEntry[]>(
		availableActions.map((a) => {
			const info = getActionTypeInfo(a.type);
			return { id: a.id, label: a.name, hint: info.label, icon: info.icon };
		})
	);

	// ── the pill owns the commit ────────────────────────────────────────────
	// svelte-ignore state_referenced_locally
	const claimed = bindBuilderContext(`action-set:${setId}`, () => {
		// `parked` still ends the hold: a stashed draft that re-entered the context
		// on the next effect run would make Stash a no-op.
		if (committing || parked) return null;
		const blocked = errorCount > 0;
		return {
			// Stash's home: restoring from another page navigates back here and the
			// remount rehydrates from this builder's own useDraft autosave.
			route: `/action-sets/${setId}`,
			// The card carries the buffer. This surface is component state, so a
			// cross-route restore remounts it empty — it used to lean on a useDraft
			// autosave that could never read itself back, which meant parking a
			// builder and walking away silently lost the work.
			stashPayload: () => ({ ...body(), selectedKey }),
			title: name.trim() || set.name,
			dirty,
			valid: !blocked,
			commitLabel: m.common_save(),
			subtext: blocked
				? `${m.action_set_detail_builder_blocked({ count: errorCount })} · ${
						badIndex >= 0
							? m.action_set_detail_builder_error_at({
									step: badIndex + 1,
									reason: issues[badIndex]?.first ?? ''
								})
							: (nameError ?? '')
					}`
				: m.action_set_detail_builder_ready({ count: steps.length }),
			subtextTone: blocked ? ('warn' as const) : ('neutral' as const),
			stashSubtitle: m.action_set_detail_builder_stash_subtitle({
				step: selectedIndex >= 0 ? selectedIndex + 1 : 1
			}),
			onCommit: commit,
			onCancel: discard,
			onStash: () => (parked = true),
			onRestore: () => (parked = false),
			extraActions: entityActions
		};
	});

	function select(key: string) {
		selectedKey = key;
	}

	function insertAt(typeValue: string, at: number) {
		const option = typeOptions.find((o) => o.value === typeValue);
		if (!option) return;
		const step = stepFromPalette(
			typeValue,
			option.type as ActionType,
			getActionTypeInfoByValue(typeValue).label
		);
		if (!step) return;
		const index = Math.max(0, Math.min(at, steps.length));
		steps = [...steps.slice(0, index), step, ...steps.slice(index)];
		selectedKey = step.key;
	}

	function move(index: number, dir: 'up' | 'down') {
		const target = dir === 'up' ? index - 1 : index + 1;
		if (target < 0 || target >= steps.length) return;
		const next = [...steps];
		const [moved] = next.splice(index, 1);
		next.splice(target, 0, moved);
		steps = next;
	}

	function remove(key: string) {
		const step = steps.find((s) => s.key === key);
		if (!step) return;
		// Only a step that is already a member needs a removal RPC; one the
		// operator just dropped in simply disappears.
		if (step.originalIndex >= 0 && step.actionId) removed = [...removed, step.actionId];
		steps = steps.filter((s) => s.key !== key);
		if (selectedKey === key) selectedKey = steps[0]?.key ?? null;
	}

	function setState(key: string, next: StepState) {
		const step = steps.find((s) => s.key === key);
		if (!step || next === 'run') return;
		// Defence in depth: the rail already renders a referenced step's group
		// disabled, but a library action's state is never this surface's to write.
		if (!step.isNew) return;
		step.desiredState = next === 'absent' ? 1 : 0;
	}


	function addExisting(ids: string[]) {
		const added: StepDraft[] = [];
		for (const id of ids) {
			const action = byId.get(id);
			if (!action) continue;
			// originalIndex -1 marks "existing action, not yet a member": commit
			// calls addActionToSet without creating anything.
			const step = stepFromAction(action, -1);
			if (step) added.push(step);
		}
		if (!added.length) return;
		steps = [...steps, ...added];
		selectedKey = added[0].key;
	}


	// A card parked on the stage outranks the server body: it is unsaved work and,
	// now that nothing autosaves, the ONLY copy of it. Declared after the bind
	// because the claim is what that call returns.
	if (claimed) {
		const parked = claimed as Body & { selectedKey: string | null };
		if (typeof parked.name === 'string') name = parked.name;
		if (typeof parked.description === 'string') description = parked.description;
		if (Array.isArray(parked.steps)) {
			const rebuilt = parked.steps
				.map(stepFromJson)
				.filter((step): step is StepDraft => step !== null);
			if (rebuilt.length || parked.steps.length === 0) steps = rebuilt;
		}
		if (Array.isArray(parked.removed)) removed = [...parked.removed];
		if (parked.selectedKey !== undefined) selectedKey = parked.selectedKey;
	}

	function discard() {
		name = base.name;
		description = base.description;
		steps = cloneSteps(base.steps);
		removed = [];
		selectedKey = base.steps[0]?.key ?? null;
	}

	function createRequest(step: StepDraft) {
		const adapter = ACTION_REGISTRY[step.formKey];
		return {
			name: step.name.trim(),
			description: step.description.trim(),
			type: step.actionType,
			desiredState: adapter.supportsAbsent ? step.desiredState : 0,
			timeoutSeconds: step.timeoutSeconds,
			schedule: scheduleFormToProto(step.schedule),
			params: {
				case: adapter.paramsCase,
				value: adapter.formToProto(step.params)
			} as Parameters<typeof apiClient.createAction>[0]['params']
		};
	}



	// Commit order is load-bearing: removals free their slots first, then the
	// final order is written index by index, so no intermediate state has two
	// members claiming one sortOrder.
	async function commit() {
		committing = true;
		try {
			if (name.trim() !== base.name) await apiClient.renameActionSet(setId, name.trim());
			if (description.trim() !== base.description) {
				await apiClient.updateActionSetDescription(setId, description.trim());
			}
			for (const id of removed) await apiClient.removeActionFromSet(setId, id);

			const ordered = $state.snapshot(steps) as StepDraft[];
			for (let i = 0; i < ordered.length; i++) {
				const step = ordered[i];
				if (step.isNew) {
					const created = await apiClient.createAction(createRequest(step));
					if (created) await apiClient.addActionToSet(setId, created.id, i);
					continue;
				}
				if (step.originalIndex < 0) {
					await apiClient.addActionToSet(setId, step.actionId, i);
					continue;
				}
				// No action mutation from here. A member is a REFERENCE to a library
				// action: renaming it, retyping its params or flipping its desired
				// state from inside one set changed it for every other set and
				// assignment using it — a REMOVE toggled here armed an uninstall
				// fleet-wide. The set commits membership and order only.
				if (step.originalIndex !== i) {
					await apiClient.reorderActionInSet(setId, step.actionId, i);
				}
			}

			toast.success(m.action_set_detail_builder_saved());
			await onsaved();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			committing = false;
		}
	}
</script>

<div class="rounded-xl border border-hair bg-surface p-3 shadow-plate">
	<!-- The same opening row every create and edit surface uses. The two
	     hand-rolled halves here drifted: a one-row Textarea beside a plain
	     Input, with different label spacing on each side. -->
	<IdentityRow
		idPrefix="set"
		nameLabel={m.common_name()}
		bind:name
		nameError={nameError ?? undefined}
		descriptionLabel={m.common_description()}
		bind:description
	/>
	
	<!-- palette · pipeline · step config. Below xl the config panel drops under the
	     pipeline instead of squeezing it (Twenty/Airtable rail + right panel). -->
	<div class="grid gap-3 lg:grid-cols-[15rem_minmax(0,1fr)] xl:grid-cols-[15rem_minmax(0,1fr)_27rem]">
		<StepPalette
			title={m.action_set_detail_builder_palette()}
			entries={paletteEntries}
			oninsert={(id) => insertAt(id, steps.length)}
			existing={existingEntries}
			oninsertExisting={(id) => addExisting([id])}
			existingEmptyLabel={m.step_palette_existing_empty()}
			searchPlaceholder={m.action_set_detail_builder_filter_types()}
			emptyLabel={m.common_no_results_search()}
		/>

		<PipelineRail
			steps={railSteps}
			{selectedKey}
			onselect={select}
			onmove={move}
			onremove={remove}
			onstate={setState}
			oninsertAt={insertAt}
			emptyLabel={m.action_set_detail_builder_empty()}
			dropLabel={m.action_set_detail_builder_drop_hint()}
		/>

		<div class="lg:col-span-2 xl:col-span-1">
			{#if selectedIndex >= 0}
				{#key steps[selectedIndex].key}
					<ActionStepPanel
						bind:step={steps[selectedIndex]}
						index={selectedIndex}
						errors={issues[selectedIndex]?.fields ?? {}}
					/>
				{/key}
			{:else}
				<p class="rounded-xl border border-dashed bg-sunken p-4 text-center text-xs text-faint">
					{m.action_set_detail_builder_select_step()}
				</p>
			{/if}
		</div>
	</div>
</div>
