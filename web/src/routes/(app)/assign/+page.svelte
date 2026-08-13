<script lang="ts">
	// B2 + B3 — assign an action set to a target.
	//
	// Two targeting modes share one surface and one pill context:
	//
	//   carried — the fleet page hands this surface a set of device ids through
	//     the carried selection. Snapshot targeting: these devices, now.
	//   rule    — B3. The chips compile a query, ValidateDynamicQuery counts it
	//     live, and committing saves the rule as a dynamic device group and
	//     assigns the set to THAT GROUP. Standing targeting: whatever matches,
	//     now and later. Because that keeps applying without another approval,
	//     the commit is gated by a real confirm, not by a banner.
	//
	// The pill morphs selection → context and "Assign to N →" IS the pill's Save,
	// so this page renders NO commit button of its own — in either mode.
	//
	// Every state on screen is read, never assumed. No ring plan (the contract
	// has no ring orchestration), no timed or maintenance-window dispatch of a
	// SET (DispatchActionSetRequest carries neither field) — see assign-data.ts
	// for the full RPC mapping.
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
	import type { ActionSet, ActionSetMember } from '$sdk/powermanage/v1/control_pb';
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

	/** Stable context identity — also the stashed draft's identity. */
	const CONTEXT_ID = 'assign';
	/** This surface's home. A stashed draft restores by navigating here. */
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
	/** Parked on the stage — the pill must NOT re-enter this context. */
	let parked = $state(false);
	/** Cancelled or committed — likewise, and the page is on its way out. */
	let closed = $state(false);

	// ── rule mode ────────────────────────────────────────────────────────────
	/** The bindable the chip editor owns; it always holds the compiled string. */
	let ruleQuery = $state('');
	let groupName = $state('');
	let ruleState = $state<QueryEditorState>({
		text: '',
		complete: false,
		valid: null,
		count: null,
		error: '',
		validating: false
	});
	/** The standing-rule acknowledgement. While it is open the pill steps aside,
	 *  so the commit cannot be re-triggered behind the dialog. */
	let confirmOpen = $state(false);
	/** A dynamic group already created from this draft (Save as group, or a
	 *  commit whose assignment step failed). */
	let savedGroup = $state<RuleGroup | null>(null);
	/** Exactly what `savedGroup` was created from. The group is only reused while
	 *  the draft still says the same thing — a group already stores its own rule,
	 *  so a changed draft is a different group, not an edit of that one. */
	let savedFor = $state<{ name: string; query: string } | null>(null);
	let savingGroup = $state(false);
	let ruleError = $state<string | null>(null);

	const eligibility = $derived(computeEligibility(devices, assignedIds));
	const groups = $derived(hostnameGroups(devices));
	const chosenSet = $derived(sets.find((s) => s.id === setId) ?? null);

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

	/** Countable = the server has actually answered for THIS query. A rule that
	 *  has not been counted cannot claim an "Assign to N".
	 *
	 *  The EMPTY rule is excluded on purpose: the builder counts it now (it is
	 *  the legal match-all rule, and group pages show its fleet-wide count), but
	 *  an assignment against every device the org will ever enroll is not a
	 *  target this page arms from an untouched builder — the blast-radius gate
	 *  the empty-uncounted short-circuit used to provide, now stated. */
	const ruleCountable = $derived(
		ruleState.complete &&
			ruleState.valid !== false &&
			ruleState.count !== null &&
			ruleState.text.trim().length > 0
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

	/** B3's single source of truth: the compiled query and the live match count,
	 *  in the pill's caption. The card never repeats the query — the builder's own
	 *  status line is off — and the count appears there only as the preview's
	 *  content, because no RPC can list the rows it stands for. What blocks the
	 *  commit is stated in front of it rather than left to be guessed. */
	const ruleCaption = $derived.by((): { text: string; tone: 'neutral' | 'warn' } => {
		const query = ruleState.text;
		if (!ruleState.complete) return { text: m.query_incomplete(), tone: 'warn' };
		if (ruleState.validating) return { text: `${m.query_counting()} · ${query}`, tone: 'neutral' };
		if (ruleState.valid === false) return { text: `${ruleState.error} · ${query}`, tone: 'warn' };
		// The empty rule is counted by the builder now, but on THIS page it stays
		// a warned, un-armable state (see ruleCountable) — the caption keeps
		// saying so instead of advertising a fleet-wide "Assign to N".
		if (!query) return { text: m.query_future_scope_empty_query(), tone: 'warn' };
		if (ruleState.count === null) {
			return { text: query, tone: 'neutral' };
		}
		const body = `${m.query_match_count_devices({ count: ruleState.count })} · ${query}`;
		if (setId === null) return { text: `${m.assign_caption_choose()} · ${body}`, tone: 'warn' };
		if (!groupName.trim()) return { text: `${m.assign_rule_name_required()} · ${body}`, tone: 'warn' };
		return { text: body, tone: 'neutral' };
	});

	/** Bumped by Retry so the set-dependent reads re-run for the same set id. */
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
		// Take back whatever this surface parked on the stage. The choices
		// themselves live in module state (see draft.svelte.ts), so the claim only
		// has to clear the card — but it MUST run, or a cross-route restore would
		// navigate here and leave an orphaned card behind.
		claimDraft(CONTEXT_ID);
		// A restored draft re-enters with the same target mode AND the same
		// choices — including the rule and the group name, which ARE the target.
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

	// Status for exactly the carried devices — bounded fan-out, see assign-data.
	// A rule target does not read them: its devices are the server's answer to
	// the query, not this selection.
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

	// The chosen set drives two reads: its steps, and which carried devices
	// already carry it. Both re-run when the choice changes and both are dropped
	// when it changes again mid-flight. The second one is carried-mode only —
	// "who already has this set" is a question about a selection.
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
					// An unreadable assignment list is NOT "nobody has this set" —
					// swallowing it would silently understate the update-in-place row,
					// so the failure is surfaced and the previous answer is kept.
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
			// Partial failure keeps the carried selection and the choices, so the
			// operator can retry exactly the devices that did not land.
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
		void goto(`${base}/executions`);
	}

	// ── rule commit ──────────────────────────────────────────────────────────
	// The pill's commit only OPENS the acknowledgement. Nothing is created until
	// the operator confirms that the rule keeps applying — the banner states the
	// consequence, this dialog is where it is accepted.
	function openRuleConfirm() {
		if (!ruleReady) return;
		confirmOpen = true;
	}

	function cancelRuleConfirm() {
		// The pill re-enters with the same draft: closing the dialog is the only
		// state change, and the context effect owns re-entry.
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
			// The group is real and stays named on screen: a retry assigns THAT
			// group instead of creating a second one for the same rule.
			committing = false;
			ruleError = `${m.assign_rule_group_kept({ name: group.name })} ${getLocalizedError(error)}`;
			toast.error(m.assign_rule_failed());
			return;
		}

		committing = false;
		closed = true;
		clearAssignDraft();
		toast.success(m.assign_rule_commit_success({ name: group.name }));
		// The standing rule itself is the result — the group page is where its
		// membership and its assignments are.
		void goto(`${base}/device-groups/${group.id}`);
	}

	/** The secondary pill action: save the rule as a group WITHOUT assigning. */
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

	/** In-place resume only — the store calls this when the operator restores
	 *  while /assign is still the mounted surface. A cross-route restore never
	 *  gets here: the chrome navigates and onMount claims the card instead. */
	function restore() {
		parked = false;
	}

	function ruleSnapshot(): Omit<ContextState, 'id'> {
		return {
			route: ROUTE,
			title: m.assign_rule_pill_title(),
			dirty: ruleState.text.length > 0 || groupName.length > 0 || setId !== null,
			// A standing rule needs all three: a set to apply, a counted rule to
			// apply it to, and a name for the group that carries it. The guard is
			// the store's, so ⌘S is closed too, not only the button.
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
			// The commit is impossible without a set — the guard is the store's,
			// so ⌘S is closed too, not only the button.
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
		// updateContext patches the object a stashed stage card closed over, so a
		// restored draft keeps working; enterContext replaces the whole state.
		if (held) updateContext(next);
		else enterContext({ id: CONTEXT_ID, ...next });
	});

	// Never leak a stale pill context: leaving without commit or stash drops it.
	$effect(() => () => {
		// auto-stash-on-navigate: park a dirty assignment instead of discarding it.
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

<!-- The future-scope acknowledgement: a real confirm on first apply, naming the
     standing-rule consequence AND the group the commit will create. -->
<FutureScopeDialog
	bind:open={confirmOpen}
	queryText={ruleState.text}
	count={ruleState.count}
	kind="device"
	note={m.assign_rule_confirm_group({ name: groupName.trim() })}
	onconfirm={() => void confirmRuleCommit()}
	oncancel={cancelRuleConfirm}
/>
