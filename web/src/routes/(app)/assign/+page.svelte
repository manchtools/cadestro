<script lang="ts">

	import { onMount } from 'svelte';
	import { base } from '$app/paths';
	import { goto } from '$app/navigation';
	import { toast } from 'svelte-sonner';
	import * as m from '$lib/paraglide/messages';
	import { getLocalizedError } from '$lib/errors';
	import { getCarried, setCarried } from '$lib/shell/carried-selection.svelte';
	import {
		shell,
		enterContext,
		updateContext,
		exitContext,
		leaveContext,
		claimDraft,
		type ContextState
	} from '$lib/shell/shell.svelte';
	import FutureScopeDialog from '$lib/components/future-scope-dialog.svelte';
	import type { QueryEditorState } from '$lib/components/query-builder.svelte';
	import type { ActionSet, ActionSetMember } from '$contract/cadestro/v1/control_pb';
	import {
		MAX_CARRIED,
		assignSetToGroup,
		commitAssign,
		createRuleGroup,
		loadActionSets,
		loadAssignedDeviceIds,
		loadCarriedDevices,
		loadSetSteps,
		type AssignOutcome,
		type AssignSchedule,
		type RuleGroup
	} from './assign-data';
	import { computeEligibility, hostnameGroups, type CarriedDevice } from './eligibility';
	import {
		clearAssignDraft,
		stashAssignDraft,
		takeAssignDraft,
		type AssignDraft,
		type TargetMode
	} from './draft.svelte';
	import CarriedStage from './carried-stage.svelte';
	import RuleStage from './rule-stage.svelte';
	import SetSheet from './set-sheet.svelte';

	const CONTEXT_ID = 'assign';

	const ROUTE = '/assign';

	const carried = $derived(getCarried());
	const deviceIds = $derived(carried?.deviceIds ?? []);
	const overCap = $derived(deviceIds.length > MAX_CARRIED);
	const hasCarried = $derived(deviceIds.length > 0);

	let mode = $state<TargetMode>('carried');

	let devices = $state<CarriedDevice[]>([]);
	let devicesLoading = $state(false);

	let sets = $state<ActionSet[]>([]);
	let setsLoading = $state(true);
	let loadError = $state<string | null>(null);

	let setId = $state<string | null>(null);
	let schedule = $state<AssignSchedule>('now');
	let steps = $state<ActionSetMember[]>([]);
	let stepsLoading = $state(false);
	let assignedIds = $state<ReadonlySet<string>>(new Set());

	let committing = $state(false);
	let failures = $state<AssignOutcome[]>([]);

	let parked = $state(false);

	let closed = $state(false);

	let ruleQuery = $state('');
	let groupName = $state('');
	let ruleState = $state<QueryEditorState>({
		text: '',
		valid: false,
		count: null,
		error: m.query_incomplete(),
		validating: false
	});

	let confirmOpen = $state(false);

	let savedGroup = $state<RuleGroup | null>(null);

	let savedFor = $state<{ name: string; query: string } | null>(null);
	let savingGroup = $state(false);
	let ruleError = $state<string | null>(null);

	const eligibility = $derived(computeEligibility(devices, assignedIds));
	const groups = $derived(hostnameGroups(devices));
	const chosenSet = $derived(sets.find((s) => (s.id?.value ?? '') === setId) ?? null);

	const caption = $derived.by(() => {
		if (!chosenSet) return m.assign_caption_choose();
		const rollup = m.assign_caption({
			set: chosenSet.name,
			ready: eligibility.ready,
			update: eligibility.update,
			queued: eligibility.queued
		});
		return eligibility.unknown > 0
			? rollup + m.assign_caption_unknown({ count: eligibility.unknown })
			: rollup;
	});

	const ruleCountable = $derived(
		ruleState.valid === true && ruleState.count !== null && ruleState.text.trim().length > 0
	);
	const ruleReady = $derived(ruleCountable && groupName.trim().length > 0 && setId !== null);
	const reusableGroup = $derived(
		savedGroup &&
			savedFor &&
			savedFor.query === ruleState.text &&
			savedFor.name === groupName.trim()
			? savedGroup
			: null
	);

	const ruleCaption = $derived.by((): { text: string; tone: 'neutral' | 'warn' } => {
		const query = ruleState.text;
		if (!query.trim()) return { text: m.query_incomplete(), tone: 'warn' };
		if (ruleState.validating) return { text: `${m.query_counting()} · ${query}`, tone: 'neutral' };
		if (!ruleState.valid) return { text: `${ruleState.error} · ${query}`, tone: 'warn' };
		if (ruleState.count === null) {
			return { text: query, tone: 'neutral' };
		}
		const body = `${m.query_match_count_devices({ count: ruleState.count })} · ${query}`;
		if (setId === null) return { text: `${m.assign_caption_choose()} · ${body}`, tone: 'warn' };
		if (!groupName.trim()) return { text: `${m.assign_rule_name_required()} · ${body}`, tone: 'warn' };
		return { text: body, tone: 'neutral' };
	});

	let readNonce = $state(0);

	async function loadSets() {
		setsLoading = true;
		loadError = null;
		try {
			sets = await loadActionSets();
		} catch (error) {
			loadError = getLocalizedError(error);
		} finally {
			setsLoading = false;
		}
	}

	function retry() {
		void loadSets();
		readNonce++;
	}

	onMount(() => {

		claimDraft(CONTEXT_ID);

		const draft = takeAssignDraft();
		if (draft) {
			mode = draft.mode;
			setId = draft.setId;
			schedule = draft.schedule;
			ruleQuery = draft.query;
			groupName = draft.groupName;
		}
		void loadSets();
	});

	$effect(() => {
		const ids = deviceIds;
		void readNonce;
		if (mode !== 'carried') return;
		if (!ids.length || ids.length > MAX_CARRIED) {
			devices = [];
			return;
		}
		let stale = false;
		devicesLoading = true;
		loadCarriedDevices(ids)
			.then((read) => {
				if (stale) return;
				devices = read;
				devicesLoading = false;
			})
			.catch((error) => {
				if (stale) return;
				devicesLoading = false;
				loadError = getLocalizedError(error);
			});
		return () => {
			stale = true;
		};
	});

	$effect(() => {
		const id = setId;
		void readNonce;
		if (!id) {
			steps = [];
			assignedIds = new Set();
			return;
		}
		let stale = false;
		stepsLoading = true;
		loadSetSteps(id)
			.then((members) => {
				if (stale) return;
				steps = members;
				stepsLoading = false;
			})
			.catch((error) => {
				if (stale) return;
				stepsLoading = false;
				loadError = getLocalizedError(error);
			});
		if (mode === 'carried') {
			loadAssignedDeviceIds(id)
				.then((ids) => {
					if (!stale) assignedIds = ids;
				})
				.catch((error) => {

					if (!stale) loadError = getLocalizedError(error);
				});
		}
		return () => {
			stale = true;
		};
	});

	function selectMode(next: TargetMode) {
		if (next === mode) return;
		if (next === 'carried' && !hasCarried) return;
		mode = next;
	}

	async function runCommit() {
		const id = setId;
		const ids = deviceIds;
		if (!id || !ids.length) return;
		committing = true;
		failures = [];
		const outcomes = await commitAssign(ids, id, schedule);
		committing = false;
		const failed = outcomes.filter((o) => !o.ok);
		if (failed.length) {

			failures = failed;
			toast.error(
				m.assign_commit_partial({ ok: outcomes.length - failed.length, failed: failed.length })
			);
			return;
		}
		closed = true;
		toast.success(m.assign_commit_success({ count: outcomes.length }));
		setCarried(null);
		clearAssignDraft();
	}

	function openRuleConfirm() {
		if (!ruleReady) return;
		confirmOpen = true;
	}

	function cancelRuleConfirm() {

		confirmOpen = false;
	}

	async function confirmRuleCommit() {
		const id = setId;
		confirmOpen = false;
		if (!id || !ruleReady) return;
		const name = groupName.trim();
		const query = ruleState.text;
		committing = true;
		ruleError = null;

		let group = reusableGroup;
		if (!group) {
			try {
				group = await createRuleGroup(name, query);
			} catch (error) {
				committing = false;
				ruleError = `${m.assign_rule_failed()} ${getLocalizedError(error)}`;
				toast.error(m.assign_rule_failed());
				return;
			}
			savedGroup = group;
			savedFor = { name, query };
		}

		try {
			await assignSetToGroup(id, group.id);
		} catch (error) {

			committing = false;
			ruleError = `${m.assign_rule_group_kept({ name: group.name })} ${getLocalizedError(error)}`;
			toast.error(m.assign_rule_failed());
			return;
		}

		committing = false;
		closed = true;
		clearAssignDraft();
		toast.success(m.assign_rule_commit_success({ name: group.name }));

		void goto(`${base}/device-groups/${group.id}`);
	}

	async function saveAsGroup() {
		const name = groupName.trim();
		if (!ruleCountable || !name || savingGroup || reusableGroup) return;
		savingGroup = true;
		ruleError = null;
		try {
			const group = await createRuleGroup(name, ruleState.text);
			savedGroup = group;
			savedFor = { name, query: ruleState.text };
			toast.success(m.assign_rule_group_saved({ name: group.name }));
		} catch (error) {
			ruleError = `${m.assign_rule_failed()} ${getLocalizedError(error)}`;
			toast.error(m.assign_rule_failed());
		} finally {
			savingGroup = false;
		}
	}

	function runCancel() {
		closed = true;
		clearAssignDraft();
		void goto(`${base}/devices`);
	}

	function draftSnapshot(): AssignDraft {
		return { mode, setId, schedule, query: ruleQuery, groupName };
	}

	function stash() {
		stashAssignDraft(draftSnapshot());
		parked = true;
	}

	function restore() {
		parked = false;
	}

	function ruleSnapshot(): Omit<ContextState, 'id'> {
		return {
			route: ROUTE,
			title: m.assign_rule_pill_title(),
			dirty: ruleState.text.length > 0 || groupName.length > 0 || setId !== null,

			valid: ruleReady,
			commitLabel:
				ruleState.count !== null
					? m.assign_commit_label({ count: ruleState.count })
					: m.assign_rule_commit_pending(),
			subtext: ruleCaption.text,
			subtextTone: ruleCaption.tone,
			onCommit: openRuleConfirm,
			onCancel: runCancel,
			onStash: stash,
			onRestore: restore,
			stashSubtitle: m.assign_rule_stash_subtitle({
				name: groupName.trim() || m.assign_rule_pill_title()
			}),
			extraActions: [
				{ id: 'save-as-group', label: m.assign_rule_save_group(), onRun: () => void saveAsGroup() }
			]
		};
	}

	function snapshot(): Omit<ContextState, 'id'> | null {
		if (parked || closed || committing || confirmOpen) return null;
		if (mode === 'rule') return ruleSnapshot();
		const sel = carried;
		if (!sel || overCap) return null;
		return {
			route: ROUTE,
			title: m.assign_pill_title({ label: sel.label }),
			dirty: setId !== null,

			valid: setId !== null,
			commitLabel: m.assign_commit_label({ count: sel.deviceIds.length }),
			subtext: caption,
			onCommit: () => void runCommit(),
			onCancel: runCancel,
			onStash: stash,
			onRestore: restore,
			stashSubtitle: chosenSet ? m.assign_stash_subtitle({ set: chosenSet.name }) : undefined
		};
	}

	$effect(() => {
		const next = snapshot();
		const held = shell.pill.context?.id === CONTEXT_ID;
		if (!next) {
			if (held) exitContext();
			return;
		}

		if (held) updateContext(next);
		else enterContext({ id: CONTEXT_ID, ...next });
	});

	$effect(() => () => {

			leaveContext(CONTEXT_ID);
	});
