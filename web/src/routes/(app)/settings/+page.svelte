<script lang="ts">

	import { onMount } from 'svelte';
	import type { Snippet } from 'svelte';
	import { goto } from '$lib/navigation';
	import { toast } from 'svelte-sonner';
	import {
		configStore,
		authStore,
		apiClient,
		fetchAllPages,
		formatTimestampDateTime,
		type ApiToken,
		type IdentityLink,
		type SshPublicKey
	} from '$lib/sdk';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Select from '$lib/components/ui/select';
	import { Key, LogOut, RefreshCw, Unlink, Plus, X, Copy } from '@lucide/svelte';
	import { Textarea } from '$lib/components/ui/textarea';
	import PageShell from '$lib/components/page-shell.svelte';
	import { getVersionCookie, clearVersionCookie } from '$lib/version';
	import { fetchHealth } from '$lib/health';
	import { startTour } from '$lib/onboarding';
	import * as m from '$lib/paraglide/messages';
	import { getLocale, setLocale } from '$lib/paraglide/runtime';
	import { getLocalizedError } from '$lib/errors';
	import { Switch } from '$lib/components/ui/switch';
	import { userPrefersMode, setMode } from 'mode-watcher';

	const languages = [
		{ value: 'en', label: () => m.language_en() },
		{ value: 'de', label: () => m.language_de() }
	] as const;

	const themes = [
		{ value: 'system', label: () => m.settings_theme_system() },
		{ value: 'light', label: () => m.settings_theme_light() },
		{ value: 'dark', label: () => m.settings_theme_dark() }
	] as const;

	function handleLanguageChange(value: string | undefined) {
		if (value && value !== getLocale()) {
			setLocale(value as 'en' | 'de');
		}
	}

	function handleThemeChange(value: string | undefined) {
		if (value === 'light' || value === 'dark' || value === 'system') setMode(value);
	}

	let logoutDialogOpen = $state(false);
	let serverVersion = $state('');
	let pinnedVersion = $state<string | null>(null);

	let identityLinks = $state<IdentityLink[]>([]);
	let identityLinksLoading = $state(true);
	let unlinkDialogOpen = $state(false);
	let unlinkingLinkId = $state('');
	let unlinkLoading = $state(false);

	let sshKeys = $state<SshPublicKey[]>([]);
	let addSshKeyOpen = $state(false);
	let newSshKeyValue = $state('');
	let newSshKeyComment = $state('');
	let removeSshKeyConfirmOpen = $state(false);
	let removingSshKeyId = $state('');

	let rebuildingSearchIndex = $state(false);

	let globalUserProvisioning = $state(false);
	let globalSshAccessForAll = $state(false);
	let settingsLoaded = $state(false);
	let apiTokens = $state<ApiToken[]>([]);
	let apiTokensLoading = $state(false);
	let apiTokenDialogOpen = $state(false);
	let apiTokenName = $state('');
	let apiTokenExpiresAt = $state('');
	let apiTokenValue = $state('');
	let apiTokenCreating = $state(false);
	let revokeApiTokenConfirmOpen = $state(false);
	let apiTokenToRevoke = $state<ApiToken | null>(null);
	let apiTokenRevoking = $state(false);

	const canManageSshKeys = $derived(authStore.hasPermission('AddUserSshKey:self'));
	const canRebuildIndex = $derived(authStore.hasPermission('RebuildSearchIndex'));
	const canProvision = $derived(settingsLoaded && authStore.hasPermission('UpdateServerSettings'));
	const canManageApiTokens = $derived(
		authStore.hasPermission('CreateApiToken') &&
		authStore.hasPermission('ListApiTokens') &&
		authStore.hasPermission('RevokeApiToken')
	);

	onMount(async () => {
		pinnedVersion = getVersionCookie();
		try {
			const { response, version } = await fetchHealth(configStore.serverUrl);
			if (response.ok) {
				serverVersion = version ?? '';
			}
		} catch (err) {
			console.warn(err);
		}

		await Promise.allSettled([loadIdentityLinks(), loadSshKeys(), loadServerSettings(), loadApiTokens()]);
	});

	function apiTokenStatus(token: ApiToken): 'active' | 'expired' | 'revoked' {
		if (token.revokedAt) return 'revoked';
		if (token.expiresAt && new Date(Number(token.expiresAt.seconds) * 1000) <= new Date()) return 'expired';
		return 'active';
	}

	function apiTokenStatusLabel(token: ApiToken): string {
		const status = apiTokenStatus(token);
		if (status === 'revoked') return m.settings_api_tokens_status_revoked();
		if (status === 'expired') return m.settings_api_tokens_status_expired();
		return m.settings_api_tokens_status_active();
	}

	async function loadApiTokens() {
		if (!canManageApiTokens) return;
		apiTokensLoading = true;
		try {
			apiTokens = await fetchAllPages<ApiToken>(async (pageSize, pageToken) => {
				const response = await apiClient.listApiTokens(pageSize, pageToken);
				return { items: response.tokens, nextPageToken: response.nextPageToken };
			});
		} catch (error) {
			console.warn('Failed to load API tokens', error);
			apiTokens = [];
		} finally {
			apiTokensLoading = false;
		}
	}

	function openApiTokenDialog() {
		apiTokenName = '';
		apiTokenExpiresAt = '';
		apiTokenValue = '';
		apiTokenDialogOpen = true;
	}

	function handleApiTokenDialogOpen(open: boolean) {
		apiTokenDialogOpen = open;
		if (!open) {
			apiTokenName = '';
			apiTokenExpiresAt = '';
			apiTokenValue = '';
		}
	}

	async function createApiToken() {
		const name = apiTokenName.trim();
		const expiresAt = new Date(apiTokenExpiresAt);
		if (!name || !apiTokenExpiresAt || Number.isNaN(expiresAt.getTime()) || expiresAt <= new Date()) return;
		apiTokenCreating = true;
		try {
			const response = await apiClient.createApiToken(name, expiresAt);
			apiTokenValue = response.value;
			toast.success(m.settings_api_tokens_created_success());
			await loadApiTokens();
		} catch (error) {
			toast.error(getLocalizedError(error));
		} finally {
			apiTokenCreating = false;
		}
	}

	async function copyApiToken() {
		if (!apiTokenValue) return;
		try {
			await navigator.clipboard.writeText(apiTokenValue);
			toast.success(m.settings_api_tokens_copied());
		} catch (error) {
			toast.error(getLocalizedError(error));
		}
	}

	function confirmRevokeApiToken(token: ApiToken) {
		apiTokenToRevoke = token;
		revokeApiTokenConfirmOpen = true;
	}

	async function revokeApiToken() {
		const id = apiTokenToRevoke?.id?.value ?? '';
		if (!id) return;
		apiTokenRevoking = true;
		try {
			await apiClient.revokeApiToken(id);
			toast.success(m.settings_api_tokens_revoked_success());
			revokeApiTokenConfirmOpen = false;
			await loadApiTokens();
		} catch (error) {
			toast.error(getLocalizedError(error));
		} finally {
			apiTokenRevoking = false;
			apiTokenToRevoke = null;
		}
	}

	async function loadIdentityLinks() {
		identityLinksLoading = true;
		try {
			const response = await apiClient.listIdentityLinks();
			identityLinks = response.links;
		} catch (error) {
			console.warn('Failed to load identity links', error);
			identityLinks = [];
		} finally {
			identityLinksLoading = false;
		}
	}

	async function unlinkIdentity() {
		unlinkLoading = true;
		try {
			await apiClient.unlinkIdentity(unlinkingLinkId);
			toast.success(m.settings_unlink_success());
			unlinkDialogOpen = false;
			unlinkingLinkId = '';
			await loadIdentityLinks();
		} catch (error) {
			toast.error(getLocalizedError(error));
		} finally {
			unlinkLoading = false;
		}
	}

	async function loadSshKeys() {
		if (!authStore.hasPermission('AddUserSshKey:self')) return;
		try {
			const user = await apiClient.getCurrentUser();
			sshKeys = user?.sshPublicKeys ?? [];
		} catch (error) {
			console.warn('Failed to load SSH keys', error);
			sshKeys = [];
		}
	}

	async function addSshKey() {
		const userId = authStore.user?.id;
		if (!userId || !newSshKeyValue.trim()) return;
		try {
			await apiClient.addUserSshKey(userId.value, newSshKeyValue.trim(), newSshKeyComment.trim());
			toast.success(m.settings_ssh_key_added());
			addSshKeyOpen = false;
			newSshKeyValue = '';
			newSshKeyComment = '';
			await loadSshKeys();
		} catch (error) {
			toast.error(getLocalizedError(error));
		}
	}

	async function removeSshKey() {
		const userId = authStore.user?.id;
		if (!userId || !removingSshKeyId) return;
		try {
			await apiClient.removeUserSshKey(userId.value, removingSshKeyId);
			toast.success(m.settings_ssh_key_removed());
			removeSshKeyConfirmOpen = false;
			removingSshKeyId = '';
			await loadSshKeys();
		} catch (error) {
			toast.error(getLocalizedError(error));
		}
	}

	async function rebuildSearchIndex() {
		rebuildingSearchIndex = true;
		try {
			await apiClient.rebuildSearchIndex();
			toast.success(m.settings_rebuild_search_index_success());
		} catch (error) {
			toast.error(getLocalizedError(error));
		} finally {
			rebuildingSearchIndex = false;
		}
	}

	async function loadServerSettings() {
		if (!authStore.hasPermission('GetServerSettings')) return;
		try {
			const res = await apiClient.getServerSettings();
			if (res.settings) {
				globalUserProvisioning = res.settings.userProvisioningEnabled;
				globalSshAccessForAll = res.settings.sshAccessForAll;
			}
			settingsLoaded = true;
		} catch (error) {
			console.error('Failed to load server settings', error);
		}
	}

	async function updateGlobalSettings() {
		try {
			await apiClient.updateServerSettings(globalUserProvisioning, globalSshAccessForAll);
		} catch (error) {
			toast.error(getLocalizedError(error));
		}
	}

	async function handleResetVersion() {
		clearVersionCookie();

		if ('serviceWorker' in navigator) {
			const registrations = await navigator.serviceWorker.getRegistrations();
			for (const reg of registrations) {
				await reg.unregister();
			}
		}
		if ('caches' in window) {
			const cacheNames = await caches.keys();
			for (const name of cacheNames) {
				await caches.delete(name);
			}
		}
		window.location.reload();
	}

	async function handleLogout() {
		await authStore.logout();
		goto('/login');
	}

	async function replayTour() {
		await goto('/devices');
		startTour();
	}

	async function clearDataAndLogout() {
		await authStore.logout();
		configStore.serverUrl = '';
		if (typeof localStorage !== 'undefined') {
			localStorage.clear();
		}
		if (typeof indexedDB !== 'undefined') {
			indexedDB.deleteDatabase('cadestro-offline');
		}
		goto('/setup');
	}
