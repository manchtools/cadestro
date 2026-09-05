<script lang="ts">

	import { toast } from 'svelte-sonner';
	import { goto } from '$lib/navigation';
	import { api } from '$lib/api';
 import { Permission } from '$contract/cadestro/v1/control_pb';
 import { consoleContext } from '$lib/console-context.svelte';
 const { can } = consoleContext();
	import { bindBuilderContext } from '$lib/components/actions/pipeline/builder-pill.svelte';
	import CreatePlate from '$lib/components/create/create-plate.svelte';
	import IdentityRow from '$lib/components/create/identity-row.svelte';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import { Switch } from '$lib/components/ui/switch';
	import { FieldError } from '$lib/components/ui/field-error';
	import { Users, Zap, TriangleAlert } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { getLocalizedError } from '$lib/errors';

	const CONTEXT_ID = 'device-group:create';
	const ROUTE = '/device-groups/new';

	type GroupDraft = {
		name: string;
		description: string;
	};

	function emptyDraft(): GroupDraft {
		return { name: '', description: '' };
	}

	function hydrate(raw: unknown): GroupDraft | null {
		if (!raw || typeof raw !== 'object') return null;
		const d = raw as Partial<GroupDraft>;
		const base = emptyDraft();
		return {
			name: typeof d.name === 'string' ? d.name : base.name,
			description: typeof d.description === 'string' ? d.description : base.description,
		};
	}

	const claimed = bindBuilderContext(CONTEXT_ID, () => snapshot());

	let draft = $state<GroupDraft>(hydrate(claimed) ?? emptyDraft());

	const PRISTINE = JSON.stringify(emptyDraft());
	const isDirty = () => JSON.stringify($state.snapshot(draft)) !== PRISTINE;

	let saving = $state(false);

	let parked = $state(false);
	const errors = $derived<Record<string, string>>(draft.name.trim() ? {} : { name: 'Enter a group name' });
	const firstError = $derived(Object.values(errors)[0] ?? null);

	function snapshot() {
		if (saving || parked) return null;
		return {
			route: ROUTE,
			title: draft.name.trim() || m.device_groups_create(),
			dirty: isDirty(),
			valid: firstError === null && can(Permission.CREATE_DEVICE_GROUP),
			commitLabel: m.common_create(),
			subtext: firstError ?? m.common_create_ready(),
			subtextTone: firstError ? ('warn' as const) : ('neutral' as const),
			stashSubtitle: m.common_create_stash_subtitle({
				entity: draft.name.trim() || m.device_groups_create()
			}),
			onCommit: () => void commit(),
			onCancel: () => void goto('/device-groups'),
			onStash: () => (parked = true),
			onRestore: () => (parked = false),
			stashPayload: () => $state.snapshot(draft)
		};
	}

	async function commit() {
		if (firstError || !can(Permission.CREATE_DEVICE_GROUP)) return;
		saving = true;
		try {
			const { group } = await api.createDeviceGroup({ name: draft.name.trim(), description: draft.description.trim() });
			if (group) {
				toast.success(m.device_groups_created());
				void goto((can(Permission.GET_DEVICE_GROUP) || can(Permission.LIST_DEVICE_GROUPS)) ? `/device-groups/${group.id?.value ?? ''}` : '/device-groups');
			}
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head><title>{m.device_groups_create_dialog_title()}</title></svelte:head>

<div class="flex-1 overflow-auto p-4 md:p-6">
	<CreatePlate
		icon={Users}
		title={m.device_groups_create_dialog_title()}
		description={m.device_groups_create_dialog_description()}
		testid="device-group-create"
	>
		<IdentityRow
			idPrefix="group"
			nameLabel={m.common_name()}
			namePlaceholder={m.device_groups_name_placeholder()}
			bind:name={draft.name}
			nameError={errors.name}
			descriptionLabel={m.common_description()}
			descriptionPlaceholder={m.device_groups_desc_placeholder()}
			bind:description={draft.description}
		/>
	</CreatePlate>
</div>
