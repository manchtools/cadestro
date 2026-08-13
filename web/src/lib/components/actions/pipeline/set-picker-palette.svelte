<script lang="ts">
	// Round 2, Movement C's set-picker idiom: a bordered option row per action
	// set — marker, name, "N actions" meta — that expands to the set's own steps
	// so the operator can see what a set does before dropping it into a
	// definition. Same drag/keyboard contract as the action-type palette.
	import * as m from '$lib/paraglide/messages';
	import { ChevronRight, Search } from '@lucide/svelte';
	import type { Snippet } from 'svelte';
	import { STEP_DND_TYPE, type SetOption } from './types';

	let {
		title,
		options,
		oninsert,
		onpeek,
		peeked = {},
		emptyLabel,
		footer
	}: {
		title: string;
		options: SetOption[];
		oninsert: (id: string) => void;
		/** Asked to load a set's own steps the first time it is expanded. */
		onpeek?: (id: string) => void;
		/** Resolved step names per set id; absent means "not loaded yet". */
		peeked?: Record<string, string[] | undefined>;
		emptyLabel: string;
		footer?: Snippet;
	} = $props();

	let filter = $state('');
	let open = $state<string | null>(null);

	const shown = $derived.by(() => {
		const needle = filter.trim().toLocaleLowerCase();
		if (!needle) return options;
		return options.filter((o) => o.name.toLocaleLowerCase().includes(needle));
	});

	function toggle(id: string) {
		open = open === id ? null : id;
		if (open === id && !peeked[id]) onpeek?.(id);
	}
</script>

<div data-tour="builder-palette" class="self-start rounded-xl border bg-sunken p-2">
	<p class="px-1 pb-1.5 pt-0.5 font-mono text-[0.62rem] uppercase tracking-[0.1em] text-faint">
		{title}
	</p>

	<div class="relative mb-2">
		<Search class="pointer-events-none absolute left-2 top-1/2 h-3 w-3 -translate-y-1/2 text-faint" />
		<input
			type="search"
			bind:value={filter}
			placeholder={m.definition_detail_search_sets()}
			aria-label={m.definition_detail_search_sets()}
			class="w-full rounded-md border bg-surface py-1 pl-7 pr-2 text-xs outline-none focus-visible:border-ring"
		/>
	</div>

	<ul class="m-0 grid list-none gap-1.5 p-0">
		{#each shown as option (option.id)}
			<li class="overflow-hidden rounded-[9px] border bg-surface">
				<div class="flex items-center gap-1.5 px-1.5 py-1.5 text-xs">
					<button
						type="button"
						aria-expanded={open === option.id}
						aria-label={m.definition_detail_builder_peek()}
						class="grid h-4 w-4 shrink-0 place-items-center text-faint"
						onclick={() => toggle(option.id)}
					>
						<ChevronRight class="h-3 w-3 transition-transform {open === option.id ? 'rotate-90' : ''}" />
					</button>
					<button
						type="button"
						draggable="true"
						data-palette-entry={option.id}
						class="min-w-0 flex-1 cursor-grab text-left"
						ondragstart={(e) => e.dataTransfer?.setData(STEP_DND_TYPE, option.id)}
						onclick={() => oninsert(option.id)}
					>
						<span class="block truncate font-medium">{option.name}</span>
					</button>
					<span class="shrink-0 font-mono text-[0.6rem] text-faint">
						{m.action_sets_count({ count: option.memberCount })}
					</span>
				</div>
				{#if open === option.id}
					<div class="grid gap-0.5 border-t bg-surface px-2 py-1.5 pl-7 font-mono text-[0.66rem] text-muted-foreground">
						{#each peeked[option.id] ?? [] as step, i (i)}
							<span class="truncate">{i + 1} · {step}</span>
						{:else}
							<span class="text-faint">{m.common_loading()}</span>
						{/each}
					</div>
				{/if}
			</li>
		{:else}
			<li class="px-1 py-2 text-xs text-muted-foreground">{emptyLabel}</li>
		{/each}
	</ul>

	{#if footer}
		<div class="mt-2 border-t pt-2">{@render footer()}</div>
	{/if}
</div>
