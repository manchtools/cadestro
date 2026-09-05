<script lang="ts">
 import { ModeWatcher } from 'mode-watcher';
 import { Toaster } from '$lib/components/ui/sonner';
	import '../app.css';

	let { children } = $props();
</script>

<svelte:head>
	<title>Cadestro</title>
</svelte:head>

{@render children()}

<ModeWatcher />
<Toaster />
