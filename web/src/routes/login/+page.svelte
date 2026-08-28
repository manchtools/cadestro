<script lang="ts">
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

<main class="centered">
	<section class="card login">
		<p class="eyebrow">Linux device management</p>
		<h1>Cadestro</h1>
		<p>Sign in with your administrator identity.</p>
		{#if loading}
			<p>Loading identity providers…</p>
		{:else if providers.length === 0}
			<p class="error">{error || 'No identity provider is configured.'}</p>
		{:else}
			{#each providers as provider (provider.slug)}
				<button class="primary wide" onclick={() => signIn(provider.slug)}>Continue with {provider.name}</button>
			{/each}
		{/if}
		{#if error && providers.length > 0}<p class="error" role="alert">{error}</p>{/if}
	</section>
</main>
