<script lang="ts">
	// /actions/new — two steps, one pill-committed surface.
	//
	//   1. the type wall: a searchable tile grid derived from ACTION_REGISTRY;
	//   2. configure: name/description + the type's REAL params form on a plate.
	//
	// The route carries no Save button of its own: B1's rule is that the commit
	// lives in the context pill, which also owns Cancel (esc) and Stash (⤓).
	//
	// The shared `action-create-form.svelte` is untouched and still serves the
	// dialogs that embed it — this surface is the full-page one, and it owns a
	// DIFFERENT context id so a dialog opening mid-draft cannot claim this
	// route's parked card.
	import { toast } from 'svelte-sonner';
	import { goto } from '$lib/navigation';
	import { apiClient, useDraft } from '$lib/sdk';
	import { ActionType } from '$sdk/powermanage/v1/actions_pb';
	import * as m from '$lib/paraglide/messages';
	import { getLocalizedError } from '$lib/errors';
	import { actionBasicSchema } from '$lib/forms/schemas/actions';
	import { ACTION_REGISTRY } from '$lib/components/actions/registry';
	import { getActionTypeEnum, getActionTypeInfoByValue } from '$lib/components/actions/action-type';
	import { scheduleFormToProto } from '$lib/components/actions/forms/types';
	import {
		setUserLoaders,
		apiUserLoaders
	} from '$lib/components/actions/forms/user-loader-context.svelte';
	import { bindBuilderContext } from '$lib/components/actions/pipeline/builder-pill.svelte';
	import TypeChooser from './type-chooser.svelte';
	import ConfigurePanel from './configure-panel.svelte';
	import { formKeyForTypeValue, COMPLIANCE_KEY } from './type-tiles';
	import { draftForType, emptyDraft, hydrate, type ActionDraft } from './draft';

	/** Distinct from action-create-form's 'action:new' on purpose — see above. */
	const CONTEXT_ID = 'action:create';
	const ROUTE = '/actions/new';

	// Nested UserPicker / GroupParamsForm resolve their loaders from context, the
	// same way the create form and the pipeline builders wire them.
	setUserLoaders(apiUserLoaders);

	// Autosave. The DraftType union is owned by the SDK and cannot be extended
	// from the web app, so this surface namespaces itself inside the
	// 'create-definition' bucket by id, exactly as the pipeline builders do.
	const persist = useDraft<ActionDraft>('create-definition', CONTEXT_ID, emptyDraft());

	// Two ways back into an unfinished action, in precedence order:
	//   - a STASHED card, claimed here on mount. It is the newer of the two (the
	//     operator parked deliberately) and it is the only one that survives a
	//     restore from another route;
	//   - the autosave, which additionally survives a full page reload.
	// `bindBuilderContext` performs the claim, so the pill binding and the claim
	// can never drift apart.
	// svelte-ignore state_referenced_locally
	const claimed = bindBuilderContext(CONTEXT_ID, () => snapshot());
	// svelte-ignore state_referenced_locally
	let draft = $state<ActionDraft>(hydrate(claimed) ?? hydrate(persist.data) ?? emptyDraft());

	/** A create surface opens EMPTY: there is nothing to save and nothing worth
	 *  parking. Saying `dirty: true` regardless is what made an untouched form
	 *  offer Save, and auto-stash itself onto the stage on the way out. */
	const PRISTINE = JSON.stringify(emptyDraft());
	const isDirty = () => JSON.stringify($state.snapshot(draft)) !== PRISTINE;

	let saving = $state(false);
	/** Parked on the stage — the pill must NOT re-enter this context. */
	let parked = $state(false);
	let scheduleOpen = $state(false);
	/** Which step is on screen. Separate from `draft.typeValue` on purpose: the
	 *  back link is a step change, not a reset, so walking back to the wall and
	 *  re-picking the same type must return the operator to their own buffer.
	 *  A draft that already carries a type — claimed from the stage or restored
	 *  from the autosave after a reload — lands straight on step 2. */
	// svelte-ignore state_referenced_locally
	let atWall = $state(draft.typeValue === null);

	const typeValue = $derived(draft.typeValue);
	const formKey = $derived(typeValue ? formKeyForTypeValue(typeValue) : null);
	const typeLabel = $derived(typeValue ? getActionTypeInfoByValue(typeValue).label : '');

	// Live validation against exactly the two schemas the commit submits against,
	// so ⌘S is closed before it is pressed. Pure derivation — a field stops being
	// an error with the keystroke that fixes it.
	const errors = $derived.by(() => {
		const out: Record<string, string> = {};
		if (!formKey) return out;
		const collect = (result: { success: boolean; error?: { issues: { path: PropertyKey[]; message: string }[] } }) => {
			if (result.success || !result.error) return;
			for (const issue of result.error.issues) {
				const field = issue.path.length ? String(issue.path[0]) : '_';
				if (!(field in out)) out[field] = issue.message;
			}
		};
		collect(
			actionBasicSchema.safeParse({
				name: draft.name.trim(),
				description: draft.description.trim(),
				timeoutSeconds: draft.timeoutSeconds
			})
		);
		collect(ACTION_REGISTRY[formKey].schema.safeParse(draft.params));
		return out;
	});

	const firstError = $derived(Object.values(errors)[0] ?? null);

	// Autosave. $state.snapshot reads the buffer deeply, so this re-runs on any
	// nested params edit AND hands the draft store a structured-cloneable plain
	// object (an IndexedDB put cannot clone a $state proxy).
	$effect(() => {
		persist.data = $state.snapshot(draft) as ActionDraft;
	});

	function snapshot() {
		// Step 1 has nothing to commit, so the pill stays in nav there.
		if (atWall || !typeValue || !formKey || saving || parked) return null;
		return {
			// Stash's home. Restoring from any other page navigates back here and
			// the remount claims the payload below.
			route: ROUTE,
			title: draft.name.trim() || typeLabel,
			dirty: isDirty(),
			valid: firstError === null,
			commitLabel: m.common_create(),
			// The action's own settings hang off the pill, exactly as they do on an
			// existing action — the schedule is not one more section to scroll past.
			extraActions: [
				{
					id: 'schedule',
					label: m.container_schedule_title(),
					onRun: () => (scheduleOpen = true)
				}
			],
			subtext: firstError
				? `${m.actions_create_incomplete()} · ${firstError}`
				: m.actions_new_ready({ type: typeLabel }),
			subtextTone: firstError ? ('warn' as const) : ('neutral' as const),
			stashSubtitle: m.actions_new_stash_subtitle({ type: typeLabel }),
			onCommit: () => void commit(),
			onCancel: cancel,
			onStash: () => (parked = true),
			onRestore: () => (parked = false),
			// The buffer rides ON the card too, not only in the autosave: a claim
			// after a cross-route restore must not depend on the debounced write
			// having landed.
			stashPayload: () => $state.snapshot(draft)
		};
	}

	function choose(next: string) {
		// Re-picking the SAME type keeps the buffer; a different type gets that
		// type's defaults, because the params bucket is not transferable.
		if (draft.typeValue !== next) draft = draftForType(next);
		atWall = false;
	}

	function back() {
		atWall = true;
	}

	function cancel() {
		void persist.clear();
		void goto('/actions');
	}

	async function commit() {
		const value = typeValue;
		const key = formKey;
		if (!value || !key || firstError) return;
		saving = true;
		try {
			const adapter = ACTION_REGISTRY[key];
			// APP_IMAGE / DEB / RPM share the APP adapter, so the tile's own type
			// wins; COMPLIANCE_CHECK has no proto enum of its own and falls back to
			// the adapter's (SHELL), which is exactly what it is.
			const fromValue = getActionTypeEnum(value);
			const type = fromValue === ActionType.UNSPECIFIED ? adapter.actionType : fromValue;
			const params = $state.snapshot(draft.params);
			if (value === COMPLIANCE_KEY) (params as Record<string, unknown>).isCompliance = true;

			const action = await apiClient.createAction({
				name: draft.name.trim(),
				description: draft.description.trim(),
				type,
				desiredState: adapter.supportsAbsent ? draft.desiredState : 0,
				timeoutSeconds: draft.timeoutSeconds,
				schedule: scheduleFormToProto($state.snapshot(draft.schedule)),
				params: {
					case: adapter.paramsCase,
					value: adapter.formToProto(params)
				} as Parameters<typeof apiClient.createAction>[0]['params']
			});

			if (action) {
				await persist.clear();
				toast.success(m.actions_created());
				void goto(`/actions/${action.id}`);
			}
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head><title>{m.actions_create()}</title></svelte:head>

<div class="flex-1 overflow-auto p-4 md:p-6">
	{#if !atWall && typeValue && formKey}
		<ConfigurePanel bind:draft bind:scheduleOpen {typeValue} {formKey} {errors} onback={back} />
	{:else}
		<TypeChooser onchoose={choose} />
	{/if}
</div>
