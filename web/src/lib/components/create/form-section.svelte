<script lang="ts">

	import type { Snippet } from 'svelte';

	let {
		title,
		description,
		lead = false,
		children
	}: {
		title: string;

		description?: string;

		lead?: boolean;
		children: Snippet;
	} = $props();
</script>

<section class="space-y-3 {lead ? '' : 'border-t pt-4'}">
	<div class="space-y-0.5">
		<h2 class="text-sm font-medium">{title}</h2>
		{#if description}
			<p class="text-xs text-muted-foreground">{description}</p>
		{/if}
	</div>
	{@render children()}
</section>
