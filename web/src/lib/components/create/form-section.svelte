<script lang="ts">
	// A titled group of fields inside a create/edit plate.
	//
	// Every form was repeating the same ad-hoc trio — a top rule, a small bold
	// heading, sometimes a line of help — with slightly different spacing each
	// time. One component makes the rhythm identical everywhere, which is the
	// whole point of a form language: the operator learns the shape once.
	//
	// The separator is skipped on the first section (`lead`), because a rule
	// immediately under the plate header is a double line.
	import type { Snippet } from 'svelte';

	let {
		title,
		description,
		lead = false,
		children
	}: {
		title: string;
		/** One line on what this group is for. Sits under the title, never after
		 *  the fields — guidance the operator reads too late is not guidance. */
		description?: string;
		/** First section in a plate: no top rule. */
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
