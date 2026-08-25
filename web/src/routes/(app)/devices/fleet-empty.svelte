<script lang="ts">

	import type { Snippet } from 'svelte';
	import { MonitorOff } from '@lucide/svelte';

	let {
		title,
		hint,
		extra
	}: { title: string; hint: string; extra?: Snippet } = $props();
</script>

<div
	data-testid="fleet-empty"
	class="flex flex-col items-center justify-center gap-3 rounded-xl border border-hair bg-surface px-6 py-14 text-center"
>
	<MonitorOff class="h-10 w-10 text-faint" />
	<h3 class="font-semibold">{title}</h3>
	<p class="max-w-sm text-sm text-muted-foreground">{hint}</p>
	{#if extra}{@render extra()}{/if}
</div>
