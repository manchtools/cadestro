<script lang="ts">
	import type { Snippet } from 'svelte';
	import { cubicOut } from 'svelte/easing';
	import { Search, ChevronDown, House, X, ArrowDownToLine, Trash2 } from '@lucide/svelte';
	import type { PillEntry, PillGroup } from '$lib/shell/nav';
	import * as m from '$lib/paraglide/messages';
	import { goto } from '$lib/navigation';
	import {
		shell,
		pillMode,
		pillSubtext,
		runPillAction,
		clearSelection,
		commitContext,
		requestCancelContext,
		confirmCancelContext,
		dismissCancelConfirm,
		stashContext,
		shellPath,
		shellPreviousPath,
		handlePillKey
	} from '$lib/shell/shell.svelte';

	/** The fleet home — the shell's resting surface (nav's first section). */
	const HOME_ROUTE = '/devices';

	// Stash parks the context AND returns the operator to where they opened the
	// editor from — otherwise they are stranded on the now-empty create/edit
	// surface with their work tucked onto the stage. The origin is captured
	// before stashContext() clears the context. A deep-linked editor has no
	// prior path, so the chrome leaves them put.
	function stash() {
		const back = shellPreviousPath();
		const here = shellPath();
		stashContext();
		if (back && back !== here) void goto(back);
	}

	/** The home glyph in context mode: park unsaved work on the stage and go to
	 *  the fleet home. Stash, never discard — leaving must not cost the operator
	 *  their buffer. A context with no route cannot park, so it just navigates
	 *  (the surface's own teardown then applies).
	 *
	 *  Only DIRTY work parks. The pill is held for an entity's whole visit now, so
	 *  parking unconditionally littered the stage with cards for pages the
	 *  operator had merely looked at. */
	function stashAndGoHome() {
		if (shell.pill.context?.route && shell.pill.context.dirty) stashContext();
		void goto(HOME_ROUTE);
	}

	// The pill renders props: the layout owns the auth client and passes
	// the permission-filtered tables (adaptor seam; this file must stay free of
	// $pmSdk/$sdk imports, guard-enforced).
	//
	// `searchSurface` is that same seam for ⌘K. There is exactly ONE search
	// implementation — the palette — and it talks to the Search RPC, so it cannot
	// live in this file. The layout owns it and hands it in as a snippet; the pill
	// contributes the morph, the surface, and the dismiss layer around it.
	let {
		pathname,
		hrefBase = '',
		sections = [],
		overflow = [],
		searchSurface
	}: {
		pathname: string;
		hrefBase?: string;
		sections?: PillEntry[];
		overflow?: PillGroup[];
		searchSurface?: Snippet;
	} = $props();

	// Four modes, one at a time; the store owns precedence and transitions.
	const mode = $derived(pillMode());
	const selection = $derived(shell.pill.selection);
	const ctx = $derived(shell.pill.context);
	const sub = $derived(pillSubtext());

	// ⌘S commits, Esc cancels (confirming only when dirty) — the store decides,
	// this only forwards and suppresses the browser default it consumed.
	function onWindowKey(e: KeyboardEvent) {
		if (e.defaultPrevented) return;
		if (handlePillKey(e)) e.preventDefault();
	}

	let moreOpen = $state(false);
	const href = (h: string) => `${hrefBase}${h}`;
	const active = (h: string) => pathname === href(h) || pathname.startsWith(href(h) + '/');

	// Smooth morph. fit-content can't be tweened by CSS, so the pill snaps between
	// modes. We measure the live content and drive explicit px width/height, which
	// CSS *can* animate — the container glides while the content crossfades on top.
	let inner: HTMLElement | undefined = $state();
	let dims = $state({ w: 0, h: 0 });
	$effect(() => {
		const el = inner;
		if (!el) return;
		const measure = () => (dims = { w: el.offsetWidth, h: el.offsetHeight });
		measure();
		const ro = new ResizeObserver(measure);
		ro.observe(el);
		return () => ro.disconnect();
	});
	// The whole chrome is FIXED, so the scrolling page underneath cannot see it.
	// The pill alone is a constant the layout could hard-code, but the caption is
	// not: it appears, disappears and wraps to two lines. Publishing the column's
	// real height lets the content reserve exactly that much and stop the caption
	// landing on top of the first card. Cleared on teardown so a page rendered
	// without the chrome does not keep the reservation.
	let column: HTMLElement | undefined = $state();
	$effect(() => {
		const el = column;
		if (!el || typeof document === 'undefined') return;
		const publish = () =>
			document.documentElement.style.setProperty('--pill-block', `${el.offsetHeight}px`);
		publish();
		const ro = new ResizeObserver(publish);
		ro.observe(el);
		return () => {
			ro.disconnect();
			document.documentElement.style.removeProperty('--pill-block');
		};
	});

	const reduceMotion =
		typeof window !== 'undefined' && !!window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;

	// Content emerges from / collapses into the surface centre (subtle scale + fade),
	// NOT a blur-dissolve and NOT a hard crossfade. The old content leaves FAST and
	// the new one arrives on a delay, so they cross at low combined opacity instead of
	// both sitting at ~50% (which reads as two stacked cards). Meanwhile the grid keeps
	// BOTH branches — transform/opacity don't affect layout — so the container measures
	// the max size throughout and never empties; the box glides while its contents swap.
	function zoomFade(
		node: Element,
		{ duration = 170, start = 0.94, delay = 0 }: { duration?: number; start?: number; delay?: number } = {}
	) {
		return {
			duration,
			delay,
			easing: cubicOut,
			css: (t: number) => `opacity:${t};transform:scale(${start + (1 - start) * t});`
		};
	}
	const zoomIn = { duration: reduceMotion ? 0 : 170, start: 0.94, delay: reduceMotion ? 0 : 70 };
	const zoomOut = { duration: reduceMotion ? 0 : 110, start: 0.96 };

	// Only width/height animate — the corner radius is a CONSTANT, which reads
	// as a capsule when the pill is short and a rounded-rect when it's tall, with no
	// radius tween to balloon/squash mid-morph.
	const morphStyle = $derived(
		(reduceMotion
			? 'transition:none;'
			: 'transition:width 220ms cubic-bezier(0.22,1,0.36,1),height 220ms cubic-bezier(0.22,1,0.36,1);') +
			(dims.w ? `width:${dims.w}px;height:${dims.h}px;` : '')
	);

	$effect(() => {
		if (mode !== 'nav') moreOpen = false;
	});
