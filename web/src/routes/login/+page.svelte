<script lang="ts">
 import { Button } from '$lib/components/ui/button';
 import { Globe } from '@lucide/svelte';
 import * as m from '$lib/paraglide/messages';
	import { onMount } from 'svelte';
	import { publicAPI, errorMessage } from '$lib/api';

	let providers = $state<Array<{ slug: string; name: string }>>([]);
	let loading = $state(true);
	let error = $state('');

	onMount(async () => {
		try {
			providers = (await publicAPI.listAuthMethods({})).providers;
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			loading = false;
		}
	});

	async function signIn(slug: string) {
		error = '';
		try {
			const redirectUrl = `${window.location.origin}/auth/callback/${encodeURIComponent(slug)}`;
			const response = await publicAPI.getSSOLoginURL({ slug, redirectUrl });
			const target = new URL(response.loginUrl);
			if (target.protocol !== 'https:' && !(target.protocol === 'http:' && target.hostname === 'localhost')) {
				throw new Error('The identity provider returned an unsafe URL');
			}
			window.location.assign(target.href);
		} catch (cause) {
			error = errorMessage(cause);
		}
	}
</script>

<div class="flex min-h-screen items-center justify-center bg-page p-4">
	<div class="w-full max-w-md rounded-[14px] border bg-surface shadow-plate">
		<div class="space-y-1 px-6 pb-4 pt-6">
			<div class="flex items-center justify-between">
				<h1 class="text-2xl font-semibold tracking-tight">{m.login_title()}</h1>

			</div>
			<p class="text-sm text-muted-foreground">{m.login_description()}</p>
		</div>
		<div class="space-y-4 px-6 pb-6">


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

							onclick={() => signIn(provider.slug)}
						>
							<Globe class="mr-2 h-4 w-4" />
							{provider.name}
						</Button>
					{/each}
				</div>
			{/if}

			{#if error && providers.length > 0}
				<p role="alert" class="text-sm text-crit">{error}</p>
			{/if}
		</div>
	</div>
</div>
