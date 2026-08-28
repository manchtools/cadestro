<script lang="ts">
	import '../app.css';

	let { children } = $props();
</script>

<svelte:head>
	<title>Cadestro</title>
</svelte:head>

{@render children()}
