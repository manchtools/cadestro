<script lang="ts">
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

<main class="centered">
	<section class="card login">
		<h1>{error ? 'Sign-in failed' : 'Signing in…'}</h1>
		{#if error}
			<p class="error" role="alert">{error}</p>
			<a class="button" href="/login">Back to sign in</a>
		{/if}
	</section>
</main>
