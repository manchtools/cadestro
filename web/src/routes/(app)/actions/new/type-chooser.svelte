<script lang="ts">

	import { Search } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { getActionTypeInfoByValue } from '$lib/components/actions/action-type';
	import { TILE_VALUES } from './type-tiles';

	let { onchoose }: { onchoose: (typeValue: string) => void } = $props();

	let filter = $state('');

	const tiles = $derived(
		TILE_VALUES.map((value) => ({ value, ...getActionTypeInfoByValue(value) }))
	);

	const shown = $derived.by(() => {
		const needle = filter.trim().toLocaleLowerCase();
		if (!needle) return tiles;
		return tiles.filter(
			(t) =>
				t.label.toLocaleLowerCase().includes(needle) ||
				t.description.toLocaleLowerCase().includes(needle) ||
				t.value.toLocaleLowerCase().includes(needle)
		);
	});
</script>

<div class="mx-auto flex w-full max-w-5xl flex-col gap-4" data-testid="action-type-chooser">
	<div class="space-y-1">
		<h1 class="text-2xl font-semibold tracking-tight">{m.actions_new_choose_heading()}</h1>
		<p class="text-sm text-muted-foreground">{m.actions_new_choose_subtitle()}</p>
	</div>

	<div class="relative">
		<Search
			class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-faint"
		/>
		<input
			type="search"
			bind:value={filter}
			data-testid="action-type-filter"
			placeholder={m.actions_new_search_placeholder()}
			aria-label={m.actions_new_search_placeholder()}
			class="w-full rounded-xl border bg-surface py-2.5 pl-9 pr-3 text-sm shadow-plate outline-none focus-visible:border-ring"
		/>
	</div>

	<ul class="m-0 grid list-none gap-2.5 p-0 sm:grid-cols-2 lg:grid-cols-3">
		{#each shown as tile (tile.value)}
			{@const Icon = tile.icon}
			<li>
				<button
					type="button"
					data-testid="action-type-tile"
					data-type-value={tile.value}
					onclick={() => onchoose(tile.value)}
					class="flex h-full w-full items-start gap-2.5 rounded-xl border bg-surface p-3 text-left shadow-plate transition-colors hover:border-primary focus-visible:border-ring focus-visible:outline-none"
				>
					<span
						class="grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-accent-soft text-accent-ink"
					>
						<Icon class="h-4 w-4" />
					</span>
					<span class="min-w-0 flex-1">
						<span class="block truncate text-sm font-semibold">{tile.label}</span>
						<span class="mt-0.5 line-clamp-2 block text-xs text-muted-foreground">
							{tile.description}
						</span>
					</span>
				</button>
			</li>
		{:else}
			<li class="col-span-full rounded-xl border border-dashed bg-sunken p-6 text-center text-sm text-muted-foreground">
				{m.common_no_results_search()}
			</li>
		{/each}
	</ul>
</div>
