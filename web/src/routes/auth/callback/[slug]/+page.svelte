<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { goto } from '$lib/navigation';
	import { apiClient } from '$lib/sdk';
	import { getLocalizedError } from '$lib/errors';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { RefreshCw, AlertTriangle, ArrowLeft } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';

	let error = $state('');

	onMount(async () => {
		const slug = page.params.slug ?? '';
		const code = page.url.searchParams.get('code') ?? '';
		const state = page.url.searchParams.get('state') ?? '';
		if (!slug || !code || !state) {
			error = m.sso_callback_error_description();
			return;
		}

		try {
			await apiClient.ssoCallback(slug, code, state);
			goto('/');
		} catch (cause) {
			error = getLocalizedError(cause);
		}
	});
</script>

<div class="flex min-h-screen items-center justify-center bg-background p-4">
	<Card.Root class="w-full max-w-md">
		{#if error}
			<Card.Header class="space-y-1">
				<div class="flex items-center gap-2">
					<AlertTriangle class="h-6 w-6 text-destructive" />
					<Card.Title class="text-2xl">{m.sso_callback_error()}</Card.Title>
				</div>
				<Card.Description>{error}</Card.Description>
			</Card.Header>
			<Card.Content>
				<Button variant="outline" class="w-full" onclick={() => goto('/login')}>
					<ArrowLeft class="mr-2 h-4 w-4" /> {m.sso_callback_back_to_login()}
				</Button>
			</Card.Content>
		{:else}
			<Card.Header><Card.Title class="text-2xl">{m.sso_callback_title()}</Card.Title></Card.Header>
			<Card.Content class="flex items-center justify-center py-8">
				<RefreshCw class="h-8 w-8 animate-spin text-muted-foreground" />
			</Card.Content>
		{/if}
	</Card.Root>
</div>