</script>

<svelte:window onkeydown={onWindowKey} />

{#if mode === 'search'}
	<!-- Dismiss layer, not a modal backdrop: search is absorbed into the pill and
	     the list it filters must stay legible behind it, so this only catches the
	     click that means "done". -->
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-40 bg-black/20"
		data-testid="pill-search-dismiss"
		onclick={() => (shell.paletteOpen = false)}
	></div>
{/if}

<div class="pointer-events-none fixed inset-x-0 top-3 z-50 flex justify-center px-4" data-testid="morph-bar" data-mode={mode}>
	<div
		bind:this={column}
		class="pointer-events-auto relative flex w-fit max-w-[calc(100vw-2rem)] flex-col items-center"
	>
		<!-- The pill carries EXPLICIT measured width/height (see morphStyle) so the
		     container can animate between modes; content crossfades on top, centered
		     so growth is symmetric (island-style) and never clips one side. -->
		<div
			data-testid="pill"
			style={morphStyle}
			class="relative z-10 flex max-w-[calc(100vw-2rem)] items-start justify-center overflow-hidden rounded-[26px] border bg-popover/95 text-popover-foreground shadow-pill backdrop-blur"
		>
			<div bind:this={inner} class="grid w-max max-w-[calc(100vw-2rem)]">
				{#if mode === 'search'}
					<!-- ⌘K is the pill's fourth posture: the surface the layout handed
					     in renders INSIDE the morph, so one keystroke searches the page,
					     the fleet and this tab from the same box. -->
					<div class="col-start-1 row-start-1 w-max justify-self-center" data-testid="pill-search" in:zoomFade={zoomIn} out:zoomFade={zoomOut}>
						{@render searchSurface?.()}
					</div>
				{:else if mode === 'selection' && selection}
					<!-- selection: "12 selected · Assign · … · ✕" -->
					<div
						class="col-start-1 row-start-1 flex w-max items-center justify-self-center gap-[3px] px-2 py-[7px]"
						role="group"
						aria-label={m.pill_selection_region()}
						in:zoomFade={zoomIn}
						out:zoomFade={zoomOut}
					>
						<span data-testid="pill-selection-count" class="px-2.5 py-2 text-sm font-semibold tabular-nums">
							{m.pill_selection_count({ count: selection.count })}
						</span>
						<span class="mx-[7px] h-[17px] w-px bg-border-strong"></span>
						{#each selection.actions as a (a.id)}
							<button
								type="button"
								data-testid="pill-action"
								data-action-id={a.id}
								data-tone={a.tone ?? 'neutral'}
								onclick={() => runPillAction(a.id)}
								class="flex items-center gap-1.5 whitespace-nowrap rounded-full px-3.5 py-2 text-sm {a.tone ===
								'danger'
									? 'text-crit hover:bg-crit-soft'
									: a.primary
										? 'bg-accent font-semibold text-accent-foreground'
										: 'text-muted-foreground hover:bg-accent/50'}"
							>
								<!-- A destructive action on a SELECTION is the most dangerous
								     button in the app — it acts on everything selected. -->
								{#if a.tone === 'danger'}<Trash2 class="h-3.5 w-3.5" />{/if}
								{a.label}
							</button>
						{/each}
						<button
							type="button"
							data-testid="pill-clear"
							aria-label={m.pill_clear_selection()}
							onclick={clearSelection}
							class="grid h-8 w-8 place-items-center rounded-full text-muted-foreground hover:bg-accent/50"
						>
							<X class="h-3.5 w-3.5" />
						</button>
					</div>
				{:else if mode === 'context' && ctx}
					<!-- context: home glyph · dirty dot + target name · Stash / Cancel / commit -->
					<div
						class="col-start-1 row-start-1 flex w-max items-center justify-self-center gap-[3px] px-2 py-[7px]"
						role="group"
						aria-label="{m.pill_context_region()}: {ctx.title}"
						in:zoomFade={zoomIn}
						out:zoomFade={zoomOut}
					>
						<!-- Not decoration: the way out. It parks the work on the stage
						     (never discards it) and returns to the fleet home. -->
						<button
							type="button"
							data-testid="pill-home"
							aria-label={m.pill_context_home()}
							onclick={stashAndGoHome}
							class="grid h-7 w-7 shrink-0 place-items-center rounded-full text-muted-foreground hover:bg-accent/50 hover:text-foreground"
						>
							<House class="h-3.5 w-3.5" />
						</button>
						<span class="mx-[7px] h-[17px] w-px bg-border-strong"></span>
						<span class="flex items-center gap-2 px-2 py-2 text-sm font-semibold">
							{#if ctx.dirty}
								<span
									data-testid="pill-dirty"
									role="img"
									aria-label={m.pill_unsaved_changes()}
									class="h-2 w-2 shrink-0 rounded-full bg-warn ring-[3px] ring-warn/25"
								></span>
							{/if}
							<span class="max-w-[20rem] truncate">{ctx.title}</span>
						</span>
						<span class="mx-[7px] h-[17px] w-px bg-border-strong"></span>
						{#each ctx.extraActions ?? [] as a (a.id)}
							<button
								type="button"
								data-testid="pill-action"
								data-action-id={a.id}
								data-tone={a.tone ?? 'neutral'}
								onclick={() => runPillAction(a.id)}
								class="flex items-center gap-1.5 whitespace-nowrap rounded-full px-3 py-2 text-sm {a.tone ===
								'danger'
									? 'text-crit hover:bg-crit-soft'
									: 'text-muted-foreground hover:bg-accent/50'}"
							>
								{#if a.tone === 'danger'}<Trash2 class="h-3.5 w-3.5" />{/if}
								{a.label}
							</button>
						{/each}
						<!-- Stash and Cancel are EDITING exits: they only mean something
						     once there are changes. An edit surface holds the pill for its
						     whole visit so the entity's own actions have a home, so a clean
						     context is a resting state — offering "discard" and "park" with
						     nothing to discard or park would be noise.
						     Stash additionally needs a route: a parked draft that cannot say
						     where it came from could never be restored, and the store
						     refuses it, so an offered-but-refused button would be a lie. -->
						{#if ctx.dirty}
							{#if ctx.route}
								<button
									type="button"
									data-testid="pill-stash"
									onclick={stash}
									class="flex items-center gap-1.5 whitespace-nowrap rounded-full px-3 py-2 text-sm text-muted-foreground hover:bg-accent/50"
								>
									<ArrowDownToLine class="h-3.5 w-3.5" />
									{m.pill_stash()}
								</button>
							{/if}
							<button
								type="button"
								data-testid="pill-cancel"
								onclick={requestCancelContext}
								class="flex items-center gap-1.5 whitespace-nowrap rounded-full px-3 py-2 text-sm text-muted-foreground hover:bg-accent/50"
							>
								{m.common_cancel()}
								<kbd class="rounded border border-border-strong px-1.5 py-0.5 text-[10px]">esc</kbd>
							</button>
						{/if}
						<!-- Nothing to save, or not savable: disabled AND guarded in the
						     store, so the keyboard path is closed too. -->
						<button
							type="button"
							data-testid="pill-commit"
							disabled={!(ctx.valid && ctx.dirty)}
							onclick={commitContext}
							class="flex items-center gap-1.5 whitespace-nowrap rounded-full bg-primary px-4 py-2 text-sm font-semibold text-primary-foreground {ctx.valid &&
							ctx.dirty
								? ''
								: 'cursor-not-allowed opacity-40'}"
						>
							{ctx.commitLabel}
							<kbd class="rounded border border-primary-foreground/40 px-1.5 py-0.5 text-[10px]">⌘S</kbd>
						</button>
					</div>
				{:else}
					<nav class="col-start-1 row-start-1 flex w-max items-center justify-self-center gap-[3px] px-2 py-[7px]" aria-label={m.shell_sections_region()} in:zoomFade={zoomIn} out:zoomFade={zoomOut}>
						{#each sections as s (s.href)}
							<a
								href={href(s.href)}
								aria-current={active(s.href) ? 'page' : undefined}
								class="flex items-center gap-1.5 rounded-full px-3.5 py-2 text-sm {active(s.href) ? 'bg-accent font-medium text-accent-foreground' : 'text-muted-foreground hover:bg-accent/50'}"
							>
								<s.icon class="h-3.5 w-3.5" />
								{s.label()}
							</a>
						{/each}
						{#if overflow.length}
							<div data-testid="pill-more">
								<!-- Labelled "More", not the draft's "Admin": the overflow deliberately
								     carries a Workspace group too, and a content-honest label wins. -->
								<button type="button" data-tour="nav-pill-overflow" aria-expanded={moreOpen} onclick={() => (moreOpen = !moreOpen)} class="flex items-center gap-1.5 rounded-full px-3.5 py-2 text-sm text-muted-foreground hover:bg-accent/50">
									{m.shell_more()}
									<ChevronDown class="h-3.5 w-3.5 transition-transform {moreOpen ? 'rotate-180' : ''}" />
								</button>
							</div>
						{/if}
						<span class="mx-[7px] h-[17px] w-px bg-border-strong"></span>
						<button type="button" aria-label={m.shell_open_search()} onclick={() => (shell.paletteOpen = true)} class="flex items-center gap-1.5 rounded-full px-3.5 py-2 text-sm text-muted-foreground hover:bg-accent/50">
							<Search class="h-3.5 w-3.5" />
							<kbd class="rounded border border-border-strong bg-muted px-1.5 py-0.5 text-[10px]">⌘K</kbd>
						</button>
					</nav>
				{/if}
			</div>
		</div>

		<!-- Subtext strip — a DETACHED caption (round 2 supersedes the tucked-behind
		     round-1 treatment): a real gap below the pill, its own border and shadow,
		     capped to the pill's width (width:0 + min-width:100% keeps the caption
		     from widening the shrink-wrapped column) and clamped to two lines. It is
		     the one home for validation rollups, selection implications and compiled
		     queries, and renders only when there is something to say. -->
		{#if sub}
			<!-- The row is width-neutral (width:0 + min-width:100%) so a long caption
			     can never widen the shrink-wrapped column and stretch the pill with
			     it; the caption inside hugs its own text and is capped at that width.
			     "Name is required" is three words, so a bar spanning the whole pill
			     was reading as a banner rather than a caption. -->
			<div style="width:0;min-width:100%" class="mt-2.5 flex justify-center">
				<div
					data-testid="pill-subtext"
					data-tone={sub.tone ?? 'neutral'}
					title={sub.text}
					class="line-clamp-2 max-w-full rounded-[11px] border-[1.5px] px-3.5 py-1.5 text-center text-[11px] leading-[1.5] shadow-plate {sub.tone ===
					'warn'
						? 'border-warn/65 bg-warn-soft text-warn'
						: 'border-border-strong bg-sunken text-muted-foreground'}"
				>
					{sub.text}
				</div>
			</div>
		{/if}

		{#if mode === 'context' && shell.pill.cancelPending}
			<!-- Esc on a DIRTY context asks before discarding; a clean one never asks. -->
			<div class="absolute left-1/2 top-[calc(100%+0.5rem)] z-50 w-72 -translate-x-1/2 rounded-xl border bg-popover p-3 text-popover-foreground shadow-pill" data-testid="pill-cancel-confirm">
				<div class="pb-2.5 text-sm">{m.pill_discard_title()}</div>
				<div class="flex justify-end gap-2">
					<button type="button" data-testid="pill-keep-editing" onclick={dismissCancelConfirm} class="rounded-full border px-3 py-1.5 text-sm text-muted-foreground hover:bg-accent/50">
						{m.pill_keep_editing()}
					</button>
					<button type="button" data-testid="pill-discard" onclick={confirmCancelContext} class="rounded-full bg-destructive px-3 py-1.5 text-sm font-semibold text-destructive-foreground">
						{m.pill_discard_confirm()}
					</button>
				</div>
			</div>
		{/if}

		{#if mode === 'nav' && moreOpen && overflow.length}
			<!-- Render outside the overflow-hidden morph container so the menu is not clipped. -->
			<div class="absolute left-1/2 top-[calc(100%+0.5rem)] z-50 w-56 -translate-x-1/2 rounded-xl border bg-popover p-1.5 text-popover-foreground shadow-pill" data-testid="pill-more-menu">
				{#each overflow as g (g.group)}
					<div class="px-2 pb-0.5 pt-1.5 text-[10px] uppercase tracking-wide text-muted-foreground">{g.group()}</div>
					{#each g.items as item (item.href)}
						<a href={href(item.href)} onclick={() => (moreOpen = false)} class="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm {active(item.href) ? 'bg-accent text-accent-foreground' : 'text-popover-foreground hover:bg-accent/60'}">
							<item.icon class="h-3.5 w-3.5 text-muted-foreground" />
							{item.label()}
						</a>
					{/each}
				{/each}
			</div>
		{/if}
	</div>
</div>
