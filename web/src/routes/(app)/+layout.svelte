<script lang="ts">
	import { page } from '$app/state';
 import AppShell from '$lib/components/workspace/app-shell.svelte';
	let { children } = $props();
</script>

<AppShell>{#key page.url.pathname}{@render children()}{/key}</AppShell>
