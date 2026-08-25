<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$lib/navigation';
	import { configStore, authStore } from '$lib/sdk';
	import * as m from '$lib/paraglide/messages';

	onMount(() => {

		if (!configStore.isConfigured) {
			goto('/setup');
		} else if (authStore.isAuthenticated) {
			goto(authStore.isAdmin ? '/devices' : '/my-devices');
		} else {
			goto('/login');
		}
	});
</script>

<div class="flex h-screen items-center justify-center">
	<div class="text-muted-foreground">{m.common_loading()}</div>
</div>
