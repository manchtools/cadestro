<script lang="ts">
	import { onMount, onDestroy, untrack } from 'svelte';
	import { goto } from '$lib/navigation';
	import { page } from '$app/state';
	import { toast } from 'svelte-sonner';
	import { apiClient, type IdentityProvider, type Role } from '$lib/sdk';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import PageShell from '$lib/components/page-shell.svelte';
	import FormSection from '$lib/components/create/form-section.svelte';
	import { Label } from '$lib/components/ui/label';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import * as Select from '$lib/components/ui/select';
	import ConfirmDeleteDialog from '$lib/components/confirm-delete-dialog.svelte';
	import { ArrowLeft, RefreshCw, Shield, Copy } from '@lucide/svelte';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import { Chip } from '$lib/components/fleet';
	import * as m from '$lib/paraglide/messages';
	import { getLocalizedError } from '$lib/errors';
	import {
		shell,
		enterContext,
		updateContext,
		exitContext,
		leaveContext,
		removeDraft,
		claimDraft,
		draftIdFor,
		type ContextState,
		type PillAction
	} from '$lib/shell/shell.svelte';

	let provider = $state<IdentityProvider | null>(null);
	let loading = $state(true);
	let saving = $state(false);
	let deleteDialogOpen = $state(false);

	// ── one editable buffer for the whole page ──────────────────────────────
	// Connection, endpoints and JIT are three sections of ONE edit: the operator
	// changes what they need across all of them and commits once, from the pill.
	// `baseline` is the snapshot the buffer is compared against, so "dirty" is a
	// fact about the loaded provider, not a flag someone has to remember to set.
	interface IdpForm {
		name: string;
		enabled: boolean;
		clientId: string;
		/** Write-only: blank means "keep the stored secret". */
		clientSecret: string;
		issuerUrl: string;
		authorizationUrl: string;
		tokenUrl: string;
		userinfoUrl: string;
		scopes: string;
		autoCreateUsers: boolean;
		autoLinkByEmail: boolean;
		trustEmail: boolean;
		/** Role granted to JIT-created users; empty string means "no default role". */
		defaultRoleId: string;
		groupClaim: string;
	}

	function formOf(p: IdentityProvider | null): IdpForm {
		return {
			name: p?.name ?? '',
			enabled: p?.enabled ?? false,
			clientId: p?.clientId?.value ?? '',
			clientSecret: '',
			issuerUrl: p?.issuerUrl ?? '',
			authorizationUrl: p?.authorizationUrl ?? '',
			tokenUrl: p?.tokenUrl ?? '',
			userinfoUrl: p?.userinfoUrl ?? '',
			scopes: p?.scopes.join(', ') ?? '',
			autoCreateUsers: p?.autoCreateUsers ?? false,
			autoLinkByEmail: p?.autoLinkByEmail ?? false,
			trustEmail: p?.trustEmailAssertions ?? false,
			defaultRoleId: p?.defaultRoleId?.value ?? '',
			groupClaim: p?.groupClaim ?? ''
		};
	}

	const FORM_KEYS = Object.keys(formOf(null)) as (keyof IdpForm)[];

	let form = $state<IdpForm>(formOf(null));
	let baseline = $state<IdpForm>(formOf(null));

	// The default-role select's option list. Supporting data only: the provider
	// stores a role ID, this list turns it into a name and offers the others.
	let roles = $state<Role[]>([]);

	// SCIM state
	let scimToken = $state('');
	let scimEnabling = $state(false);
	let scimDisabling = $state(false);
	let scimRotating = $state(false);
	let scimDisableDialogOpen = $state(false);
	let scimRotateDialogOpen = $state(false);

	const providerId = $derived(page.params.id ?? '');

	const redirectUri = $derived(
		provider ? `${window.location.origin}/auth/callback/${provider.slug}` : ''
	);

	const dirty = $derived(provider !== null && FORM_KEYS.some((k) => form[k] !== baseline[k]));

	// The slug is immutable here (the header shows it read-only), so the display
	// name is the one required field the editor can actually empty.
	const nameValid = $derived(form.name.trim().length > 0);

	onMount(() => {
		loadProvider();
		loadRoles();
	});

	async function loadRoles() {
		try {
			roles = (await apiClient.listRoles()).roles;
		} catch (error) {
			console.error('Failed to load roles', error);
			toast.error(getLocalizedError(error));
		}
	}

	/** The stored role may have been deleted since; showing its raw ID is more
	 *  honest than pretending "no default role" while the server still grants it. */
	const defaultRoleLabel = $derived(
		form.defaultRoleId
			? (roles.find((r) => (r.id?.value ?? '') === form.defaultRoleId)?.name ?? form.defaultRoleId)
			: m.idp_field_default_role_none()
	);

	async function loadProvider() {
		loading = true;
		try {
			const p = await apiClient.getIdentityProvider(providerId);
			if (p) {
				// SCIM enable/disable/rotate reload the provider too. An uncommitted
				// buffer outranks a background refresh: re-seeding here would drop the
				// operator's edits while the pill still promises to commit them.
				const keep = dirty;
				provider = p;
				if (!keep) resetForm(p);
				// Take back a draft this page parked on the stage. The buffer is
				// component state and did not survive the unmount, so the stash
				// snapshotted it onto the card; this is where it comes back. The
				// baseline stays the SERVER's provider, so `dirty` still means
				// "diverges from what is stored", not "diverges from the card".
				const parked = claimDraft(contextId) as IdpForm | undefined;
				if (parked) form = { ...parked };
			}
		} catch (error) {
			console.error('Failed to load identity provider', error);
			toast.error(getLocalizedError(error));
		} finally {
			loading = false;
		}
	}

	function resetForm(p: IdentityProvider) {
		form = formOf(p);
		baseline = formOf(p);
	}

	function revertEdits() {
		form = { ...baseline };
	}

	async function saveProvider() {
		saving = true;
		try {
			const scopes = form.scopes
				.split(',')
				.map((s) => s.trim())
				.filter(Boolean);
			const updated = await apiClient.updateIdentityProvider({
				id: providerId,
				name: form.name.trim(),
				enabled: form.enabled,
				clientId: form.clientId.trim(),
				clientSecret: form.clientSecret || undefined,
				issuerUrl: form.issuerUrl.trim(),
				authorizationUrl: form.authorizationUrl.trim(),
				tokenUrl: form.tokenUrl.trim(),
				userinfoUrl: form.userinfoUrl.trim(),
				scopes: scopes.length > 0 ? scopes : undefined,
				autoCreateUsers: form.autoCreateUsers,
				autoLinkByEmail: form.autoLinkByEmail,
				trustEmailAssertions: form.trustEmail,
				// Full-replace semantics: omitting this would CLEAR the stored
				// default role on every save. Empty string means "no default role".
				defaultRoleId: form.defaultRoleId,
				groupClaim: form.groupClaim.trim()
			});
			if (updated) {
				provider = updated;
				resetForm(updated);
				toast.success(m.idp_detail_updated());
			}
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			saving = false;
		}
	}

	// ── the pill is this provider's action bar ──────────────────────────────
	// No Save button anywhere on the page, and the pill is held for the WHOLE
	// visit rather than only while the form diverges: Delete and the SCIM
	// lifecycle act on the provider as a whole, and they cannot live somewhere
	// that only appears after you have typed. `dirty` still says whether there is
	// a form edit to commit, so ⌘S, Stash and Cancel stay honest. The SCIM
	// one-shots and the delete still write immediately and never join this
	// context's commit.
	const contextId = $derived(`identity-provider:${providerId}`);
	/** This editor's home — a stashed draft restores by navigating back to it. */
	const contextRoute = $derived(`/identity-providers/${providerId}`);

	const pillSubtext = $derived(nameValid ? (provider?.slug ?? '') : m.validation_name_required());

	// Plain `let`, not `$state`: the effect writes it, so a tracked read would
	// make the effect depend on its own write.
	let owns = false;

	/** The provider's own actions. SCIM is provisioning lifecycle, not a form
	 *  field: enable / rotate / disable each write on their own and change what
	 *  the NEXT action can be, so the set is rebuilt on every patch rather than
	 *  frozen at acquire time. Rotate and disable keep their confirm dialogs. */
	function entityActions(): PillAction[] {
		const actions: PillAction[] = [];
		if (!provider) return actions;
		if (!provider.scimEnabled) {
			actions.push({
				id: 'scim-enable',
				label: scimEnabling ? m.scim_enabling() : m.scim_enable(),
				onRun: () => void enableSCIM()
			});
		} else {
			actions.push({
				id: 'scim-rotate',
				label: m.scim_rotate_token(),
				onRun: () => (scimRotateDialogOpen = true)
			});
			actions.push({
				id: 'scim-disable',
				label: m.scim_disable(),
				onRun: () => (scimDisableDialogOpen = true)
			});
		}
		actions.push({
			id: 'delete',
			label: m.common_delete(),
			tone: 'danger',
			onRun: () => (deleteDialogOpen = true)
		});
		return actions;
	}

	function contextState(): ContextState {
		return {
			id: contextId,
			route: contextRoute,
			title: form.name || (provider?.name ?? ''),
			// The truth, not a constant: the pill is held for the whole visit, so it
			// must be able to say "nothing to save here". It also decides whether
			// leaving parks a draft — a provider the operator only looked at must not
			// land on the stage.
			dirty,
			valid: nameValid,
			commitLabel: m.common_save(),
			subtext: pillSubtext,
			subtextTone: nameValid ? 'neutral' : 'warn',
			onCommit: () => {
				// The store already exited context; a FAILED save re-acquires it
				// below, so the operator never loses the buffers with the commit.
				owns = false;
				void saveProvider();
			},
			onCancel: () => {
				owns = false;
				revertEdits();
			},
			// Stash releases the pill deliberately — without this the effect would
			// immediately re-acquire and the third exit would do nothing.
			onStash: () => {
				owns = false;
			},
			onRestore: () => {
				owns = true;
			},
			// The whole buffer rides ON the card — including the write-only
			// clientSecret, which exists nowhere else once this page unmounts.
			stashPayload: (): IdpForm => ({ ...$state.snapshot(form) }),
			stashSubtitle: m.idp_detail_title(),
			extraActions: entityActions()
		};
	}

	function acquire() {
		owns = true;
		// Resuming edits supersedes any card this context parked on the stage: the
		// live buffer is newer than the snapshot on the card, so the card goes
		// (claiming it here would only hand back staler edits).
		removeDraft(draftIdFor(contextId));
		enterContext(contextState());
	}

	function release() {
		owns = false;
		// Only tear down our own context — another surface may have taken over.
		if (shell.pill.context?.id === contextId) exitContext();
	}

	$effect(() => {
		// Read every reactive input HERE. Held from load to unmount, not only while
		// dirty: the pill IS this provider's action bar. `saving` still parks it for
		// the round trip — an in-flight commit has no second commit to offer.
		const active = provider !== null && !saving;
		const patch = {
			title: form.name || (provider?.name ?? ''),
			valid: nameValid,
			// Tracked here too: the pill turns Save, Stash and Cancel on and off from
			// this, and it flips on every keystroke.
			dirty,
			subtext: pillSubtext,
			subtextTone: (nameValid ? 'neutral' : 'warn') as 'neutral' | 'warn',
			// Enabling SCIM replaces "Enable" with "Rotate"/"Disable" while the pill
			// is still held, so the action set has to ride the patch, not the acquire.
			extraActions: entityActions()
		};
		// …and write to the store UNTRACKED. The store helpers read
		// `shell.pill.context` themselves, so a tracked call would make this
		// effect depend on the pill it just wrote — and Stash, which clears the
		// context, would be undone by an instant re-acquire.
		untrack(() => {
			if (!active) {
				if (owns) release();
				return;
			}
			if (owns) updateContext(patch);
			else acquire();
		});
	});

	// If the page goes away mid-edit the pill must not keep pointing at a
	// provider that is no longer on screen — and must not DISCARD the buffer
	// (the write-only client secret lives nowhere else once this unmounts).
	// Auto-stash-on-navigate parks it on the stage instead; a commit/cancel/stash
	// already cleared `owns`, so this only fires on a genuine leave.
	onDestroy(() => {
		if (owns) {
			owns = false;
			leaveContext(contextId);
		}
	});

	async function deleteProvider() {
		try {
			await apiClient.deleteIdentityProvider(providerId);
			toast.success(m.idp_detail_deleted());
			goto('/identity-providers');
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		}
	}

	async function enableSCIM() {
		scimEnabling = true;
		try {
			const result = await apiClient.enableSCIM(providerId);
			if (result) {
				scimToken = result.token;
				toast.success(m.scim_enabled_success());
				await loadProvider();
			}
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			scimEnabling = false;
		}
	}

	async function disableSCIM() {
		scimDisabling = true;
		try {
			await apiClient.disableSCIM(providerId);
			scimToken = '';
			toast.success(m.scim_disabled_success());
			await loadProvider();
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			scimDisabling = false;
			scimDisableDialogOpen = false;
		}
	}

	async function rotateSCIMToken() {
		scimRotating = true;
		try {
			const result = await apiClient.rotateSCIMToken(providerId);
			if (result) {
				scimToken = result.token;
				toast.success(m.scim_rotated_success());
			}
		} catch (error) {
			toast.error(getLocalizedError(error));
			console.error(error);
		} finally {
			scimRotating = false;
			scimRotateDialogOpen = false;
		}
	}

	function copyToClipboard(text: string, successMsg: string) {
		navigator.clipboard.writeText(text);
		toast.success(successMsg);
	}