</script>

{#snippet block(title: string, rows: Snippet, tone: 'plain' | 'danger' = 'plain')}
	<section class="space-y-2">
		<h2 class="px-0.5 text-xs font-semibold uppercase tracking-[0.12em] {tone === 'danger' ? 'text-crit' : 'text-faint'}">
			{title}
		</h2>
		<div class="divide-y divide-hair rounded-xl border border-hair bg-surface shadow-plate {tone === 'danger' ? 'border-crit/40' : ''}">
			{@render rows()}
		</div>
	</section>
{/snippet}

{#snippet row(label: string, description: string, control?: Snippet)}
	<div class="flex flex-col gap-3 px-4 py-3.5 sm:flex-row sm:items-center sm:justify-between sm:gap-6">
		<div class="min-w-0 space-y-0.5">
			<p class="text-sm font-medium">{label}</p>
			{#if description}<p class="text-sm text-muted-foreground">{description}</p>{/if}
		</div>
		{#if control}
			<div class="flex shrink-0 items-center gap-2 sm:justify-end">{@render control()}</div>
		{/if}
	</div>
{/snippet}

{#snippet mono(value: string)}
	<span class="truncate font-mono text-sm text-muted-foreground">{value}</span>
{/snippet}

<PageShell contentClass="space-y-8 pb-16">
	{#snippet header()}
		<div>
			<h1 class="text-2xl font-bold">{m.settings_title()}</h1>
			<p class="text-muted-foreground">{m.settings_subtitle()}</p>
		</div>
	{/snippet}

	<div class="max-w-3xl space-y-8">

		{#snippet accountRows()}
			{#snippet emailValue()}{@render mono(authStore.user?.email ?? m.common_unknown())}{/snippet}
			{@render row(m.settings_email(), m.settings_email_description(), emailValue)}

			{#snippet roleValue()}
				<div class="flex flex-wrap justify-end gap-1">
					{#each authStore.user?.roleGrants ?? [] as grant}
						{#if grant.role}
							<span class="inline-flex items-center rounded-full bg-accent-soft px-2.5 py-0.5 font-mono text-xs text-accent-ink">
								{grant.role.name}
							</span>
						{/if}
					{:else}
						<span class="text-sm text-muted-foreground">{m.settings_no_roles()}</span>
					{/each}
				</div>
			{/snippet}
			{@render row(m.settings_roles(), m.settings_roles_description(), roleValue)}
		{/snippet}
		{@render block(m.settings_account(), accountRows)}

		{#if canManageApiTokens}
			{#snippet apiTokenRows()}
				{#snippet createApiTokenControl()}
					<Button variant="outline" size="sm" onclick={openApiTokenDialog}>
						<Plus class="mr-2 h-4 w-4" />
						{m.settings_api_tokens_create()}
					</Button>
				{/snippet}
				{@render row(m.settings_api_tokens(), m.settings_api_tokens_description(), createApiTokenControl)}
				<div class="px-4 py-3">
					{#if apiTokensLoading}
						<p class="text-sm text-muted-foreground">{m.common_loading()}</p>
					{:else if apiTokens.length === 0}
						<p class="text-sm text-muted-foreground">{m.settings_api_tokens_no_tokens()}</p>
					{:else}
						<ul class="divide-y divide-hair rounded-lg border bg-sunken">
							{#each apiTokens as token}
								<li class="flex flex-col gap-2 px-3 py-3 sm:flex-row sm:items-center">
									<div class="min-w-0 flex-1">
										<p class="truncate font-mono text-sm font-medium">{token.name}</p>
										<p class="text-xs text-muted-foreground">
											{m.settings_api_tokens_created()}: {formatTimestampDateTime(token.createdAt)}
											· {m.settings_api_tokens_expires()}: {formatTimestampDateTime(token.expiresAt)}
										</p>
									</div>
									<span class="text-xs font-medium text-muted-foreground">{apiTokenStatusLabel(token)}</span>
									{#if apiTokenStatus(token) !== 'revoked'}
										<Button
											variant="ghost"
											size="sm"
											aria-label={m.settings_api_tokens_revoke()}
											onclick={() => confirmRevokeApiToken(token)}
										>
											{m.settings_api_tokens_revoke()}
										</Button>
									{/if}
								</li>
							{/each}
						</ul>
					{/if}
				</div>
			{/snippet}
			{@render block(m.settings_api_tokens(), apiTokenRows)}
		{/if}

		{#snippet appearanceRows()}
			{#snippet themeControl()}
				<Select.Root type="single" value={userPrefersMode.current} onValueChange={handleThemeChange}>
					<Select.Trigger class="w-[200px]" aria-label={m.settings_theme()}>
						{themes.find((t) => t.value === userPrefersMode.current)?.label() ?? m.settings_theme_system()}
					</Select.Trigger>
					<Select.Content>
						{#each themes as t}
							<Select.Item value={t.value}>{t.label()}</Select.Item>
						{/each}
					</Select.Content>
				</Select.Root>
			{/snippet}
			{@render row(m.settings_theme(), m.settings_theme_description(), themeControl)}

			{#snippet languageControl()}
				<Select.Root type="single" value={getLocale()} onValueChange={handleLanguageChange}>
					<Select.Trigger class="w-[200px]" aria-label={m.settings_language()}>
						{languages.find((l) => l.value === getLocale())?.label() ?? m.settings_language()}
					</Select.Trigger>
					<Select.Content>
						{#each languages as lang}
							<Select.Item value={lang.value}>{lang.label()}</Select.Item>
						{/each}
					</Select.Content>
				</Select.Root>
			{/snippet}
			{@render row(m.settings_language(), m.settings_language_description(), languageControl)}

			{#snippet tourControl()}
				<Button variant="outline" size="sm" onclick={replayTour}>
					{m.onboarding_restart_tour()}
				</Button>
			{/snippet}
			{@render row(m.onboarding_tour_label(), m.onboarding_welcome_lead(), tourControl)}
		{/snippet}
		{@render block(m.settings_appearance(), appearanceRows)}

		{#snippet sshRows()}
			{#snippet linuxUser()}
				{@render mono(authStore.user?.linuxUsername || m.settings_linux_username_unset())}
			{/snippet}
			{@render row(m.settings_linux_username(), m.settings_linux_username_description(), linuxUser)}

			{#if canManageSshKeys}
				{#snippet addKey()}
					<Button
						variant="outline"
						size="sm"
						onclick={() => {
							newSshKeyValue = '';
							newSshKeyComment = '';
							addSshKeyOpen = true;
						}}
					>
						<Plus class="mr-2 h-4 w-4" />
						{m.settings_ssh_add_key()}
					</Button>
				{/snippet}
				{@render row(m.settings_ssh_keys(), m.settings_ssh_keys_description(), addKey)}

				<div class="px-4 py-3">
					{#if sshKeys.length === 0}
						<p class="text-sm text-muted-foreground">{m.settings_ssh_no_keys()}</p>
					{:else}
						<ul class="divide-y divide-hair rounded-lg border bg-sunken">
							{#each sshKeys as key}
								<li class="flex items-center gap-3 px-3 py-2">
									<Key class="h-3.5 w-3.5 shrink-0 text-faint" />
									<div class="min-w-0 flex-1">
										<p class="truncate font-mono text-xs">{key.publicKey}</p>
										{#if key.comment}
											<p class="truncate text-xs text-muted-foreground">{key.comment}</p>
										{/if}
									</div>
									<Button
										variant="ghost"
										size="icon"
										aria-label={m.settings_ssh_remove_key_title()}
										onclick={() => {
											removingSshKeyId = (key.id?.value ?? '');
											removeSshKeyConfirmOpen = true;
										}}
									>
										<X class="h-4 w-4" />
									</Button>
								</li>
							{/each}
						</ul>
					{/if}
				</div>
			{/if}

			{@render row(m.settings_connected_accounts(), m.settings_connected_accounts_description())}
			<div class="px-4 py-3">
				{#if identityLinksLoading}
					<p class="text-sm text-muted-foreground">{m.common_loading()}</p>
				{:else if identityLinks.length === 0}
					<p class="text-sm text-muted-foreground">{m.settings_no_linked_accounts()}</p>
				{:else}
					<ul class="divide-y divide-hair rounded-lg border bg-sunken">
						{#each identityLinks as link}
							<li class="flex items-center gap-3 px-3 py-2">
								<div class="min-w-0 flex-1">
									<p class="truncate text-sm font-medium">{link.providerName}</p>
									<p class="truncate font-mono text-xs text-muted-foreground">{link.externalEmail}</p>
								</div>
								<Button
									variant="ghost"
									size="sm"
									onclick={() => {
										unlinkingLinkId = (link.id?.value ?? '');
										unlinkDialogOpen = true;
									}}
								>
									<Unlink class="mr-2 h-4 w-4" />
									{m.settings_unlink()}
								</Button>
							</li>
						{/each}
					</ul>
				{/if}
			</div>
		{/snippet}
		{@render block(m.settings_ssh_identity(), sshRows)}

		{#if canRebuildIndex}
			{#snippet searchRows()}
				{#snippet rebuild()}
					<Button variant="outline" onclick={rebuildSearchIndex} disabled={rebuildingSearchIndex}>
						<RefreshCw class="mr-2 h-4 w-4 {rebuildingSearchIndex ? 'animate-spin' : ''}" />
						{rebuildingSearchIndex ? m.settings_rebuilding_search_index() : m.settings_rebuild_search_index()}
					</Button>
				{/snippet}
				{@render row(
					m.settings_rebuild_search_index(),
					m.settings_rebuild_search_index_description(),
					rebuild
				)}
			{/snippet}
			{@render block(m.settings_search_index(), searchRows)}
		{/if}

		{#if canProvision}
			{#snippet provisioningRows()}
				{#snippet provisioningSwitch()}
					<Switch
						checked={globalUserProvisioning}
						aria-label={m.settings_user_provisioning()}
						onCheckedChange={(v) => {
							globalUserProvisioning = v;
							updateGlobalSettings();
						}}
					/>
				{/snippet}
				{@render row(
					m.settings_user_provisioning(),
					m.settings_user_provisioning_description(),
					provisioningSwitch
				)}

				{#snippet sshForAllSwitch()}
					<Switch
						checked={globalSshAccessForAll}
						aria-label={m.settings_ssh_access_for_all()}
						onCheckedChange={(v) => {
							globalSshAccessForAll = v;
							updateGlobalSettings();
						}}
					/>
				{/snippet}
				{@render row(
					m.settings_ssh_access_for_all(),
					m.settings_ssh_access_for_all_description(),
					sshForAllSwitch
				)}
			{/snippet}
			{@render block(m.settings_provisioning(), provisioningRows)}
		{/if}

		{#snippet serverRows()}
			{#snippet urlValue()}{@render mono(configStore.serverUrl)}{/snippet}
			{@render row(m.settings_server_url(), m.settings_server_url_description(), urlValue)}

			{#if serverVersion}
				{#snippet versionValue()}{@render mono(serverVersion)}{/snippet}
				{@render row(m.settings_server_version(), '', versionValue)}
			{/if}

			{#if pinnedVersion}
				{#snippet pinnedControl()}
					{@render mono(pinnedVersion ?? '')}
					<Button variant="outline" size="sm" onclick={handleResetVersion}>
						{m.settings_reset_version()}
					</Button>
				{/snippet}
				{@render row(m.settings_pinned_version(), m.settings_reset_version_description(), pinnedControl)}
			{/if}

			{#snippet changeServer()}
				<Button variant="outline" size="sm" onclick={() => goto('/setup')}>
					{m.settings_change_server()}
				</Button>
			{/snippet}
			{@render row(m.settings_change_server(), m.settings_change_server_description(), changeServer)}
		{/snippet}
		{@render block(m.settings_server_config(), serverRows)}

		{#snippet dangerRows()}
			{#snippet signOut()}
				<Button variant="outline" onclick={() => (logoutDialogOpen = true)}>
					<LogOut class="mr-2 h-4 w-4" />
					{m.settings_sign_out()}
				</Button>
			{/snippet}
			{@render row(m.settings_sign_out(), m.settings_sign_out_description(), signOut)}

			{#snippet clearData()}
				<Button variant="destructive" onclick={clearDataAndLogout}>
					{m.settings_clear_data_button()}
				</Button>
			{/snippet}
			{@render row(m.settings_clear_data(), m.settings_clear_data_description(), clearData)}
		{/snippet}
		{@render block(m.common_danger_zone(), dangerRows, 'danger')}

		<p class="text-center font-mono text-xs text-faint">{m.settings_version()} {__APP_VERSION__}</p>
	</div>
</PageShell>

<AlertDialog.Root bind:open={logoutDialogOpen}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{m.settings_sign_out()}</AlertDialog.Title>
			<AlertDialog.Description>
				{m.settings_sign_out_confirm()}
			</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>{m.common_cancel()}</AlertDialog.Cancel>
			<AlertDialog.Action onclick={handleLogout}>{m.settings_sign_out()}</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

<AlertDialog.Root bind:open={unlinkDialogOpen}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{m.settings_unlink()}</AlertDialog.Title>
			<AlertDialog.Description>
				{m.settings_unlink_confirm()}
			</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel onclick={() => { unlinkingLinkId = ''; }}>{m.common_cancel()}</AlertDialog.Cancel>
			<Button variant="destructive" onclick={unlinkIdentity} disabled={unlinkLoading}>
				{unlinkLoading ? m.common_loading() : m.settings_unlink()}
			</Button>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

<Dialog.Root bind:open={addSshKeyOpen}>
	<Dialog.Content>
		<Dialog.Header>
			<Dialog.Title>{m.settings_ssh_add_key_title()}</Dialog.Title>
			<Dialog.Description>{m.settings_ssh_add_key_description()}</Dialog.Description>
		</Dialog.Header>
		<form onsubmit={(e) => { e.preventDefault(); addSshKey(); }} class="space-y-4">
			<div class="space-y-2">
				<Label for="sshKey">{m.settings_ssh_public_key()}</Label>
				<Textarea id="sshKey" bind:value={newSshKeyValue} rows={3} class="font-mono text-sm" />
			</div>
			<div class="space-y-2">
				<Label for="sshComment">{m.settings_ssh_comment()}</Label>
				<Input id="sshComment" bind:value={newSshKeyComment} />
			</div>
			<Dialog.Footer>
				<Button type="button" variant="outline" onclick={() => (addSshKeyOpen = false)}>
					{m.common_cancel()}
				</Button>
				<Button type="submit" disabled={!newSshKeyValue.trim()}>
					{m.settings_ssh_add_key()}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>

<Dialog.Root bind:open={apiTokenDialogOpen} onOpenChange={handleApiTokenDialogOpen}>
	<Dialog.Content>
		<Dialog.Header>
			<Dialog.Title>{m.settings_api_tokens_create_title()}</Dialog.Title>
			<Dialog.Description>{m.settings_api_tokens_create_description()}</Dialog.Description>
		</Dialog.Header>
		{#if apiTokenValue}
			<div class="space-y-4">
				<div class="space-y-2">
					<Label for="apiTokenValue">{m.settings_api_tokens_value()}</Label>
					<div class="flex items-center gap-2">
						<Input id="apiTokenValue" value={apiTokenValue} readonly class="font-mono text-sm" />
						<Button type="button" variant="outline" size="icon" aria-label={m.settings_api_tokens_copied()} onclick={copyApiToken}>
							<Copy class="h-4 w-4" />
						</Button>
					</div>
					<p class="text-sm text-muted-foreground">{m.settings_api_tokens_value_warning()}</p>
				</div>
				<Dialog.Footer>
					<Button type="button" onclick={() => (apiTokenDialogOpen = false)}>{m.common_cancel()}</Button>
				</Dialog.Footer>
			</div>
		{:else}
			<form onsubmit={(event) => { event.preventDefault(); createApiToken(); }} class="space-y-4">
				<div class="space-y-2">
					<Label for="apiTokenName">{m.settings_api_tokens_name()}</Label>
					<Input id="apiTokenName" bind:value={apiTokenName} required />
				</div>
				<div class="space-y-2">
					<Label for="apiTokenExpiresAt">{m.settings_api_tokens_expiry()}</Label>
					<Input
						id="apiTokenExpiresAt"
						type="datetime-local"
						bind:value={apiTokenExpiresAt}
						min={new Date().toISOString().slice(0, 16)}
						required
					/>
				</div>
				<Dialog.Footer>
					<Button type="button" variant="outline" onclick={() => (apiTokenDialogOpen = false)}>{m.common_cancel()}</Button>
					<Button type="submit" disabled={apiTokenCreating || !apiTokenName.trim() || !apiTokenExpiresAt}>
						{apiTokenCreating ? m.common_loading() : m.settings_api_tokens_create()}
					</Button>
				</Dialog.Footer>
			</form>
		{/if}
	</Dialog.Content>
</Dialog.Root>

<AlertDialog.Root
	bind:open={revokeApiTokenConfirmOpen}
	onOpenChange={(open) => {
		revokeApiTokenConfirmOpen = open;
		if (!open) apiTokenToRevoke = null;
	}}
>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{m.settings_api_tokens_revoke_title()}</AlertDialog.Title>
			<AlertDialog.Description>{m.settings_api_tokens_revoke_confirm()}</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>{m.common_cancel()}</AlertDialog.Cancel>
			<AlertDialog.Action onclick={revokeApiToken} disabled={apiTokenRevoking}>
				{apiTokenRevoking ? m.common_loading() : m.settings_api_tokens_revoke()}
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

<AlertDialog.Root bind:open={removeSshKeyConfirmOpen}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{m.settings_ssh_remove_key_title()}</AlertDialog.Title>
			<AlertDialog.Description>{m.settings_ssh_remove_key_confirm()}</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel onclick={() => { removeSshKeyConfirmOpen = false; removingSshKeyId = ''; }}>{m.common_cancel()}</AlertDialog.Cancel>
			<AlertDialog.Action onclick={removeSshKey}>{m.common_delete()}</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
