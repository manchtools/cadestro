<script lang="ts">
	import { cn } from '$lib/utils';

	interface Props {
		error?: string | undefined;
		class?: string;
	}

	let { error, class: className }: Props = $props();
</script>

{#if error}
	<p class={cn('text-destructive text-sm font-medium', className)} role="alert">
		{error}
	</p>
{/if}
