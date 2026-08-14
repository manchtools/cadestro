<script lang="ts">
	// /setup — server-URL configuration AND the one-time bootstrap onboarding.
	//
	// `control bootstrap-admin` prints <origin>/setup#bootstrap_token=<T>. The
	// token is single-use, ~15min TTL, and is presented to control as
	// `Authorization: Cadestro-Bootstrap <T>` (NOT a Bearer session token). It
	// buys exactly ONE authenticated call — registering the first OIDC provider —
	// and is then spent. It is never stored, never logged, and never exchanged for
	// a session.
	//
	// The token is captured into in-memory state on mount so it survives the
	// server-URL → provider step transition (which is in-page, never a route
	// change that would drop the fragment). It is scrubbed from both memory and
	// the URL only after the single call succeeds.
	import { onMount } from 'svelte';
	import { goto } from '$lib/navigation';
	import { toast } from 'svelte-sonner';
	import { z } from 'zod';
	import { ConnectError, Code } from '@connectrpc/connect';
	import { configStore, apiClient } from '$lib/sdk';
	import { IdentityProviderType } from '$contract/cadestro/v1/common_pb';
	import { checkAndSwitchVersion } from '$lib/version';
	import { getLocalizedError, getErrorCode } from '$lib/errors';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import * as Select from '$lib/components/ui/select';
	import { Server, Fingerprint, Copy } from '@lucide/svelte';
	import { createFormValidation } from '$lib/forms';
	import { setupSchema } from '$lib/forms/schemas/auth';
	import { FieldError } from '$lib/components/ui/field-error';
	import * as m from '$lib/paraglide/messages';

	// ── server-URL step ──────────────────────────────────────────────────────
	let serverUrl = $state(configStore.serverUrl || '');
	let testing = $state(false);
	const fv = createFormValidation(setupSchema);

	// ── bootstrap onboarding state ───────────────────────────────────────────
	// Memory-only. Never persisted, never logged.
	let bootstrapToken = $state<string | null>(null);
	// Whether the server URL is known yet. Seeded from config, then flipped in
	// the bootstrap flow when the server step completes — an explicit flag rather
	// than reading config so the in-page step transition is deterministic.
	let serverReady = $state(!!configStore.serverUrl);
	let submitting = $state(false);
	let bootstrapError = $state('');

	const showProviderStep = $derived(!!bootstrapToken && serverReady);

	onMount(() => {
		// Capture the token BEFORE anything can navigate. The fragment is not
		// storage; it is left in the URL until the call succeeds so it also
		// survives a version-switch reload, then scrubbed on success.
		const hash = window.location.hash.replace(/^#/, '');
		const token = new URLSearchParams(hash).get('bootstrap_token');
		if (token) bootstrapToken = token;
	});

	// ── provider form (same field set + validation as identity-providers/new) ──
	//
	// JIT is preconfigured, not fetched: the bootstrap token buys exactly ONE
	// RPC — the CreateIdentityProvider call — so this page cannot call ListRoles.
	// The two system roles have fixed, well-known IDs; the select offers exactly
	// those plus "no default role".
	const SYSTEM_ROLE_ADMIN_ID = '00000000000000000000000001';
	const SYSTEM_ROLE_USER_ID = '00000000000000000000000002';

	interface IdpDraft {
		name: string;
		slug: string;
		clientId: string;
		clientSecret: string;
		issuerUrl: string;
		scopes: string;
		autoCreateUsers: boolean;
		defaultRoleId: string;
	}
	// Auto-create defaults ON with the User role: the whole point of the OIDC
	// bootstrap path is that the first sign-in through this provider creates
	// the account — and least privilege is the safe default for what it gets.
	let draft = $state<IdpDraft>({
		name: '',
		slug: '',
		clientId: '',
		clientSecret: '',
		issuerUrl: '',
		scopes: '',
		autoCreateUsers: true,
		defaultRoleId: SYSTEM_ROLE_USER_ID
	});

	const bootstrapRoleLabel = $derived(
		draft.defaultRoleId === SYSTEM_ROLE_ADMIN_ID
			? m.setup_bootstrap_role_admin()
			: draft.defaultRoleId === SYSTEM_ROLE_USER_ID
				? m.setup_bootstrap_role_user()
				: m.idp_field_default_role_none()
	);

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

	const providerErrors = $derived.by(() => {
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
	const firstError = $derived(Object.values(providerErrors)[0] ?? null);

	// The URI the operator must whitelist in their IdP. Shown read-only so it can
	// be copied — mirrors the identity-providers/[id] detail page.
	const redirectUri = $derived(
		typeof window !== 'undefined' && draft.slug.trim()
			? `${window.location.origin}/auth/callback/${draft.slug.trim().toLowerCase()}`
			: ''
	);

	async function testConnection() {
		if (!serverUrl) {
			toast.error(m.setup_enter_url());
			return;
		}
		testing = true;
		try {
			const response = await fetch(`${serverUrl}/health`, { method: 'GET', mode: 'cors' });
			if (response.ok) {
				toast.success(m.setup_connection_success());
			} else {
				toast.warning(m.setup_connection_warning());
			}
		} catch {
			toast.info(m.setup_connection_info());
		} finally {
			testing = false;
		}
	}

	async function saveAndContinue() {
		if (!fv.validate({ serverUrl })) return;

		const normalizedUrl = serverUrl.replace(/\/+$/, '');
		configStore.serverUrl = normalizedUrl;
		toast.success(m.setup_configured());

		// Version mismatch reloads the app. The bootstrap token rides along in the
		// fragment and is re-parsed on the fresh mount.
		const switched = await checkAndSwitchVersion(normalizedUrl);
		if (switched) return;

		if (bootstrapToken) {
			// Advance to the provider step in-page — no route change, so the token
			// held in memory is untouched.
			serverReady = true;
			return;
		}
		goto('/login');
	}

	function isTokenRejected(err: unknown): boolean {
		if (err instanceof ConnectError && err.code === Code.Unauthenticated) return true;
		return getErrorCode(err) === 'not_authenticated';
	}

	function scrubFragment() {
		if (typeof window === 'undefined') return;
		// Remove the fragment (and thus the token) from the address bar without a
		// navigation or reload.
		window.history.replaceState(
			window.history.state,
			'',
			window.location.pathname + window.location.search
		);
	}

	async function registerProvider() {
		if (firstError || !bootstrapToken || submitting) return;
		submitting = true;
		bootstrapError = '';
		try {
			const scopes = draft.scopes
				.split(',')
				.map((s) => s.trim())
				.filter(Boolean);
			await apiClient.createIdentityProviderWithBootstrapToken(bootstrapToken, {
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

			// Spent. Scrub the token from memory and the URL, then sign in through
			// the provider we just created.
			bootstrapToken = null;
			scrubFragment();
			toast.success(m.idp_create_success());
			goto('/login');
		} catch (err) {
			// The token is single-use: on a spent/expired/invalid token, tell the
			// operator to mint a fresh one. Never retry automatically.
			bootstrapError = isTokenRejected(err)
				? m.setup_bootstrap_token_rejected()
				: getLocalizedError(err);
		} finally {
			submitting = false;
		}
	}

	async function copyRedirect() {
		if (!redirectUri) return;
		try {
			await navigator.clipboard.writeText(redirectUri);
			toast.success(m.idp_redirect_uri_copied());
		} catch {
			// Clipboard may be unavailable; the field is selectable to copy by hand.
		}
	}
</script>

{#if showProviderStep}
	<!-- Bootstrap onboarding: register the first OIDC provider by spending the
	     single-use setup token on exactly one authenticated call. -->
	<div class="flex min-h-screen items-center justify-center bg-page p-4">
		<div class="w-full max-w-md rounded-[14px] border bg-surface shadow-plate">
			<div class="space-y-1 px-6 pb-4 pt-6">
				<div class="flex items-center gap-2">
					<Fingerprint class="h-5 w-5 text-muted-foreground" />
					<h1 class="text-2xl font-semibold tracking-tight">{m.setup_bootstrap_title()}</h1>
				</div>
				<p class="text-sm text-muted-foreground">{m.setup_bootstrap_description()}</p>
			</div>
			<div class="px-6 pb-6">
				<form
					onsubmit={(e) => {
						e.preventDefault();
						registerProvider();
					}}
					class="space-y-4"
				>
					<div class="space-y-1.5">
						<Label for="idpName">{m.idp_field_name()}</Label>
						<Input id="idpName" bind:value={draft.name} aria-invalid={!!providerErrors.name} />
						<FieldError error={providerErrors.name} />
					</div>
					<div class="space-y-1.5">
						<Label for="idpSlug">{m.idp_field_slug()}</Label>
						<Input id="idpSlug" bind:value={draft.slug} aria-invalid={!!providerErrors.slug} />
						<p class="text-xs text-muted-foreground">{m.idp_field_slug_help()}</p>
						<FieldError error={providerErrors.slug} />
					</div>
					<div class="space-y-1.5">
						<Label for="idpClientId">{m.idp_field_client_id()}</Label>
						<Input
							id="idpClientId"
							bind:value={draft.clientId}
							aria-invalid={!!providerErrors.clientId}
						/>
						<FieldError error={providerErrors.clientId} />
					</div>
					<div class="space-y-1.5">
						<Label for="idpClientSecret">{m.idp_field_client_secret()}</Label>
						<Input
							id="idpClientSecret"
							type="password"
							bind:value={draft.clientSecret}
							aria-invalid={!!providerErrors.clientSecret}
						/>
						<FieldError error={providerErrors.clientSecret} />
					</div>
					<div class="space-y-1.5">
						<Label for="idpIssuerUrl">{m.idp_field_issuer_url()}</Label>
						<Input
							id="idpIssuerUrl"
							type="url"
							placeholder="https://accounts.google.com"
							bind:value={draft.issuerUrl}
							aria-invalid={!!providerErrors.issuerUrl}
						/>
						<p class="text-xs text-muted-foreground">{m.idp_field_issuer_url_help()}</p>
						<FieldError error={providerErrors.issuerUrl} />
					</div>
					<div class="space-y-1.5">
						<Label for="idpScopes">{m.idp_field_scopes()}</Label>
						<Input id="idpScopes" bind:value={draft.scopes} placeholder="openid, profile, email" />
						<p class="text-xs text-muted-foreground">{m.idp_field_scopes_help()}</p>
					</div>

					<!-- JIT provisioning. Auto-create defaults ON: the first sign-in
					     through this provider is what creates the operator's account. -->
					<div class="flex items-center justify-between gap-3">
						<div class="space-y-0.5">
							<Label for="idpAutoCreate">{m.idp_field_auto_create_users()}</Label>
							<p class="text-xs text-muted-foreground">{m.setup_bootstrap_auto_create_help()}</p>
						</div>
						<Checkbox id="idpAutoCreate" bind:checked={draft.autoCreateUsers} />
					</div>

					<!-- No ListRoles here — the bootstrap token buys exactly one RPC, so
					     the select offers only the two fixed system roles and "none". -->
					<div class="space-y-1.5">
						<Label>{m.idp_field_default_role()}</Label>
						<Select.Root
							type="single"
							value={draft.defaultRoleId}
							onValueChange={(v) => (draft.defaultRoleId = v ?? '')}
						>
							<Select.Trigger class="w-full" aria-label={m.idp_field_default_role()}>
								{bootstrapRoleLabel}
							</Select.Trigger>
							<Select.Content>
								<Select.Item value={SYSTEM_ROLE_USER_ID}>{m.setup_bootstrap_role_user()}</Select.Item>
								<Select.Item value={SYSTEM_ROLE_ADMIN_ID}>{m.setup_bootstrap_role_admin()}</Select.Item>
								<Select.Item value="">{m.idp_field_default_role_none()}</Select.Item>
							</Select.Content>
						</Select.Root>
						<p class="text-xs text-muted-foreground">{m.setup_bootstrap_default_role_help()}</p>
					</div>

					<!-- The redirect/callback URI the operator whitelists in the IdP. -->
					<div class="space-y-1.5">
						<Label for="idpRedirect">{m.idp_field_redirect_uri()}</Label>
						<div class="flex gap-2">
							<Input
								id="idpRedirect"
								value={redirectUri}
								readonly
								class="bg-sunken font-mono text-xs"
							/>
							<Button
								type="button"
								variant="outline"
								size="icon"
								aria-label={m.idp_copy_redirect_uri()}
								disabled={!redirectUri}
								onclick={copyRedirect}
							>
								<Copy class="h-4 w-4" />
							</Button>
						</div>
						<p class="text-xs text-muted-foreground">{m.idp_field_redirect_uri_help()}</p>
					</div>

					{#if bootstrapError}
						<p role="alert" class="text-sm text-crit">{bootstrapError}</p>
					{/if}

					<Button type="submit" class="w-full" disabled={!!firstError || submitting}>
						{submitting ? m.setup_bootstrap_submitting() : m.setup_bootstrap_submit()}
					</Button>
				</form>
			</div>
		</div>
	</div>
{:else}
	<!-- Server-URL step: the same one-field form on a centered plate. -->
	<div class="flex min-h-screen items-center justify-center bg-page p-4">
		<div class="w-full max-w-md rounded-[14px] border bg-surface shadow-plate">
			<div class="space-y-1 px-6 pb-4 pt-6">
				<div class="flex items-center gap-2">
					<Server class="h-5 w-5 text-muted-foreground" />
					<h1 class="text-2xl font-semibold tracking-tight">{m.setup_title()}</h1>
				</div>
				<p class="text-sm text-muted-foreground">
					{m.setup_description()}
				</p>
			</div>
			<div class="px-6 pb-6">
				<form
					onsubmit={(e) => {
						e.preventDefault();
						saveAndContinue();
					}}
					class="space-y-4"
				>
					<div class="space-y-2">
						<Label for="serverUrl">{m.setup_url_label()}</Label>
						<Input
							id="serverUrl"
							type="url"
							class="font-mono text-sm"
							placeholder={m.setup_url_placeholder()}
							bind:value={serverUrl}
							required
							aria-invalid={!!fv.errors.serverUrl}
						/>
						<FieldError error={fv.errors.serverUrl} />
						<p class="text-xs text-muted-foreground">
							{m.setup_url_hint()}
						</p>
					</div>

					<div class="flex gap-2">
						<Button
							type="button"
							variant="outline"
							onclick={testConnection}
							disabled={testing || !serverUrl}
							class="flex-1"
						>
							{testing ? m.setup_testing() : m.setup_test()}
						</Button>
						<Button type="submit" disabled={!serverUrl} class="flex-1">
							{m.setup_continue()}
						</Button>
					</div>
				</form>
			</div>
		</div>
	</div>
{/if}
