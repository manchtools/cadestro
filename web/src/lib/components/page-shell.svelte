<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		header,
		children,
		contentClass = 'space-y-4'
	}: {
		header?: Snippet;
		children: Snippet;
		contentClass?: string;
	} = $props();
</script>

<div class="flex flex-1 flex-col min-h-0">
	{#if header}
		<div class="shrink-0 space-y-6 px-4 pt-4 pb-6 md:px-6 md:pt-6">
			{@render header()}
		</div>
	{/if}
	<div class="flex-1 overflow-auto px-4 pb-4 md:px-6 md:pb-6 {contentClass}">
		{@render children()}
	</div>
</div>
