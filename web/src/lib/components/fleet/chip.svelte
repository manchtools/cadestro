<script lang="ts">

	import type { Snippet } from 'svelte';
	import { TONE_SOFT, type FleetTone } from './tone';

	let {
		tone = 'idle',
		label,
		children
	}: { tone?: FleetTone; label?: string; children?: Snippet } = $props();
</script>

<span
	data-testid="fleet-chip"
	data-tone={tone}
	class="inline-flex items-center gap-1.5 whitespace-nowrap rounded-full border border-transparent px-2 py-0.5 font-mono text-xs {TONE_SOFT[tone]}"
>
	{#if children}{@render children()}{:else}{label}{/if}
</span>
