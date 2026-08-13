<script lang="ts">
	// /identity-providers/new — OIDC provider creation as a pill-committed surface.
	//
	// Six fields, one of them a client secret: this is the longest create form in
	// the app and the one an operator is most likely to abandon halfway to go and
	// fetch a value from the provider's console. As a modal that trip destroyed the
	// form; declaring `route` earns the Stash button so the work parks instead.
	//
	// Deliberately NOT autosaved through `useDraft`: that hook persists to
	// IndexedDB, and a client secret has no business being written to disk. The
	// stash payload lives in the in-memory shell store only, exactly as long as the
	// form field it came from.
	import { onMount } from 'svelte';
	import { toast } from 'svelte-sonner';
	import { goto } from '$lib/navigation';
	import { apiClient, type Role } from '$lib/sdk';
	import { IdentityProviderType } from '$sdk/powermanage/v1/common_pb';
	import { z } from 'zod';
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

	type IdpDraft = {
		name: string;
		slug: string;
		clientId: string;
		clientSecret: string;
		issuerUrl: string;
		scopes: string;
		autoCreateUsers: boolean;
		/** Role granted to JIT-created users; empty string means "no default role". */
		defaultRoleId: string;
	};

	function emptyDraft(): IdpDraft {
		return {
			name: '',
			slug: '',
			clientId: '',
			clientSecret: '',
			issuerUrl: '',
			scopes: '',
			autoCreateUsers: false,
			defaultRoleId: ''
		};
	}

	function hydrate(raw: unknown): IdpDraft | null {
		if (!raw || typeof raw !== 'object') return null;
		const d = raw as Partial<IdpDraft>;
		const base = emptyDraft();
		const str = (v: unknown, fallback: string) => (typeof v === 'string' ? v : fallback);
		const bool = (v: unknown, fallback: boolean) => (typeof v === 'boolean' ? v : fallback);
		return {
			name: str(d.name, base.name),
			slug: str(d.slug, base.slug),
			clientId: str(d.clientId, base.clientId),
			clientSecret: str(d.clientSecret, base.clientSecret),
			issuerUrl: str(d.issuerUrl, base.issuerUrl),
			scopes: str(d.scopes, base.scopes),
			autoCreateUsers: bool(d.autoCreateUsers, base.autoCreateUsers),
			defaultRoleId: str(d.defaultRoleId, base.defaultRoleId)
		};
	}

	// svelte-ignore state_referenced_locally
	const claimed = bindBuilderContext(CONTEXT_ID, () => snapshot());
	// svelte-ignore state_referenced_locally
	let draft = $state<IdpDraft>(hydrate(claimed) ?? emptyDraft());

	/** A create surface opens EMPTY: there is nothing to save and nothing worth
	 *  parking. Saying `dirty: true` regardless is what made an untouched form
	 *  offer Save, and auto-stash itself onto the stage on the way out. */
	const PRISTINE = JSON.stringify(emptyDraft());
	const isDirty = () => JSON.stringify($state.snapshot(draft)) !== PRISTINE;

	let saving = $state(false);
	/** Parked on the stage — the pill must NOT re-enter this context. */
	let parked = $state(false);

	// The default-role select's option list. Supporting data only; a load
	// failure surfaces as a toast and leaves the select at "no default role".
	let roles = $state<Role[]>([]);

	onMount(() => {
		void loadRoles();
	});

	async function loadRoles() {
		try {
			roles = (await apiClient.listRoles()).roles;
		} catch (error) {
			console.error('Failed to load roles', error);
			toast.error(getLocalizedError(error));
		}
	}

	const defaultRoleLabel = $derived(
		draft.defaultRoleId
			? (roles.find((r) => r.id === draft.defaultRoleId)?.name ?? draft.defaultRoleId)
			: m.idp_field_default_role_none()
	);

	// The same schema the dialog submitted against, evaluated live so the commit
	// is closed at the store before it can be pressed.
	const createSchema = z.object({
		name: z.string().min(1, m.validation_name_required()),
		slug: z
			.string()
			.min(1)
			.regex(/^[a-z0-9]+$/, m.idp_field_slug_help()),
		clientId: z.string().min(1),
		clientSecret: z.string().min(1),
		issuerUrl: z.string().url()
	});

	const errors = $derived.by(() => {
		const out: Record<string, string> = {};
		const result = createSchema.safeParse({
			name: draft.name,
			slug: draft.slug,
			clientId: draft.clientId,
			clientSecret: draft.clientSecret,
			issuerUrl: draft.issuerUrl
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
			title: draft.name.trim() || m.idp_create_title(),
			dirty: isDirty(),
			valid: firstError === null,
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
		if (firstError) return;
		saving = true;
		try {
			const scopes = draft.scopes
				.split(',')
				.map((s) => s.trim())
				.filter(Boolean);
			const provider = await apiClient.createIdentityProvider({
				name: draft.name.trim(),
				slug: draft.slug.trim().toLowerCase(),
				providerType: IdentityProviderType.OIDC,
				clientId: draft.clientId.trim(),
				clientSecret: draft.clientSecret.trim(),
				issuerUrl: draft.issuerUrl.trim(),
				scopes: scopes.length > 0 ? scopes : ['openid', 'profile', 'email'],
				autoCreateUsers: draft.autoCreateUsers,
				defaultRoleId: draft.defaultRoleId
			});
			if (provider) {
				toast.success(m.idp_create_success());
				void goto(`/identity-providers/${provider.id}`);
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
		<!-- Name and slug are both short tokens, and so are the two credentials:
		     paired they read as the two halves they are, and the detail page
		     already lays the same four fields out this way. -->
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
			<div class="space-y-1.5">
				<Label for="idpClientSecret">{m.idp_field_client_secret()}</Label>
				<Input
					id="idpClientSecret"
					type="password"
					bind:value={draft.clientSecret}
					aria-invalid={!!errors.clientSecret}
				/>
				<FieldError error={errors.clientSecret} />
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
		<!-- JIT provisioning: same row/select idiom as the detail page's JIT
		     section. Auto-create defaults OFF here — an authenticated admin
		     opts in deliberately. -->
		<div class="flex items-center justify-between gap-3">
			<div class="space-y-0.5">
				<Label for="idpAutoCreate">{m.idp_field_auto_create_users()}</Label>
				<p class="text-xs text-muted-foreground">{m.idp_field_auto_create_users_help()}</p>
			</div>
			<Checkbox id="idpAutoCreate" bind:checked={draft.autoCreateUsers} />
		</div>
		<div class="space-y-1.5">
			<Label>{m.idp_field_default_role()}</Label>
			<Select.Root
				type="single"
				value={draft.defaultRoleId}
				onValueChange={(v) => (draft.defaultRoleId = v ?? '')}
			>
				<Select.Trigger class="w-full" aria-label={m.idp_field_default_role()}>
					{defaultRoleLabel}
				</Select.Trigger>
				<Select.Content>
					<Select.Item value="">{m.idp_field_default_role_none()}</Select.Item>
					{#each roles as role (role.id)}
						<Select.Item value={role.id}>{role.name}</Select.Item>
					{/each}
				</Select.Content>
			</Select.Root>
			<p class="text-xs text-muted-foreground">{m.idp_field_default_role_help()}</p>
		</div>
	</CreatePlate>
</div>
