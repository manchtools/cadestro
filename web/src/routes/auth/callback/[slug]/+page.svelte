<script lang="ts">
 import { Button } from '$lib/components/ui/button';
 import * as Card from '$lib/components/ui/card';
 import { RefreshCw, AlertTriangle, ArrowLeft } from '@lucide/svelte';
 import * as m from '$lib/paraglide/messages';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { timestampMs } from '@bufbuild/protobuf/wkt';
	import { publicAPI, errorMessage } from '$lib/api';
	import { writeSession } from '$lib/session';

	let error = $state('');

	onMount(async () => {
		const code = page.url.searchParams.get('code') ?? '';
		const state = page.url.searchParams.get('state') ?? '';
		try {
			const response = await publicAPI.sSOCallback({ slug: page.params.slug, code, state });
			if (!response.expiresAt) throw new Error('The login response has no expiration time');
			writeSession({ accessToken: response.accessToken, refreshToken: response.refreshToken, expiresAt: timestampMs(response.expiresAt) });
			await goto('/');
		} catch (cause) {
			error = errorMessage(cause);
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
				<Card.Description role="alert">{error}</Card.Description>
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
