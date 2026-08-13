<script lang="ts">
	import { base } from '$app/paths';
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { goto } from '$lib/navigation';
	import { configStore, authStore, apiClient } from '$lib/sdk';
	import { checkAndSwitchVersion } from '$lib/version';
	import { getLocalizedError } from '$lib/errors';
	import { Button } from '$lib/components/ui/button';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Label } from '$lib/components/ui/label';
	import { Settings, Globe } from '@lucide/svelte';
	import type { IdentityProviderType } from '$sdk/powermanage/v1/common_pb';
	import * as m from '$lib/paraglide/messages';

	type SSOProvider = { slug: string; name: string; providerType: IdentityProviderType };

	let rememberMe = $state(false);
	let providers = $state<SSOProvider[]>([]);
	let loading = $state(true);
	let starting = $state('');
	let error = $state('');

	function redirectPath(): string {
		const raw = page.url.searchParams.get('redirect');
		const normalized = raw?.replace(/\\/g, '/');
		if (normalized?.startsWith('/') && !normalized.startsWith('//')) return normalized;
		return authStore.isAdmin ? '/devices' : '/my-devices';
	}

	onMount(async () => {
		if (!configStore.isConfigured) {
			goto('/setup');
			return;
		}
		if (authStore.isAuthenticated) {
			goto(redirectPath());
			return;
		}

		await checkAndSwitchVersion(configStore.serverUrl);
		try {
			const response = await apiClient.listAuthMethods();
			providers = response.providers.map((provider) => ({
				slug: provider.slug,
				name: provider.name,
				providerType: provider.providerType
			}));
		} catch (cause) {
			error = getLocalizedError(cause);
		} finally {
			loading = false;
		}
	});

	async function signIn(provider: SSOProvider) {
		if (starting) return;
		starting = provider.slug;
		error = '';
		try {
			authStore.setPersist(rememberMe);
			const redirectUrl = `${window.location.origin}${base}/auth/callback/${encodeURIComponent(provider.slug)}`;
			const response = await apiClient.getSSOLoginURL(provider.slug, redirectUrl);
			const url = new URL(response.loginUrl);
			const localHTTP = url.protocol === 'http:' && ['localhost', '127.0.0.1', '::1'].includes(url.hostname);
			if (url.protocol !== 'https:' && !localHTTP) throw new Error('Identity provider returned an unsafe login URL');
			window.location.assign(url.href);
		} catch (cause) {
			error = getLocalizedError(cause);
			starting = '';
		}
	}
</script>

<!-- Re-skin only: a centered plate on the page plate, the first identity
     provider as the accent CTA, and the server it will talk to in mono at the
     foot. No sign-in logic is expressed here. -->
<div class="flex min-h-screen items-center justify-center bg-page p-4">
	<div class="w-full max-w-md rounded-[14px] border bg-surface shadow-plate">
		<div class="space-y-1 px-6 pb-4 pt-6">
			<div class="flex items-center justify-between">
				<h1 class="text-2xl font-semibold tracking-tight">{m.login_title()}</h1>
				<Button variant="ghost" size="icon" onclick={() => goto('/setup')} aria-label={m.setup_title()}>
					<Settings class="h-4 w-4" />
				</Button>
			</div>
			<p class="text-sm text-muted-foreground">{m.login_description()}</p>
		</div>
		<div class="space-y-4 px-6 pb-6">
			<div class="flex items-center gap-2">
				<Checkbox id="remember-me" bind:checked={rememberMe} />
				<Label for="remember-me" class="cursor-pointer text-sm font-normal">{m.login_remember_me()}</Label>
			</div>

			{#if loading}
				<p class="text-sm text-muted-foreground">{m.login_sso_loading()}</p>
			{:else if providers.length === 0}
				<p class="text-sm text-muted-foreground">{error || m.login_no_identity_providers()}</p>
			{:else}
				<div class="space-y-2">
					{#each providers as provider, i (provider.slug)}
						<Button
							variant={i === 0 ? 'default' : 'outline'}
							class="w-full"
							disabled={!!starting}
							onclick={() => signIn(provider)}
						>
							<Globe class="mr-2 h-4 w-4" />
							{starting === provider.slug ? m.login_sso_loading() : provider.name}
						</Button>
					{/each}
				</div>
			{/if}

			{#if error && providers.length > 0}
				<p role="alert" class="text-sm text-crit">{error}</p>
			{/if}
		</div>
		<div class="border-t px-6 py-3 text-center font-mono text-xs text-faint">
			{m.login_server_prefix({ serverUrl: configStore.serverUrl })}
		</div>
	</div>
</div>
