<script lang="ts">
 import { onMount } from 'svelte';
 import { goto } from '$app/navigation';
 import { AppWindow, PencilLine, Search, ListFilter, X } from '@lucide/svelte';
 import * as m from '$lib/paraglide/messages';
 import type { PillEntry, PillGroup } from '$lib/shell/nav';
 import { activePageSearch } from '$lib/shell/page-search.svelte';
 import { shell, restorePanel, restoreDraft } from '$lib/shell/shell.svelte';
 let { open = $bindable(false), sections, overflow }: { open?: boolean; sections: PillEntry[]; overflow: PillGroup[] } = $props();
 type Row = { id: string; kind: 'page' } | { id: string; kind: 'clear' } | { id: string; kind: 'section'; title: string; href: string } | { id: string; kind: 'panel' | 'draft'; title: string; sub: string; ref: string };
 const scoped = $derived(activePageSearch());
 const ring = $derived([...(scoped ? [{ key: 'page', scope: 'page', page: true, label: scoped.label }] : []), { key: 'shell', scope: 'shell', page: false, label: m.search_group_shell }]);
 let facet = $state(0);
 const pageMode = $derived(ring[facet]?.page ?? false);
 let query = $state('');
 let sel = $state(0);
 let input = $state<HTMLInputElement>();
 const entries = $derived([...sections, ...overflow.flatMap(group => group.items)]);
 const sectionIcon = $derived(new Map(entries.map(entry => [entry.href, entry.icon])));
 const shellIcon = { panel: AppWindow, draft: PencilLine };
 const groups = $derived.by(() => {
  if (pageMode) return [{ key: 'page', heading: scoped?.label(), rows: [{ id: 'page-filter', kind: 'page' } as Row, ...(query ? [{ id: 'page-clear', kind: 'clear' } as Row] : [])] }];
  const needle = query.trim().toLocaleLowerCase();
  const navigation: Row[] = entries.filter(entry => entry.label().toLocaleLowerCase().includes(needle)).map(entry => ({ id: entry.href, kind: 'section', title: entry.label(), href: entry.href }));
  const windows: Row[] = shell.panels.filter(panel => panel.title.toLocaleLowerCase().includes(needle)).map(panel => ({ id: `panel-${panel.id}`, kind: 'panel', title: panel.title, sub: m.shell_windows(), ref: panel.id }));
  const drafts: Row[] = shell.drafts.filter(draft => draft.title.toLocaleLowerCase().includes(needle)).map(draft => ({ id: `draft-${draft.id}`, kind: 'draft', title: draft.title, sub: draft.subtitle ?? '', ref: draft.id }));
  return [{ key: 'navigation', heading: m.search_group_goto(), rows: navigation }, { key: 'shell', heading: m.search_group_shell(), rows: [...windows, ...drafts] }].filter(group => group.rows.length);
 });
 const flat = $derived(groups.flatMap(group => group.rows));
 const activeIndex = $derived(Math.min(sel, Math.max(0, flat.length - 1)));
 onMount(() => { query = scoped?.query ?? ''; input?.focus(); });
 function onInput(value: string) { query = value; sel = 0; if (pageMode) scoped?.setQuery(value); }
 function setFacet(index: number) { facet = index; query = pageMode ? scoped?.query ?? '' : ''; sel = 0; input?.focus(); }
 function activate(row: Row) {
  if (row.kind === 'clear') { scoped?.clear(); query = ''; input?.focus(); return; }
  open = false;
  if (row.kind === 'section') void goto(row.href);
  else if (row.kind === 'panel') restorePanel(row.ref);
  else if (row.kind === 'draft') { const route = restoreDraft(row.ref); if (route) void goto(route); }
 }
 function onkeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') { event.preventDefault(); open = false; }
  else if (event.key === 'ArrowDown' || event.key === 'ArrowUp') { event.preventDefault(); sel = (activeIndex + (event.key === 'ArrowDown' ? 1 : -1) + flat.length) % Math.max(1, flat.length); }
  else if (event.key === 'Enter' && flat[activeIndex]) { event.preventDefault(); activate(flat[activeIndex]); }
  else if (event.key === 'Tab') { event.preventDefault(); setFacet((facet + 1) % ring.length); }
 }
