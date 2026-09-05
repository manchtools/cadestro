<script lang="ts">
	import { fly, slide } from 'svelte/transition';
	import { X, ChevronDown, AppWindow, Pencil } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import { goto } from '$lib/navigation';
	import { panelKindPlural, panelKindSingular } from '$lib/shell/panel-labels';
	import { shell, restorePanel, closePanel, stagedByKind, restoreDraft, discardDraft, type ShellPanel } from '$lib/shell/shell.svelte';

	function restore(id: string) {
		const route = restoreDraft(id);
		if (route) void goto(route);
	}

	const stacks = $derived(stagedByKind());
	const drafts = $derived(shell.drafts);
	const anything = $derived(stacks.length > 0 || drafts.length > 0);

	let open = $state<Record<string, boolean>>({});
	const toggle = (k: string) => (open = { ...open, [k]: !open[k] });

	const label = panelKindPlural;
</script>

{#snippet windowCard(p: ShellPanel)}
	<div class="group relative">
		<button type="button" onclick={() => restorePanel(p.id)} data-testid="stage-card" data-panel-id={p.id} class="flex w-full items-center gap-2 rounded-lg border bg-card p-2 text-left shadow-plate transition-transform hover:scale-[1.03] hover:border-primary">
			<span class="grid h-8 w-8 shrink-0 place-items-center rounded-md bg-muted"><AppWindow class="h-4 w-4 text-muted-foreground" /></span>
			<span class="min-w-0 flex-1">
				<span class="block truncate font-mono text-xs font-medium">{p.title}</span>
				<span class="text-[10px] text-muted-foreground">{panelKindSingular(p.kind)}</span>
			</span>
		</button>
		<button type="button" aria-label={m.shell_close()} onclick={() => closePanel(p.id)} class="absolute -right-1.5 -top-1.5 hidden rounded-full border bg-card p-0.5 text-muted-foreground shadow group-hover:block hover:text-destructive"><X class="h-3 w-3" /></button>
	</div>
{/snippet}

{#snippet stack(kind: string, panels: ShellPanel[])}
	{#if panels.length === 1}
		{@render windowCard(panels[0])}
	{:else}
		<div>

			<div class="relative">
				<div class="absolute inset-x-1 -bottom-1 h-full rounded-lg border bg-card/50"></div>
				<div class="absolute inset-x-0.5 -bottom-0.5 h-full rounded-lg border bg-card/75"></div>
				<button type="button" onclick={() => toggle(kind)} data-testid="stage-stack" data-kind={kind} class="relative flex w-full items-center gap-2 rounded-lg border bg-card p-2 text-left shadow-plate hover:border-primary">
					<span class="grid h-8 w-8 shrink-0 place-items-center rounded-md bg-muted"><AppWindow class="h-4 w-4 text-muted-foreground" /></span>
					<span class="min-w-0 flex-1 truncate text-xs font-medium">{label(kind)}</span>
					<span class="rounded-full bg-primary px-1.5 text-[10px] font-semibold text-primary-foreground">{panels.length}</span>
					<ChevronDown class="h-3.5 w-3.5 shrink-0 text-muted-foreground transition-transform {open[kind] ? 'rotate-180' : ''}" />
				</button>
			</div>
			{#if open[kind]}
				<div transition:slide={{ duration: 160 }} class="mt-1.5 space-y-1.5 border-l-2 border-border pl-2">
					<div class="px-1 text-[10px] uppercase tracking-wide text-muted-foreground">{label(kind)} · {panels.length}</div>
					{#each panels as p (p.id)}
						{@render windowCard(p)}
					{/each}
				</div>
			{/if}
		</div>
	{/if}
{/snippet}

{#if anything}
	<div
		class="fixed right-3 top-24 z-30 flex w-48 flex-col gap-2"
		data-testid="stage-rail"
		data-tour="stage-rail"
	>
		<div class="px-1 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">{m.stage_heading()}</div>



		{#each stacks as s (s.kind)}
			{@render stack(s.kind, s.panels)}
		{/each}

		{#if drafts.length}
			<div class="px-1 pt-1 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">{m.stage_drafts_heading()}</div>
			{#each drafts as d (d.id)}
				<div class="group relative" transition:fly={{ x: 24, duration: 160 }}>
					<button
						type="button"
						data-testid="stage-draft"
						data-draft-id={d.id}
						aria-label="{m.stage_draft_restore()}: {d.title}"
						onclick={() => restore(d.id)}
						class="flex w-full items-center gap-2 rounded-lg border border-dashed bg-card p-2 text-left shadow-plate transition-transform hover:scale-[1.03] hover:border-primary"
					>
						<span class="grid h-8 w-8 shrink-0 place-items-center rounded-md bg-accent text-accent-foreground"><Pencil class="h-4 w-4" /></span>
						<span class="min-w-0 flex-1">
							<span class="block truncate text-xs font-medium">{d.title}</span>
							{#if d.subtitle}<span class="block truncate text-[10px] text-muted-foreground">{d.subtitle}</span>{/if}
						</span>
					</button>

					<button
						type="button"
						data-testid="stage-draft-discard"
						data-draft-id={d.id}
						aria-label="{m.stage_draft_discard()}: {d.title}"
						onclick={() => discardDraft(d.id)}
						class="absolute -right-1.5 -top-1.5 hidden rounded-full border bg-card p-0.5 text-muted-foreground shadow group-hover:block hover:text-destructive"
					>
						<X class="h-3 w-3" />
					</button>
				</div>
			{/each}
		{/if}
	</div>
{/if}
