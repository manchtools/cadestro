<script lang="ts">

	import { toast } from 'svelte-sonner';
	import { goto } from '$lib/navigation';
	import { apiClient } from '$lib/sdk';
	import { z } from 'zod';
	import { bindBuilderContext } from '$lib/components/actions/pipeline/builder-pill.svelte';
	import CreatePlate from '$lib/components/create/create-plate.svelte';
	import IdentityRow from '$lib/components/create/identity-row.svelte';
	import { Shield } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { getLocalizedError } from '$lib/errors';

	const CONTEXT_ID = 'role:create';
	const ROUTE = '/roles/new';

	type RoleDraft = { name: string; description: string };

	function hydrate(raw: unknown): RoleDraft | null {
		if (!raw || typeof raw !== 'object') return null;
		const d = raw as Partial<RoleDraft>;
		return {
			name: typeof d.name === 'string' ? d.name : '',
			description: typeof d.description === 'string' ? d.description : ''
		};
	}

	const claimed = bindBuilderContext(CONTEXT_ID, () => snapshot());

	let draft = $state<RoleDraft>(hydrate(claimed) ?? { name: '', description: '' });

	const PRISTINE = JSON.stringify({ name: '', description: '' });
	const isDirty = () => JSON.stringify($state.snapshot(draft)) !== PRISTINE;

	let saving = $state(false);

	let parked = $state(false);

	const createSchema = z.object({ name: z.string().min(1, m.validation_name_required()) });
	const errors = $derived.by(() => {
		const out: Record<string, string> = {};
		const result = createSchema.safeParse({ name: draft.name.trim() });
		if (!result.success) {
			for (const issue of result.error.issues) {
				const field = issue.path.length ? String(issue.path[0]) : '_';
				if (!(field in out)) out[field] = issue.message;
			}
		}
		return out;
	});
	const firstError = $derived(Object.values(errors)[0] ?? null);

	function snapshot() {
		if (saving || parked) return null;
		return {
			route: ROUTE,
			title: draft.name.trim() || m.roles_create(),
			dirty: isDirty(),
			valid: firstError === null,
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
		if (firstError) return;
		saving = true;
		try {
			const role = await apiClient.createRole(draft.name.trim(), draft.description.trim(), []);
			if (role) {
				toast.success(m.roles_created());
				void goto(`/roles/${role.id?.value ?? ''}`);
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
		hint={m.roles_create_scope_hint()}
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
	</CreatePlate>
</div>
