<script lang="ts">
	// The action library's FAR PANE — concept A4's overview lens, applied to the
	// library instead of the fleet: one bordered bubble per action type present,
	// a dense tile grid inside it, and nothing else.
	//
	// It stays a PURE SUMMARY (A4's trade note). The only NAVIGATION affordance
	// is the bubble header, which drives the page's REAL type filter. A tile is
	// still not a second navigation target: its whole interaction is the peek —
	// hover, focus or activate it and the bubble names the action in its own peek
	// slot. That is a button rather than a `<span>` only because a peek a keyboard
	// operator cannot reach is not an affordance, and a caption promising one is
	// then a lie; activating it navigates nowhere, so the grid still cannot fight
	// the header for the click.
	//
	// The tiles are hand-rolled rather than `$lib/components/fleet`'s Tile,
	// deliberately: that atom composes its accessible name from TONE_LABEL, whose
	// words are device health ("Healthy", "Critical", "Never seen"). An action has
	// no health — pointing a status word at it would be a fabricated state. These
	// tiles reuse the fleet's TONE_FILL grammar and name themselves with the
	// action's own desired state, in the same words the list row's chip uses.
	import { Chip, TONE_FILL } from '$lib/components/fleet';
	import * as m from '$lib/paraglide/messages';
	import {
		getActionTypeIcon,
		getActionTypeInfoByValue,
		getActionTypeLabel
	} from '$lib/components/actions';
	import type { LibraryAction, LibraryBubble } from './library-model';

	let {
		bubbles,
		onFocus
	}: {
		bubbles: LibraryBubble[];
		/** Narrow the list to this bucket. Called only for filterable bubbles. */
		onFocus: (bucket: string) => void;
	} = $props();

	// The peeked tile, carrying its bucket so the bubble it belongs to is the one
	// that names it — the builder palette's `peek()`/`peeked` idiom, at tile scale.
	let peeked = $state<{ bucket: string; action: LibraryAction } | null>(null);

	function peek(bucket: string, action: LibraryAction) {
		peeked = { bucket, action };
	}

	/** Guarded by id: a pointer crossing straight from one tile to the next must
	 *  not let the leave handler clear the peek the next tile just set. */
	function unpeek(action: LibraryAction) {
		if (peeked?.action.id === action.id) peeked = null;
	}

	// The list row's words for the one real binary an action carries.
	function stateLabel(action: LibraryAction) {
		return action.absent ? m.desired_state_absent() : m.desired_state_present();
	}

	// The same display rule the list row applies: a SHELL action flagged
	// is_compliance reads as a compliance check, not a shell script.
	function display(bubble: LibraryBubble) {
		if (bubble.compliance) return getActionTypeInfoByValue('COMPLIANCE_CHECK');
		return { label: getActionTypeLabel(bubble.type), icon: getActionTypeIcon(bubble.type) };
	}
</script>

<div data-tour="library-grid" class="space-y-2 rounded-xl border bg-sunken p-3">
	<div
		class="flex items-center justify-between font-mono text-[0.62rem] uppercase tracking-[0.08em] text-faint"
	>
		<span>{m.actions_overview_caption()}</span>
		<span>{m.actions_overview_hint()}</span>
	</div>

	{#each bubbles as bubble (bubble.id)}
		{@const info = display(bubble)}
		{@const Icon = info.icon}
		<div
			data-testid="library-bubble"
			data-bucket={bubble.id}
			data-filterable={bubble.filterable ? 'true' : 'false'}
			class="rounded-[10px] border bg-surface p-2"
		>
			{#snippet head()}
				<span class="flex min-w-0 items-center gap-1.5">
					<Icon class="h-3.5 w-3.5 shrink-0 text-accent-ink" />
					<span class="truncate font-mono text-[0.7rem] font-semibold">{info.label}</span>
				</span>
				<span class="flex shrink-0 items-center gap-1.5">
					{#if !bubble.filterable}
						<!-- Said out loud rather than silently dropping the bucket: these
						     actions are real, the type menu just has no entry for them. -->
						<Chip tone="idle" label={m.actions_overview_no_filter()} />
					{/if}
					<span class="font-mono text-[0.6rem] text-faint">
						{m.actions_overview_meta({ count: bubble.actions.length, remove: bubble.remove })}
					</span>
				</span>
			{/snippet}

			{#if bubble.filterable}
				<button
					type="button"
					data-testid="library-bubble-header"
					onclick={() => onFocus(bubble.id)}
					class="flex w-full items-center justify-between gap-2 rounded-[6px] text-left focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
				>
					{@render head()}
				</button>
			{:else}
				<div class="flex w-full items-center justify-between gap-2">{@render head()}</div>
			{/if}

			<!-- The peek slot: a FIXED row between this bubble's header and its own
			     grid, so the preview lands beside the tile it describes and its
			     appearance can never reflow the grid out from under the pointer.
			     It is aria-hidden because it says exactly what the focused tile's
			     accessible name already says — assistive tech would hear it twice. -->
			<div
				data-testid="library-peek"
				data-bucket={bubble.id}
				aria-hidden="true"
				class="flex h-6 items-center gap-1.5 overflow-hidden text-xs transition-opacity duration-150 motion-reduce:transition-none {peeked?.bucket ===
				bubble.id
					? 'opacity-100'
					: 'opacity-0'}"
			>
				{#if peeked?.bucket === bubble.id}
					{@const a = peeked.action}
					<span data-testid="library-peek-name" class="min-w-0 truncate font-medium">{a.name}</span>
					<span class="shrink-0 font-mono text-[0.6rem] text-faint">{info.label}</span>
					<Chip tone={a.absent ? 'crit' : 'ok'} label={stateLabel(a)} />
				{/if}
			</div>

			<div class="grid grid-cols-[repeat(auto-fill,minmax(14px,1fr))] gap-[3px]">
				{#each bubble.actions as a (a.id)}
					<button
						type="button"
						data-testid="library-tile"
						data-action-id={a.id}
						data-state={a.absent ? 'absent' : 'present'}
						aria-label="{a.name} · {stateLabel(a)}"
						onmouseenter={() => peek(bubble.id, a)}
						onmouseleave={() => unpeek(a)}
						onfocus={() => peek(bubble.id, a)}
						onblur={() => unpeek(a)}
						onclick={() => peek(bubble.id, a)}
						class="relative block aspect-square w-full min-w-[14px] rounded-[4px] border-0 p-0 focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-ring {a.absent
							? TONE_FILL.crit
							: TONE_FILL.ok}"
					>
						{#if a.absent}
							<!-- Never colour-alone: a removal also carries the fleet's
							     crit notch, as a real child element. -->
							<span
								data-marker="notch"
								class="pointer-events-none absolute right-0 top-0 h-2 w-2 bg-marker-strong [clip-path:polygon(100%_0,0_0,100%_100%)]"
							></span>
						{/if}
					</button>
				{/each}
			</div>
		</div>
	{/each}
</div>