</script>

{#snippet sectionLabel(text: string)}
	<span class="font-mono text-[0.62rem] tracking-[0.1em] text-faint uppercase">{text}</span>
{/snippet}

<PageShell contentClass="space-y-4">
	{#snippet header()}
		<div class="flex items-start gap-3">
			<Button
				variant="ghost"
				size="icon"
				aria-label={m.common_back()}
				onclick={() => goto('/identity-providers')}
			>
				<ArrowLeft class="h-4 w-4" />
			</Button>
			<div class="min-w-0 flex-1">
				<div class="flex flex-wrap items-center gap-2">
					<h1 class="truncate text-2xl font-bold">{provider?.name ?? m.idp_detail_title()}</h1>
					{#if provider}
						<Chip
							tone={provider.enabled ? 'ok' : 'idle'}
							label={provider.enabled ? m.idp_enabled() : m.idp_disabled()}
						/>
						{#if provider.scimEnabled}
							<Chip tone="info" label={m.scim_enabled_badge()} />
						{/if}
					{/if}
				</div>
				<p class="font-mono text-xs text-faint">{provider?.slug ?? ''}</p>
			</div>
			<div class="flex gap-2">
				<Button onclick={loadProvider} variant="outline" size="sm" disabled={loading || saving}>
					<RefreshCw class="mr-2 h-4 w-4 {loading ? 'animate-spin' : ''}" />
					{m.common_refresh()}
				</Button>
			</div>
		</div>
	{/snippet}

	<!-- The editor is a reading column, the same measure a create surface gets.
	     The SCIM band keeps its two panes inside it; it is not flattened. -->
	<div class="max-w-3xl space-y-4">
		{#if loading && !provider}
			<div class="flex items-center justify-center rounded-xl border border-hair bg-surface py-12">
				<RefreshCw class="h-6 w-6 animate-spin text-muted-foreground" />
			</div>
		{:else if provider}
			<!-- Connection, endpoints and JIT are three sections of ONE commit, so they
			     are three sections of one plate — three separate plates suggested three
			     separate saves that never existed. -->
			<div class="space-y-4 rounded-xl border border-hair bg-surface p-4 shadow-plate">
				<FormSection title={m.idp_section_connection()} lead>
					<div class="grid gap-3 sm:grid-cols-2">
						<div class="space-y-1.5">
							<Label for="editName">{m.idp_field_name()}</Label>
							<Input id="editName" bind:value={form.name} />
						</div>
						<div class="space-y-1.5">
							<Label for="idpSlugRead">{m.idp_field_slug()}</Label>
							<Input id="idpSlugRead" value={provider.slug} disabled class="font-mono" />
						</div>
					</div>
					<div class="space-y-1.5">
						<Label for="idpRedirect">{m.idp_field_redirect_uri()}</Label>
						<div class="flex items-center gap-2">
							<Input id="idpRedirect" value={redirectUri} readonly class="bg-sunken font-mono text-xs" />
							<Button
								variant="outline"
								size="icon"
								aria-label={m.idp_copy_redirect_uri()}
								onclick={() => copyToClipboard(redirectUri, m.idp_redirect_uri_copied())}
							>
								<Copy class="h-4 w-4" />
							</Button>
						</div>
						<p class="text-xs text-muted-foreground">{m.idp_field_redirect_uri_help()}</p>
					</div>
					<div class="grid gap-3 sm:grid-cols-2">
						<div class="space-y-1.5">
							<Label for="editClientId">{m.idp_field_client_id()}</Label>
							<Input id="editClientId" bind:value={form.clientId} class="font-mono text-xs" />
						</div>
						<div class="space-y-1.5">
							<Label for="editClientSecret">{m.idp_field_client_secret()}</Label>
							<Input
								id="editClientSecret"
								type="password"
								placeholder="***"
								bind:value={form.clientSecret}
							/>
							<p class="text-xs text-muted-foreground">{m.idp_field_client_secret_help()}</p>
						</div>
					</div>
					<div class="flex items-center justify-between gap-3">
						<Label for="editEnabled">{m.idp_field_enabled()}</Label>
						<Checkbox id="editEnabled" bind:checked={form.enabled} />
					</div>
				</FormSection>

				<FormSection title={m.idp_section_endpoints()}>
					<!-- One URL per row: three columns turned every endpoint into a
					     scrolling sliver of the value it holds. -->
					<div class="space-y-1.5">
						<Label for="editIssuerUrl">{m.idp_field_issuer_url()}</Label>
						<Input id="editIssuerUrl" type="url" bind:value={form.issuerUrl} class="font-mono text-xs" />
						<p class="text-xs text-muted-foreground">{m.idp_field_issuer_url_help()}</p>
					</div>
					<div class="space-y-1.5">
						<Label for="editAuthUrl">{m.idp_field_authorization_url()}</Label>
						<Input id="editAuthUrl" bind:value={form.authorizationUrl} class="font-mono text-xs" />
					</div>
					<div class="space-y-1.5">
						<Label for="editTokenUrl">{m.idp_field_token_url()}</Label>
						<Input id="editTokenUrl" bind:value={form.tokenUrl} class="font-mono text-xs" />
					</div>
					<div class="space-y-1.5">
						<Label for="editUserinfoUrl">{m.idp_field_userinfo_url()}</Label>
						<Input id="editUserinfoUrl" bind:value={form.userinfoUrl} class="font-mono text-xs" />
					</div>
					<div class="space-y-1.5">
						<Label for="editScopes">{m.idp_field_scopes()}</Label>
						<Input
							id="editScopes"
							bind:value={form.scopes}
							placeholder="openid, profile, email"
							class="font-mono text-xs"
						/>
						<p class="text-xs text-muted-foreground">{m.idp_field_scopes_help()}</p>
					</div>
				</FormSection>

				<FormSection title={m.idp_section_jit()} description={m.idp_section_jit_hint()}>
					<div class="flex items-center justify-between gap-3">
						<div class="space-y-0.5">
							<Label for="editAutoCreate">{m.idp_field_auto_create_users()}</Label>
							<p class="text-xs text-muted-foreground">{m.idp_field_auto_create_users_help()}</p>
						</div>
						<Checkbox id="editAutoCreate" bind:checked={form.autoCreateUsers} />
					</div>
					<div class="space-y-1.5">
						<Label>{m.idp_field_default_role()}</Label>
						<Select.Root
							type="single"
							value={form.defaultRoleId}
							onValueChange={(v) => (form.defaultRoleId = v ?? '')}
						>
							<Select.Trigger class="w-full" aria-label={m.idp_field_default_role()}>
								{defaultRoleLabel}
							</Select.Trigger>
							<Select.Content>
								<Select.Item value="">{m.idp_field_default_role_none()}</Select.Item>
								{#each roles as role (role.id)}
									<Select.Item value={(role.id?.value ?? '')}>{role.name}</Select.Item>
								{/each}
							</Select.Content>
						</Select.Root>
						<p class="text-xs text-muted-foreground">{m.idp_field_default_role_help()}</p>
					</div>
					<div class="flex items-center justify-between gap-3">
						<div class="space-y-0.5">
							<Label for="editAutoLink">{m.idp_field_auto_link_by_email()}</Label>
							<p class="text-xs text-muted-foreground">{m.idp_field_auto_link_by_email_help()}</p>
						</div>
						<Checkbox id="editAutoLink" bind:checked={form.autoLinkByEmail} />
					</div>
					<div class="flex items-center justify-between gap-3">
						<div class="space-y-0.5">
							<Label for="editTrustEmail">{m.idp_field_trust_email_assertions()}</Label>
							<p class="text-xs text-muted-foreground">
								{m.idp_field_trust_email_assertions_help()}
							</p>
						</div>
						<Checkbox id="editTrustEmail" bind:checked={form.trustEmail} />
					</div>
					<div class="space-y-1.5">
						<Label for="editGroupClaim">{m.idp_field_group_claim()}</Label>
						<Input
							id="editGroupClaim"
							bind:value={form.groupClaim}
							placeholder="groups"
							class="font-mono text-xs"
						/>
						<p class="text-xs text-muted-foreground">{m.idp_field_group_claim_help()}</p>
					</div>
				</FormSection>
			</div>

			<!-- ── SCIM: what has to be READ ─────────────────────────────────── -->
			<!-- Enable, rotate and disable are not here. They act on the provider as
			     a whole, so they are pill actions next to Delete; what stays is the
			     state only this page can show — the endpoint the directory posts to,
			     and the bearer token that is displayed exactly once. -->
			<section
				data-testid="idp-scim-section"
				class="space-y-2 rounded-xl border border-hair bg-surface p-4 shadow-plate"
			>
				<span class="flex items-center gap-2">
					<Shield class="h-4 w-4 text-faint" />
					{@render sectionLabel(m.scim_title())}
				</span>
				<p class="max-w-[70ch] text-sm text-muted-foreground">{m.scim_description()}</p>
				{#if provider.scimEnabled}
					<div class="space-y-1.5 pt-1">
						<Label for="scimEndpoint">{m.scim_endpoint_url()}</Label>
						<div class="flex items-center gap-2">
							<Input
								id="scimEndpoint"
								value={provider.scimEndpointUrl}
								readonly
								class="bg-sunken font-mono text-xs"
							/>
							<Button
								variant="outline"
								size="icon"
								aria-label={m.idp_copy_scim_url()}
								onclick={() => copyToClipboard(provider?.scimEndpointUrl ?? '', m.scim_url_copied())}
							>
								<Copy class="h-4 w-4" />
							</Button>
						</div>
					</div>
				{/if}
				{#if scimToken}
					<!-- Shown once, straight from the enable/rotate response — the
					     provider record never carries the bearer token back. -->
					<div class="space-y-2 rounded-lg border border-warn/50 bg-warn-soft p-3">
						<p class="text-xs font-semibold text-warn">{m.scim_token()}</p>
						<div class="flex items-center gap-2">
							<code
								class="min-w-0 flex-1 overflow-x-auto rounded-md border bg-sunken px-2 py-1.5 font-mono text-[0.7rem] break-all whitespace-pre-wrap"
								>{scimToken}</code
							>
							<Button
								variant="outline"
								size="icon"
								aria-label={m.idp_copy_scim_token()}
								onclick={() => copyToClipboard(scimToken, m.scim_token_copied())}
							>
								<Copy class="h-4 w-4" />
							</Button>
						</div>
						<p class="text-xs text-warn">{m.scim_token_warning()}</p>
					</div>
				{/if}
			</section>

			<!-- No danger zone: Delete is a pill action now. A destructive control
			     parked below three form sections was both hard to reach and easy to
			     hit on the way past; the confirm dialog is still the gate. -->
		{/if}
	</div>
</PageShell>

<ConfirmDeleteDialog
	bind:open={deleteDialogOpen}
	title={m.common_delete()}
	description={m.idp_detail_confirm_delete()}
	onconfirm={deleteProvider}
/>

<AlertDialog.Root bind:open={scimRotateDialogOpen}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{m.scim_rotate_token()}</AlertDialog.Title>
			<AlertDialog.Description>{m.scim_rotate_confirm()}</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>{m.common_cancel()}</AlertDialog.Cancel>
			<AlertDialog.Action onclick={rotateSCIMToken} disabled={scimRotating}>
				{scimRotating ? m.scim_rotating() : m.scim_rotate_token()}
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

<AlertDialog.Root bind:open={scimDisableDialogOpen}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{m.scim_disable()}</AlertDialog.Title>
			<AlertDialog.Description>{m.scim_disable_confirm()}</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>{m.common_cancel()}</AlertDialog.Cancel>
			<AlertDialog.Action onclick={disableSCIM} disabled={scimDisabling}>
				{scimDisabling ? m.scim_disabling() : m.scim_disable()}
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