</script>

<svelte:head><title>{m.assign_title()}</title></svelte:head>

<div class="flex h-full min-h-0 flex-col overflow-auto">
	<div class="mb-2 flex items-center gap-2" data-testid="assign-target-mode">
		<span class="font-mono text-[0.66rem] tracking-[0.08em] text-faint uppercase">
			{m.assign_target_mode_label()}
		</span>
		<div
			role="radiogroup"
			aria-label={m.assign_target_mode_label()}
			class="inline-flex overflow-hidden rounded-[7px] border border-border font-mono text-[0.68rem]"
		>
			<button
				type="button"
				role="radio"
				aria-checked={mode === 'carried'}
				disabled={!hasCarried}
				onclick={() => selectMode('carried')}
				class="border-r border-border px-2 py-1 {mode === 'carried'
					? 'bg-accent-soft font-semibold text-accent-ink'
					: 'text-faint'} {hasCarried ? '' : 'cursor-not-allowed opacity-40'}"
			>
				{m.assign_target_carried()}
			</button>
			<button
				type="button"
				role="radio"
				aria-checked={mode === 'rule'}
				onclick={() => selectMode('rule')}
				class="px-2 py-1 {mode === 'rule'
					? 'bg-accent-soft font-semibold text-accent-ink'
					: 'text-faint'}"
			>
				{m.assign_target_rule()}
			</button>
		</div>
	</div>

	{#if loadError}
		<div
			class="mb-2 flex items-center gap-3 rounded-lg border border-crit/40 bg-crit-soft px-3 py-2 text-sm text-crit"
		>
			<span>{m.assign_load_failed()} {loadError}</span>
			<button
				type="button"
				class="ml-auto rounded-md border border-crit/40 px-2 py-0.5 text-xs"
				onclick={retry}
			>
				{m.assign_retry()}
			</button>
		</div>
	{/if}
	{#if committing}
		<p data-testid="assign-committing" class="mb-2 text-sm text-muted-foreground">
			{m.assign_committing()}
		</p>
	{/if}

	{#if mode === 'rule'}
		<div
			class="grid min-h-0 flex-1 grid-cols-1 overflow-hidden rounded-xl border border-border shadow-plate md:grid-cols-[1fr_18rem]"
		>
			<RuleStage
				bind:query={ruleQuery}
				bind:groupName
				state={ruleState}
				{savedGroup}
				savedGroupHref={savedGroup ? `${base}/device-groups/${savedGroup.id}` : ''}
				{savingGroup}
				error={ruleError}
				onstate={(next) => (ruleState = next)}
			/>
			<SetSheet
				{sets}
				loading={setsLoading}
				selectedId={setId}
				{steps}
				{stepsLoading}
				{schedule}
				showSchedule={false}
				onselect={(id) => (setId = id)}
				onschedule={(next) => (schedule = next)}
			/>
		</div>
	{:else if !carried || !carried.deviceIds.length}
		<div data-testid="assign-empty" class="m-auto max-w-md space-y-2 text-center">
			<h1 class="truncate text-2xl font-bold">{m.assign_empty_title()}</h1>
			<p class="text-sm text-muted-foreground">{m.assign_empty_hint()}</p>
			<a href="{base}/devices" class="inline-block text-sm font-medium text-accent-ink underline">
				{m.assign_empty_link()}
			</a>
		</div>
	{:else if overCap}
		<div data-testid="assign-over-cap" class="m-auto max-w-md space-y-2 text-center">
			<h1 class="truncate text-2xl font-bold">{m.assign_too_many_title()}</h1>
			<p class="text-sm text-muted-foreground">
				{m.assign_too_many_hint({ max: MAX_CARRIED, count: carried.deviceIds.length })}
			</p>
			<a href="{base}/devices" class="inline-block text-sm font-medium text-accent-ink underline">
				{m.assign_empty_link()}
			</a>
		</div>
	{:else}
		<div
			class="grid min-h-0 flex-1 grid-cols-1 overflow-hidden rounded-xl border border-border shadow-plate md:grid-cols-[1fr_18rem]"
		>
			<CarriedStage
				label={carried.label}
				{devices}
				loading={devicesLoading}
				{groups}
				{eligibility}
				{failures}
			/>
			<SetSheet
				{sets}
				loading={setsLoading}
				selectedId={setId}
				{steps}
				{stepsLoading}
				{schedule}
				onselect={(id) => (setId = id)}
				onschedule={(next) => (schedule = next)}
			/>
		</div>
	{/if}
</div>

<FutureScopeDialog
	bind:open={confirmOpen}
	queryText={ruleState.text}
	count={ruleState.count}
	kind="device"
	note={m.assign_rule_confirm_group({ name: groupName.trim() })}
	onconfirm={() => void confirmRuleCommit()}
	oncancel={cancelRuleConfirm}
/>
