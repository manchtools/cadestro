<script lang="ts">

	import { toast } from 'svelte-sonner';
	import { goto } from '$lib/navigation';
	import { create } from '@bufbuild/protobuf';
	import { apiClient, persistDraft, useDraft } from '$lib/sdk';
	import { ActionScheduleSchema } from '$contract/cadestro/v1/actions_pb';
	import { nameDescriptionSchema } from '$lib/forms/schemas/common';
	import { bindBuilderContext } from '$lib/components/actions/pipeline/builder-pill.svelte';
	import CreatePlate from '$lib/components/create/create-plate.svelte';
	import IdentityRow from '$lib/components/create/identity-row.svelte';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import { FieldError } from '$lib/components/ui/field-error';
	import { FolderTree } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { getLocalizedError } from '$lib/errors';

	const CONTEXT_ID = 'definition:create';
	const ROUTE = '/definitions/new';

	type DefinitionDraft = { name: string; description: string };

	function emptyDraft(): DefinitionDraft {
		return { name: '', description: '' };
	}

	function hydrate(raw: unknown): DefinitionDraft | null {
		if (!raw || typeof raw !== 'object') return null;
		const d = raw as Partial<DefinitionDraft>;
		return {
			name: typeof d.name === 'string' ? d.name : '',
			description: typeof d.description === 'string' ? d.description : ''
		};
	}

	const persist = useDraft<DefinitionDraft>('create-definition', CONTEXT_ID, emptyDraft());

	const claimed = bindBuilderContext(CONTEXT_ID, () => snapshot());

	let draft = $state<DefinitionDraft>(hydrate(claimed) ?? hydrate(persist.data) ?? emptyDraft());

	const PRISTINE = JSON.stringify(emptyDraft());
	const isDirty = () => JSON.stringify($state.snapshot(draft)) !== PRISTINE;

	let saving = $state(false);

	let parked = $state(false);

	persistDraft(persist, () => draft);

	const errors = $derived.by(() => {
		const out: Record<string, string> = {};
		const result = nameDescriptionSchema.safeParse({
			name: draft.name.trim(),
			description: draft.description.trim()
		});
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
			title: draft.name.trim() || m.definitions_create(),
			dirty: isDirty(),
			valid: firstError === null,
			commitLabel: m.common_create(),
			subtext: firstError ?? m.common_create_ready(),
			subtextTone: firstError ? ('warn' as const) : ('neutral' as const),
			stashSubtitle: m.common_create_stash_subtitle({
				entity: draft.name.trim() || m.definitions_create()
			}),
			onCommit: () => void commit(),
			onCancel: cancel,
			onStash: () => (parked = true),
			onRestore: () => (parked = false),
			stashPayload: () => $state.snapshot(draft)
		};
	}

	function cancel() {
		void persist.clear();
		void goto('/definitions');
	}

	async function commit() {
		if (firstError) return;
		saving = true;
		try {
			const def = await apiClient.createDefinition({
				name: draft.name.trim(),
				description: draft.description.trim(),

				schedule: create(ActionScheduleSchema, { intervalHours: 8 })
			});
			if (def) {
				await persist.clear();
				toast.success(m.definitions_created());
				void goto(`/definitions/${def.id?.value ?? ''}`);
			}
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head><title>{m.definitions_create_dialog_title()}</title></svelte:head>

<div class="flex-1 overflow-auto p-4 md:p-6">
	<CreatePlate
		icon={FolderTree}
		title={m.definitions_create_dialog_title()}
		description={m.definitions_create_dialog_description()}
		testid="definition-create"
	>
		<IdentityRow
			idPrefix="definition"
			nameLabel={m.common_name()}
			namePlaceholder={m.definitions_name_placeholder()}
			bind:name={draft.name}
			nameError={errors.name}
			descriptionLabel={m.common_description()}
			descriptionPlaceholder={m.definitions_desc_placeholder()}
			bind:description={draft.description}
		/>
	</CreatePlate>
</div>