</script>
{#if open}

	<div
		role="search"
		aria-label={m.search_dialog_label()}
		data-testid="global-search"
		data-page-mode={pageMode ? 'true' : 'false'}
		class="flex max-h-[62vh] w-[min(36rem,calc(100vw-3rem))] flex-col overflow-hidden text-foreground"
	>

			<div class="flex items-center gap-2 border-b px-3.5 py-3">
				<Search class="h-4 w-4 shrink-0 text-muted-foreground" />
				<input
					bind:this={input}
					data-tour="palette-input"
					role="combobox"
					aria-expanded="true"
					aria-controls="palette-listbox"
					aria-activedescendant={flat[activeIndex]?.id}
					aria-label={m.search_dialog_label()}
					autocomplete="off"
					spellcheck="false"
					class="flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
					placeholder={pageMode && scoped ? m.search_page_placeholder({ scope: scoped.label() }) : m.search_placeholder()}
					value={query}
					oninput={(e) => onInput(e.currentTarget.value)}
					{onkeydown}
				/>
				<kbd class="rounded border border-border-strong bg-sunken px-1.5 py-0.5 font-mono text-[10px] text-faint">esc</kbd>
			</div>

			<div
				class="flex flex-wrap gap-1.5 border-b px-3 py-2"
				data-tour="palette-facets"
				data-testid="palette-facets"
				role="group"
				aria-label={m.search_facets_label()}
			>
				{#each ring as f, i (f.key)}
					<button
						type="button"
						tabindex="-1"
						data-testid="palette-facet"
						data-scope={f.scope}
						data-page-facet={f.page ? 'true' : undefined}
						aria-pressed={i === facet}
						onclick={() => setFacet(i)}
						class="rounded-full border px-2 py-0.5 font-mono text-[10px] {i === facet
							? 'border-accent-ink text-accent-ink'
							: 'border-border text-muted-foreground hover:text-foreground'} {f.page
							? 'border-dashed'
							: ''}"
					>
						{f.label()}
					</button>
				{/each}
			</div>

			<ul id="palette-listbox" role="listbox" aria-label={m.search_results_label()} class="min-h-0 flex-1 overflow-y-auto p-1.5">
				{#each groups as g (g.key)}
					<li role="presentation">
						{#if g.heading}
							<div
								id="palette-head-{g.key}"
								class="px-2 pb-1 pt-1.5 font-mono text-[10px] uppercase tracking-[0.12em] text-faint"
							>
								{g.heading}
							</div>
						{/if}
						<ul role="group" aria-labelledby={g.heading ? `palette-head-${g.key}` : undefined} class="contents">
							{#each g.rows as row (row.id)}
								{@const i = flat.indexOf(row)}

								<li
									id={row.id}
									role="option"
									aria-selected={i === activeIndex}
									data-testid="palette-row"
									data-kind={row.kind}
									onmouseenter={() => (sel = i)}
									onmousedown={(e) => e.preventDefault()}
									onkeydown={(e) => (e.key === 'Enter' || e.key === ' ') && activate(row)}
									onclick={() => activate(row)}
									class="flex cursor-pointer items-center gap-2 rounded-lg px-2 py-1.5 text-sm {i === activeIndex
										? 'bg-accent text-accent-foreground'
										: ''}"
								>
									{#if row.kind === 'page'}
										<ListFilter class="h-3.5 w-3.5 shrink-0 text-accent-ink" />
										<span class="truncate" data-testid="palette-page-row">
											{m.search_page_row({ scope: scoped?.label() ?? '' })}
										</span>
										<span class="ml-auto shrink-0 truncate pl-3 text-xs text-muted-foreground">
											{m.search_page_row_hint()}
										</span>
									{:else if row.kind === 'clear'}
										<X class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
										<span class="truncate" data-testid="palette-page-clear">{m.search_page_clear()}</span>
									{:else if row.kind === 'section'}
										{@const SectionIcon = sectionIcon.get(row.href)}
										{#if SectionIcon}<SectionIcon class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />{/if}
										<span class="truncate">{row.title}</span>
										<span class="ml-auto shrink-0 truncate pl-3 text-xs text-muted-foreground">{m.shell_row_section()}</span>
									{:else}
										{@const Icon = shellIcon[row.kind]}
										<Icon class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
										<span class="truncate">{row.title}</span>
										<span class="ml-auto shrink-0 truncate pl-3 text-xs text-muted-foreground">{row.sub}</span>
									{/if}
								</li>
							{/each}
						</ul>
					</li>
				{:else}
					<li class="px-2.5 py-6 text-center text-sm text-muted-foreground" role="presentation">
						{#if query.trim()}
							{m.search_no_results()}
						{:else}
							{m.search_hint_empty()}
						{/if}
					</li>
				{/each}
			</ul>

			<div class="flex items-center justify-between gap-3 border-t px-3 py-2 text-[11px] text-faint">
				<span>{m.search_footer_keys()}</span>
				<span class="truncate font-mono" data-testid="palette-footer-contract">
					{pageMode ? m.search_footer_page() : m.search_group_shell()}
				</span>
			</div>
	</div>
{/if}
