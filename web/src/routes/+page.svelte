<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';

	onMount(() => void goto('/devices', { replaceState: true }));
</script>

<main class="workspace-loading"><p>Opening workspace…</p></main>
