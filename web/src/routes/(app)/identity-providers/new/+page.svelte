<script lang="ts">

	import { emptyDraft, hydrate, draftErrors, type IdpDraft } from './draft';
	import { toast } from 'svelte-sonner';
	import { goto } from '$lib/navigation';
	import { api } from '$lib/api';
 import { Permission } from '$contract/cadestro/v1/control_pb';
 import { consoleContext } from '$lib/console-context.svelte';
 const { can } = consoleContext();
	import { bindBuilderContext } from '$lib/components/actions/pipeline/builder-pill.svelte';
	import CreatePlate from '$lib/components/create/create-plate.svelte';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import * as Select from '$lib/components/ui/select';
	import { FieldError } from '$lib/components/ui/field-error';
	import { Fingerprint } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { getLocalizedError } from '$lib/errors';

	const CONTEXT_ID = 'identity-provider:create';
	const ROUTE = '/identity-providers/new';


	const claimed = bindBuilderContext(CONTEXT_ID, () => snapshot());

	let draft = $state<IdpDraft>(hydrate(claimed) ?? emptyDraft());

	const PRISTINE = JSON.stringify(emptyDraft());
	const isDirty = () => JSON.stringify($state.snapshot(draft)) !== PRISTINE;

	let saving = $state(false);

	let parked = $state(false);

	const errors = $derived(draftErrors(draft));
 const firstError = $derived(Object.values(errors)[0] ?? null);

	function snapshot() {
		if (saving || parked) return null;
		return {
			route: ROUTE,
			title: draft.name.trim() || m.idp_create_title(),
			dirty: isDirty(),
			valid: firstError === null && can(Permission.CREATE_IDENTITY_PROVIDER),
			commitLabel: m.common_create(),
			subtext: firstError ?? m.common_create_ready(),
			subtextTone: firstError ? ('warn' as const) : ('neutral' as const),
			stashSubtitle: m.common_create_stash_subtitle({
				entity: draft.name.trim() || m.idp_create_title()
			}),
			onCommit: () => void commit(),
			onCancel: () => void goto('/identity-providers'),
			onStash: () => (parked = true),
			onRestore: () => (parked = false),
			stashPayload: () => $state.snapshot(draft)
		};
	}

	async function commit() {
		if (firstError || !can(Permission.CREATE_IDENTITY_PROVIDER)) return;
		saving = true;
		try {
			const scopes = draft.scopes
				.split(',')
				.map((s) => s.trim())
				.filter(Boolean);
			const { provider } = await api.createIdentityProvider({
				name: draft.name.trim(),
				slug: draft.slug.trim(),
				clientId: { value: draft.clientId.trim() },
				issuerUrl: draft.issuerUrl.trim(),
				scopes: scopes.length > 0 ? scopes : ['openid', 'profile', 'email'],
			});
			if (provider) {
				toast.success(m.idp_create_success());
				void goto((can(Permission.GET_IDENTITY_PROVIDER) || can(Permission.LIST_IDENTITY_PROVIDERS)) ? `/identity-providers/${provider.id?.value ?? ''}` : '/identity-providers');
			}
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head><title>{m.idp_create_title()}</title></svelte:head>

<div class="flex-1 overflow-auto p-4 md:p-6">
	<CreatePlate
		icon={Fingerprint}
		title={m.idp_create_title()}
		description={m.idp_create_description()}
		testid="idp-create"
	>

		<div class="grid gap-3 sm:grid-cols-2">
			<div class="space-y-1.5">
				<Label for="idpName">{m.idp_field_name()}</Label>
				<Input id="idpName" bind:value={draft.name} aria-invalid={!!errors.name} />
				<FieldError error={errors.name} />
			</div>
			<div class="space-y-1.5">
				<Label for="idpSlug">{m.idp_field_slug()}</Label>
				<Input id="idpSlug" bind:value={draft.slug} aria-invalid={!!errors.slug} />
				<FieldError error={errors.slug} />
				<p class="text-xs text-muted-foreground">{m.idp_field_slug_help()}</p>
			</div>
		</div>
		<div class="grid gap-3 sm:grid-cols-2">
			<div class="space-y-1.5">
				<Label for="idpClientId">{m.idp_field_client_id()}</Label>
				<Input id="idpClientId" bind:value={draft.clientId} aria-invalid={!!errors.clientId} />
				<FieldError error={errors.clientId} />
			</div>

		</div>
		<div class="space-y-1.5">
			<Label for="idpIssuerUrl">{m.idp_field_issuer_url()}</Label>
			<Input
				id="idpIssuerUrl"
				type="url"
				placeholder="https://accounts.google.com"
				bind:value={draft.issuerUrl}
				aria-invalid={!!errors.issuerUrl}
			/>
			<FieldError error={errors.issuerUrl} />
			<p class="text-xs text-muted-foreground">{m.idp_field_issuer_url_help()}</p>
		</div>
		<div class="space-y-1.5">
			<Label for="idpScopes">{m.idp_field_scopes()}</Label>
			<Input id="idpScopes" bind:value={draft.scopes} placeholder="openid, profile, email" />
			<p class="text-xs text-muted-foreground">{m.idp_field_scopes_help()}</p>
		</div>

	</CreatePlate>
</div>
