<script lang="ts">

	import * as m from '$lib/paraglide/messages';
	import { Search } from '@lucide/svelte';
	import type { Snippet } from 'svelte';
	import { STEP_DND_TYPE, type PaletteEntry } from './types';

	let {
		title,
		entries,
		oninsert,
		existing,
		oninsertExisting,
		existingEmptyLabel,
		searchPlaceholder = m.common_search(),
		emptyLabel,
		footer
	}: {
		title: string;

		entries: PaletteEntry[];
		oninsert: (id: string) => void;

		existing?: PaletteEntry[];
		oninsertExisting?: (id: string) => void;
		existingEmptyLabel?: string;
		searchPlaceholder?: string;
		emptyLabel: string;
		footer?: Snippet;
	} = $props();

	let tab = $state<'existing' | 'new'>('existing');
	let filter = $state('');

	const active = $derived(tab === 'existing' && existing ? existing : entries);
	const shown = $derived.by(() => {
		const needle = filter.trim().toLocaleLowerCase();
		if (!needle) return active;
		return active.filter(
			(e) =>
				e.label.toLocaleLowerCase().includes(needle) ||
				(e.hint ?? '').toLocaleLowerCase().includes(needle)
		);
	});
	const insert = $derived(
		tab === 'existing' && oninsertExisting ? oninsertExisting : oninsert
	);
	const nothing = $derived(
		tab === 'existing' ? (existingEmptyLabel ?? emptyLabel) : emptyLabel
	);
</script>

<div
	data-tour="builder-palette"
	class="min-w-0 self-start rounded-xl border bg-sunken p-2"
>
	<p class="px-1 pb-1.5 pt-0.5 font-mono text-[0.62rem] uppercase tracking-[0.1em] text-faint">
		{title}
	</p>

	{#if existing}
		<div
			role="tablist"
			aria-label={title}
			class="mb-2 inline-flex w-full overflow-hidden rounded-lg border text-[0.7rem]"
		>
			{#each [{ id: 'existing', label: m.step_palette_tab_existing() }, { id: 'new', label: m.step_palette_tab_new() }] as t (t.id)}
				<button
					type="button"
					role="tab"
					data-palette-tab={t.id}
					aria-selected={tab === t.id}
					onclick={() => {
						tab = t.id as 'existing' | 'new';
						filter = '';
					}}
					class="flex-1 px-2 py-1 font-medium {tab === t.id
						? 'bg-accent-soft text-accent-ink'
						: 'text-muted-foreground hover:bg-accent/50'}"
				>
					{t.label}
				</button>
			{/each}
		</div>
	{/if}

	<div class="relative mb-2">
		<Search class="pointer-events-none absolute left-2 top-1/2 h-3 w-3 -translate-y-1/2 text-faint" />
		<input
			type="search"
			bind:value={filter}
			placeholder={searchPlaceholder}
			aria-label={searchPlaceholder}
			class="w-full rounded-md border bg-surface py-1 pl-7 pr-2 text-xs outline-none focus-visible:border-ring"
		/>
	</div>

	<ul class="m-0 grid max-h-[32rem] list-none gap-1.5 overflow-y-auto p-0">
		{#each shown as entry (entry.id)}
			{@const Icon = entry.icon}
			<li>
				<button
					type="button"
					draggable="true"
					data-palette-entry={entry.id}
					class="flex w-full cursor-grab items-center gap-2 rounded-lg border bg-surface px-1.5 py-1.5 text-left text-xs hover:border-border-strong focus-visible:border-ring"
					ondragstart={(e) => e.dataTransfer?.setData(STEP_DND_TYPE, entry.id)}
					onclick={() => insert(entry.id)}
				>
					<span
						class="grid h-5 w-5 shrink-0 place-items-center rounded-[5px] bg-accent-soft text-accent-ink"
					>
						{#if Icon}<Icon class="h-3 w-3" />{/if}
					</span>
					<span class="min-w-0 flex-1">
						<span class="block truncate font-medium">{entry.label}</span>
						{#if entry.hint}
							<span class="block truncate font-mono text-[0.62rem] text-faint">{entry.hint}</span>
						{/if}
					</span>
					<span aria-hidden="true" class="font-mono text-sm tracking-[-0.1em] text-faint">⣿</span>
				</button>
			</li>
		{:else}
			<li class="px-1 py-2 text-xs text-muted-foreground">{nothing}</li>
		{/each}
	</ul>

	{#if footer}
		<div class="mt-2 border-t pt-2">{@render footer()}</div>
	{/if}
</div>
