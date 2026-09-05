<script lang="ts">

	import { toast } from 'svelte-sonner';
	import { goto } from '$lib/navigation';
	import { api } from '$lib/api';
	import PermissionMatrix from '../[id]/permission-matrix.svelte';
 import { availablePermissions } from '../[id]/permission-groups';
 import { Permission } from '$contract/cadestro/v1/control_pb';
	import { consoleContext } from '$lib/console-context.svelte';
	const { can } = consoleContext();
	import { bindBuilderContext } from '$lib/components/actions/pipeline/builder-pill.svelte';
	import CreatePlate from '$lib/components/create/create-plate.svelte';
	import IdentityRow from '$lib/components/create/identity-row.svelte';
	import { Shield } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { getLocalizedError } from '$lib/errors';

	const CONTEXT_ID = 'role:create';
	const ROUTE = '/roles/new';

	type RoleDraft = { name: string; description: string; permissions: Permission[] };

	function hydrate(raw: unknown): RoleDraft | null {
		if (!raw || typeof raw !== 'object') return null;
		const d = raw as Partial<RoleDraft>;
		return {
			name: typeof d.name === 'string' ? d.name : '',
			description: typeof d.description === 'string' ? d.description : '',
 permissions: Array.isArray(d.permissions) ? d.permissions.filter(value => availablePermissions.includes(value)) : []
		};
	}

	const claimed = bindBuilderContext(CONTEXT_ID, () => snapshot());

	let draft = $state<RoleDraft>(hydrate(claimed) ?? { name: '', description: '', permissions: [] });

	const PRISTINE = JSON.stringify({ name: '', description: '', permissions: draft.permissions });
	const isDirty = () => JSON.stringify($state.snapshot(draft)) !== PRISTINE;

	let saving = $state(false);

	let parked = $state(false);

	const errors = $derived<Record<string, string>>(draft.name.trim() ? {} : { name: m.validation_name_required() });
	const firstError = $derived(Object.values(errors)[0] ?? null);

	function snapshot() {
		if (saving || parked) return null;
		return {
			route: ROUTE,
			title: draft.name.trim() || m.roles_create(),
			dirty: isDirty(),
			valid: firstError === null && can(Permission.CREATE_ROLE),
			commitLabel: m.common_create(),
			subtext: firstError ?? m.common_create_ready(),
			subtextTone: firstError ? ('warn' as const) : ('neutral' as const),
			stashSubtitle: m.common_create_stash_subtitle({
				entity: draft.name.trim() || m.roles_create()
			}),
			onCommit: () => void commit(),
			onCancel: () => void goto('/roles'),
			onStash: () => (parked = true),
			onRestore: () => (parked = false),
			stashPayload: () => $state.snapshot(draft)
		};
	}

	async function commit() {
		if (firstError || !can(Permission.CREATE_ROLE)) return;
		saving = true;
		try {
			const { role } = await api.createRole({ name: draft.name.trim(), description: draft.description.trim(), permissions: draft.permissions });
			if (role) {
				toast.success(m.roles_created());
				void goto(can(Permission.GET_ROLE) ? `/roles/${role.id?.value ?? ''}` : '/roles');
			}
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head><title>{m.roles_create()}</title></svelte:head>

<div class="flex-1 overflow-auto p-4 md:p-6">
	<CreatePlate
		icon={Shield}
		title={m.roles_create()}
		description={m.roles_create_description()}
		testid="role-create"
	>
		<IdentityRow
			idPrefix="role"
			nameLabel={m.roles_name()}
			namePlaceholder={m.roles_name_placeholder()}
			bind:name={draft.name}
			nameError={errors.name}
			descriptionLabel={m.roles_description_field()}
			descriptionPlaceholder={m.roles_description_placeholder()}
			bind:description={draft.description}
		/>
        <PermissionMatrix permissions={availablePermissions} selected={new Set(draft.permissions)} columnLabel={m.roles_name()} disabled={!can(Permission.CREATE_ROLE)} onToggle={(permission) => { draft.permissions = draft.permissions.includes(permission) ? draft.permissions.filter(value => value !== permission) : [...draft.permissions, permission]; }} onToggleGroup={(permissions, allSelected) => { draft.permissions = allSelected ? draft.permissions.filter(value => !permissions.includes(value)) : [...new Set([...draft.permissions, ...permissions])]; }} />
	</CreatePlate>
</div>
