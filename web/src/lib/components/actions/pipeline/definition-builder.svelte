<script lang="ts">
	// B1's pipeline, one level up: a definition is an ordered pipeline of action
	// SETS. The palette is round 2's Movement C set-picker (expandable option
	// rows), the centre rail is the same numbered pipeline, and the right panel
	// previews the selected set's own steps.
	//
	// A set has no params form and no PRESENT/ABSENT axis, so steps carry no
	// state toggle here — inventing one would claim a field the contract has not
	// got. Commit lives in the pill; the card has no button bar.
	//
	// The pill is held for the WHOLE visit, not only while the draft diverges: the
	// page hands down the definition's own actions (`entityActions`) and they need
	// a home that does not appear only after an edit.
	import IdentityRow from '$lib/components/create/identity-row.svelte';
	import { toast } from 'svelte-sonner';
	import { base as basePath } from '$app/paths';
	import { apiClient, type ActionSet, type Definition } from '$lib/sdk';
	import { getLocalizedError } from '$lib/errors';
	import * as m from '$lib/paraglide/messages';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import { Layers } from '@lucide/svelte';
	import { getActionTypeInfo } from '../action-type';
	import SetPickerPalette from './set-picker-palette.svelte';
	import PipelineRail from './pipeline-rail.svelte';
	import { bindBuilderContext } from './builder-pill.svelte';
	import type { PillAction } from '$lib/shell/shell.svelte';
	import type { RailStep, SetOption } from './types';

	interface SetStep {
		key: string;
		actionSetId: string;
		name: string;
		memberCount: number;
		/** sortOrder as loaded; -1 for a set the operator just added. */
		originalIndex: number;
	}

	let {
		defId,
		definition,
		members,
		library,
		entityActions = [],
		onsaved
	}: {
		defId: string;
		definition: Definition;
		members: { actionSetId: string; sortOrder: number; actionSetName: string }[];
		/** Every action set the operator may pick from. */
		library: ActionSet[];
		/** The DEFINITION's own actions (delete …), owned by the page and published
		 *  on this builder's context so they ride the same pill as the commit. */
		entityActions?: PillAction[];
		onsaved: () => Promise<void> | void;
	} = $props();

	// Construction-time input: the page remounts this builder on every reload.
	// svelte-ignore state_referenced_locally
	const setsById = new Map<string, ActionSet>(library.flatMap((s) => s.id ? [[s.id.value, s] as const] : []));

	interface Body {
		name: string;
		description: string;
		steps: SetStep[];
		removed: string[];
	}

	function serverBody(): Body {
		const steps = [...members]
			.sort((a, b) => a.sortOrder - b.sortOrder)
			.map((member) => ({
				key: `set-${member.actionSetId}`,
				actionSetId: member.actionSetId,
				name: member.actionSetName,
				memberCount: setsById.get(member.actionSetId ?? '')?.memberCount ?? 0,
				originalIndex: member.sortOrder
			}));
		return { name: definition.name, description: definition.description, steps, removed: [] };
	}

	const base = serverBody();
	const baseline = JSON.stringify(base);

	// No autosave here — see the action-set builder: `useDraft` read its stored
	// value synchronously at construction while the offline store filled its map
	// from IndexedDB after startup, so a reloaded builder always read an empty
	// map. It wrote drafts nobody could ever read. Stash is the durable
	// set-aside mechanism.
	// svelte-ignore state_referenced_locally
	let name = $state(base.name);
	// svelte-ignore state_referenced_locally
	let description = $state(base.description);
	// svelte-ignore state_referenced_locally
	let steps = $state<SetStep[]>([...base.steps]);
	let removed = $state<string[]>([]);
	// svelte-ignore state_referenced_locally
	let selectedKey = $state<string | null>(base.steps[0]?.key ?? null);
	let parked = $state(false);
	let committing = $state(false);
	/** Step names per action-set id, loaded on demand for the palette peek and
	 *  the right panel. */
	let peeked = $state<Record<string, string[] | undefined>>({});

	function body(): Body {
		return $state.snapshot({ name, description, steps, removed }) as Body;
	}

	const dirty = $derived(JSON.stringify(body()) !== baseline);
	const nameError = $derived(name.trim() ? null : m.action_set_detail_builder_name_required());

	const selectedIndex = $derived(steps.findIndex((s) => s.key === selectedKey));

	const railSteps = $derived.by((): RailStep[] =>
		steps.map((step) => ({
			key: step.key,
			title: step.name,
			summary: `${m.action_sets_count({ count: step.memberCount })} · ${step.actionSetId}`,
			icon: Layers
		}))
	);

	const options = $derived.by((): SetOption[] =>
		library
			.filter((s) => !steps.some((step) => (step.actionSetId ?? '') === (typeof s.id === 'string' ? s.id : s.id?.value ?? '')))
			.map((s) => ({ id: typeof s.id === 'string' ? s.id : s.id?.value ?? '', name: s.name, memberCount: s.memberCount }))
	);

	// svelte-ignore state_referenced_locally
	const claimed = bindBuilderContext(`definition:${defId}`, () => {
		// `parked` still ends the hold: a stashed draft that re-entered the context
		// on the next effect run would make Stash a no-op.
		if (committing || parked) return null;
		return {
			// Stash's home: restoring from another page navigates back here and the
			// remount rehydrates from this builder's own useDraft autosave.
			route: `/definitions/${defId}`,
			// The card carries the buffer — see the action-set builder.
			stashPayload: () => ({ ...body(), selectedKey }),
			title: name.trim() || definition.name,
			dirty,
			valid: nameError === null,
			commitLabel: m.common_save(),
			subtext: nameError
				? `${m.action_set_detail_builder_blocked({ count: 1 })} · ${nameError}`
				: m.definition_detail_builder_ready({ count: steps.length }),
			subtextTone: nameError ? ('warn' as const) : ('neutral' as const),
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

	async function peek(setId: string) {
		if (peeked[setId]) return;
		try {
			const response = await apiClient.getActionSet(setId);
			peeked[setId] = [...(response.members ?? [])]
				.sort((a, b) => a.sortOrder - b.sortOrder)
				.map((mem) => `${mem.actionName} · ${getActionTypeInfo(mem.actionType).label}`);
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	function insertAt(setId: string, at: number) {
		const set = setsById.get(setId);
		if (!set || steps.some((s) => (s.actionSetId ?? '') === setId)) return;
		const step: SetStep = {
			key: `set-${setId}`,
			actionSetId: setId,
			name: set.name,
			memberCount: set.memberCount,
			originalIndex: -1
		};
		const index = Math.max(0, Math.min(at, steps.length));
		steps = [...steps.slice(0, index), step, ...steps.slice(index)];
		selectedKey = step.key;
		void peek(setId);
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
		if (step.originalIndex >= 0) removed = [...removed, step.actionSetId];
		steps = steps.filter((s) => s.key !== key);
		if (selectedKey === key) selectedKey = steps[0]?.key ?? null;
	}

	function select(key: string) {
		selectedKey = key;
		const step = steps.find((s) => s.key === key);
		if (step) void peek(step.actionSetId);
	}


	// A card parked on the stage outranks the server body: it is unsaved work and,
	// now that nothing autosaves, the ONLY copy of it. Declared after the bind
	// because the claim is what that call returns.
	if (claimed) {
		const parked = claimed as Body & { selectedKey: string | null };
		if (typeof parked.name === 'string') name = parked.name;
		if (typeof parked.description === 'string') description = parked.description;
		if (Array.isArray(parked.steps)) steps = [...(parked.steps as SetStep[])];
		if (Array.isArray(parked.removed)) removed = [...parked.removed];
		if (parked.selectedKey !== undefined) selectedKey = parked.selectedKey;
	}

	function discard() {
		name = base.name;
		description = base.description;
		steps = [...base.steps];
		removed = [];
		selectedKey = base.steps[0]?.key ?? null;
	}

	async function commit() {
		committing = true;
		try {
			if (name.trim() !== base.name) await apiClient.renameDefinition(defId, name.trim());
			if (description.trim() !== base.description) {
				await apiClient.updateDefinitionDescription(defId, description.trim());
			}
			for (const id of removed) await apiClient.removeActionSetFromDefinition(defId, id);

			const ordered = $state.snapshot(steps) as SetStep[];
			for (let i = 0; i < ordered.length; i++) {
				const step = ordered[i];
				if (step.originalIndex < 0) {
					await apiClient.addActionSetToDefinition(defId, step.actionSetId, i);
					continue;
				}
				if (step.originalIndex !== i) {
					await apiClient.reorderActionSetInDefinition(defId, step.actionSetId, i);
				}
			}

			toast.success(m.definition_detail_builder_saved());
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
		idPrefix="def"
		nameLabel={m.common_name()}
		bind:name
		nameError={nameError ?? undefined}
		descriptionLabel={m.common_description()}
		bind:description
	/>
	
	<div class="grid gap-3 lg:grid-cols-[15rem_minmax(0,1fr)] xl:grid-cols-[15rem_minmax(0,1fr)_27rem]">
		<SetPickerPalette
			title={m.definition_detail_builder_palette()}
			{options}
			oninsert={(id) => insertAt(id, steps.length)}
			onpeek={(id) => void peek(id)}
			{peeked}
			emptyLabel={m.definition_detail_no_sets_available()}
		/>

		<PipelineRail
			steps={railSteps}
			{selectedKey}
			onselect={select}
			onmove={move}
			onremove={remove}
			oninsertAt={insertAt}
			emptyLabel={m.definition_detail_builder_empty()}
			dropLabel={m.definition_detail_builder_drop_hint()}
		/>

		<div class="lg:col-span-2 xl:col-span-1">
			{#if selectedIndex >= 0}
				{@const step = steps[selectedIndex]}
				<div class="rounded-xl border border-hair bg-surface shadow-plate">
					<div class="flex items-baseline gap-2 border-b px-3 py-2">
						<span class="font-mono text-[0.62rem] uppercase tracking-[0.1em] text-faint">
							{m.action_set_detail_builder_step_n({ n: selectedIndex + 1 })}
						</span>
						<span class="truncate text-sm font-semibold">{step.name}</span>
					</div>
					<div class="space-y-1 p-3">
						{#each peeked[step.actionSetId] ?? [] as label, i (i)}
							<p class="truncate font-mono text-[0.7rem] text-muted-foreground">{i + 1} · {label}</p>
						{:else}
							<p class="font-mono text-[0.7rem] text-faint">{m.action_sets_no_actions()}</p>
						{/each}
						<a
							href="{basePath}/action-sets/{step.actionSetId}"
							class="mt-2 inline-block text-xs text-accent-ink hover:underline"
						>
							{m.definition_detail_builder_open_set()}
						</a>
					</div>
				</div>
			{:else}
				<p class="rounded-xl border border-dashed bg-sunken p-4 text-center text-xs text-faint">
					{m.definition_detail_builder_select_step()}
				</p>
			{/if}
		</div>
	</div>
</div>
