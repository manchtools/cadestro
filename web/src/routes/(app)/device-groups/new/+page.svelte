<script lang="ts">

	import { toast } from 'svelte-sonner';
	import { goto } from '$lib/navigation';
	import { apiClient } from '$lib/sdk';
	import { createDeviceGroupSchema } from '$lib/forms/schemas/device-groups';
	import { bindBuilderContext } from '$lib/components/actions/pipeline/builder-pill.svelte';
	import CreatePlate from '$lib/components/create/create-plate.svelte';
	import IdentityRow from '$lib/components/create/identity-row.svelte';
	import QueryBuilder, { type QueryEditorState } from '$lib/components/query-builder.svelte';
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
		isDynamic: boolean;
		dynamicQuery: string;
	};

	function emptyDraft(): GroupDraft {
		return { name: '', description: '', isDynamic: false, dynamicQuery: '' };
	}

	function hydrate(raw: unknown): GroupDraft | null {
		if (!raw || typeof raw !== 'object') return null;
		const d = raw as Partial<GroupDraft>;
		const base = emptyDraft();
		return {
			name: typeof d.name === 'string' ? d.name : base.name,
			description: typeof d.description === 'string' ? d.description : base.description,
			isDynamic: typeof d.isDynamic === 'boolean' ? d.isDynamic : base.isDynamic,
			dynamicQuery: typeof d.dynamicQuery === 'string' ? d.dynamicQuery : base.dynamicQuery
		};
	}

	const claimed = bindBuilderContext(CONTEXT_ID, () => snapshot());

	let draft = $state<GroupDraft>(hydrate(claimed) ?? emptyDraft());

	const PRISTINE = JSON.stringify(emptyDraft());
	const isDirty = () => JSON.stringify($state.snapshot(draft)) !== PRISTINE;

	let saving = $state(false);

	let parked = $state(false);
	let queryState = $state<QueryEditorState>({
		text: '',
		valid: false,
		count: null,
		error: m.query_incomplete(),
		validating: false
	});
	const queryUnusable = $derived(draft.isDynamic && queryState.valid !== true);

	const errors = $derived.by(() => {
		const out: Record<string, string> = {};
		const result = createDeviceGroupSchema.safeParse({
			name: draft.name.trim(),
			description: draft.description.trim(),
			isDynamic: draft.isDynamic,
			dynamicQuery: draft.dynamicQuery
		});
		if (!result.success) {
			for (const issue of result.error.issues) {
				const field = issue.path.length ? String(issue.path[0]) : '_';
				if (!(field in out)) out[field] = issue.message;
			}
		}
		return out;
	});
	const firstError = $derived(
		Object.values(errors)[0] ?? (queryUnusable ? m.device_groups_query_fix() : null)
	);

	function snapshot() {
		if (saving || parked) return null;
		return {
			route: ROUTE,
			title: draft.name.trim() || m.device_groups_create(),
			dirty: isDirty(),
			valid: firstError === null,
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
		if (firstError) return;
		saving = true;
		try {
			const group = await apiClient.createDeviceGroup(
				draft.name.trim(),
				draft.description.trim(),
				draft.isDynamic,
				draft.isDynamic ? draft.dynamicQuery.trim() : ''
			);
			if (group) {
				toast.success(m.device_groups_created());
				void goto(`/device-groups/${group.id?.value ?? ''}`);
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
		<div class="flex items-center justify-between rounded-lg border p-4">
			<div class="space-y-0.5">
				<Label for="group-dynamic" class="flex items-center gap-2">
					<Zap class="h-4 w-4" />
					{m.device_groups_dynamic_label()}
				</Label>
				<p class="text-xs text-muted-foreground">{m.device_groups_dynamic_hint()}</p>
			</div>
			<Switch id="group-dynamic" bind:checked={draft.isDynamic} />
		</div>
		{#if draft.isDynamic}
			<div class="space-y-1.5">
				<Label for="dynamicQuery">{m.device_groups_dynamic_query_label()}</Label>
				<QueryBuilder
					bind:query={draft.dynamicQuery}
					kind="device"
					onstate={(s) => (queryState = s)}
				/>
				<FieldError error={errors.dynamicQuery} />
				<p class="flex items-start gap-2 rounded-md bg-warn-soft px-2.5 py-2 text-xs text-warn">
					<TriangleAlert class="mt-0.5 h-3.5 w-3.5 shrink-0" />
					<span>{m.query_futurebar()}</span>
				</p>
			</div>
		{/if}
	</CreatePlate>
</div>
