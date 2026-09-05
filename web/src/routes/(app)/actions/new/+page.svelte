<script lang="ts">
	import { untrack } from 'svelte';
	import { toast } from 'svelte-sonner';
	import { goto } from '$lib/navigation';
	import { api } from '$lib/api';
 import { Permission } from '$contract/cadestro/v1/control_pb';
 import { consoleContext } from '$lib/console-context.svelte';
 import * as m from '$lib/paraglide/messages';
 import { getLocalizedError } from '$lib/errors';
 import { getActionTypeInfoByValue } from '$lib/components/actions/action-type';
 import { bindBuilderContext } from '$lib/components/actions/pipeline/builder-pill.svelte';
 import TypeChooser from './type-chooser.svelte';
 import ConfigurePanel from './configure-panel.svelte';
 import { draftForType, emptyDraft, hydrate, serialize, draftErrors, type ActionDraft } from './draft';
 const { can } = consoleContext();

	const CONTEXT_ID = 'action:create';
	const ROUTE = '/actions/new';

	const claimed = bindBuilderContext(CONTEXT_ID, () => snapshot());

	let draft = $state<ActionDraft>(hydrate(claimed) ?? emptyDraft());

	const PRISTINE = serialize(emptyDraft());
	const isDirty = () => serialize($state.snapshot(draft)) !== PRISTINE;

	let saving = $state(false);

	let parked = $state(false);
	let scheduleOpen = $state(false);

	let atWall = $state(untrack(() => !draft.params.case));

	const typeValue = $derived(draft.params.case === 'shell' && draft.params.value.isCompliance ? 'COMPLIANCE_CHECK' : draft.params.case?.toUpperCase() ?? '');
 const typeLabel = $derived(typeValue ? getActionTypeInfoByValue(typeValue).label : '');
 const errors = $derived(draftErrors(draft));
 const firstError = $derived(Object.values(errors)[0] ?? null);

	function snapshot() {

		if (atWall || !typeValue || saving || parked) return null;
		return {

			route: ROUTE,
			title: draft.name.trim() || typeLabel,
			dirty: isDirty(),
			valid: firstError === null && can(Permission.CREATE_ACTION),
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

			stashPayload: () => serialize($state.snapshot(draft))
		};
	}

	function choose(next: string) {

		if (typeValue !== next) draft = draftForType(next);
		atWall = false;
	}

	function back() {
		atWall = true;
	}

	function cancel() {
		void goto('/actions');
	}

	async function commit() {
 if (firstError || !can(Permission.CREATE_ACTION)) return;
 saving = true;
 try {
  const { action } = await api.createAction($state.snapshot(draft));
  toast.success(m.actions_created());
  await goto(action && can(Permission.GET_ACTION) ? `/actions/${action.id?.value ?? ''}` : '/actions');
 } catch (error) { toast.error(getLocalizedError(error)); } finally { saving = false; }
 }

</script>

<svelte:head><title>{m.actions_create()}</title></svelte:head>

<div class="flex-1 overflow-auto p-4 md:p-6">
	{#if !atWall && typeValue}
		<ConfigurePanel bind:draft bind:scheduleOpen {typeValue} {errors} onback={back} />
	{:else}
		<TypeChooser onchoose={choose} />
	{/if}
</div>
