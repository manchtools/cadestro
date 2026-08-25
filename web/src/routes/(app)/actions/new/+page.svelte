<script lang="ts">
	import { untrack } from 'svelte';
	import { toast } from 'svelte-sonner';
	import { goto } from '$lib/navigation';
	import { apiClient, useDraft } from '$lib/sdk';
	import { ActionType } from '$contract/cadestro/v1/actions_pb';
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

	const CONTEXT_ID = 'action:create';
	const ROUTE = '/actions/new';

	setUserLoaders(apiUserLoaders);

	const persist = useDraft<ActionDraft>('create-definition', CONTEXT_ID, emptyDraft());

	const claimed = bindBuilderContext(CONTEXT_ID, () => snapshot());

	let draft = $state<ActionDraft>(hydrate(claimed) ?? hydrate(persist.data) ?? emptyDraft());

	const PRISTINE = JSON.stringify(emptyDraft());
	const isDirty = () => JSON.stringify($state.snapshot(draft)) !== PRISTINE;

	let saving = $state(false);

	let parked = $state(false);
	let scheduleOpen = $state(false);

	let atWall = $state(untrack(() => draft.typeValue === null));

	const typeValue = $derived(draft.typeValue);
	const formKey = $derived(typeValue ? formKeyForTypeValue(typeValue) : null);
	const typeLabel = $derived(typeValue ? getActionTypeInfoByValue(typeValue).label : '');

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

	$effect(() => {
		persist.data = $state.snapshot(draft) as ActionDraft;
	});

	function snapshot() {

		if (atWall || !typeValue || !formKey || saving || parked) return null;
		return {

			route: ROUTE,
			title: draft.name.trim() || typeLabel,
			dirty: isDirty(),
			valid: firstError === null,
			commitLabel: m.common_create(),

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

			stashPayload: () => $state.snapshot(draft)
		};
	}

	function choose(next: string) {

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
