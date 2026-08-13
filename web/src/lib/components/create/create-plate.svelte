<script lang="ts">
	// The shared plate every pill-committed create route is drawn on.
	//
	// Deliberately button-free: Save, Cancel and Stash belong to the context pill,
	// so a create surface that rendered its own footer would re-introduce exactly
	// the modal shape these routes replace. The plate only frames the fields.
	import type { Component, Snippet } from 'svelte';

	let {
		icon,
		title,
		description,
		hint,
		testid,
		children
	}: {
		/** Lucide component for the header tile. */
		icon: Component<{ class?: string }>;
		title: string;
		description?: string;
		/** Standing note about the whole form — what this thing is, what it cannot
		 *  do yet, where the rest is configured. It renders directly under the
		 *  header, BEFORE the fields: a caveat that only appears after the operator
		 *  has filled everything in has already wasted their time. */
		hint?: string;
		/** Stable hook for the route's own tests. */
		testid: string;
		children: Snippet;
	} = $props();

	const Icon = $derived(icon);
</script>

<div class="mx-auto flex w-full max-w-3xl flex-col gap-3" data-testid={testid}>
	<div class="rounded-xl border border-hair bg-surface shadow-plate">
		<div class="flex items-center gap-2.5 border-b px-3 py-2.5">
			<span
				class="grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-accent-soft text-accent-ink"
			>
				<Icon class="h-4 w-4" />
			</span>
			<div class="min-w-0 flex-1">
				<h1 class="truncate text-sm font-semibold">{title}</h1>
				{#if description}
					<p class="truncate text-xs text-muted-foreground">{description}</p>
				{/if}
			</div>
		</div>
		{#if hint}
			<p
				data-testid="{testid}-hint"
				class="border-b bg-muted/40 px-3 py-2 text-xs text-muted-foreground"
			>
				{hint}
			</p>
		{/if}
		<div class="space-y-4 p-3">
			{@render children()}
		</div>
	</div>
</div>
