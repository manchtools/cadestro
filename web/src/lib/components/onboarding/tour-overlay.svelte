<script lang="ts">
	// The coach-mark overlay: one spotlight ring plus one card per step.
	//
	// Two deliberate choices:
	//
	//   * the dim is the ring's own outsized box-shadow and the ring is
	//     `pointer-events-none`, so the page behind stays scrollable and
	//     clickable. A tour must never be able to strand an operator;
	//   * the anchor is scrolled into view BEFORE the first measurement, and the
	//     placement is re-measured on every scroll and resize, so the card
	//     tracks its anchor instead of pointing at where it used to be.
	import * as m from '$lib/paraglide/messages';
	import {
		onboarding,
		nextStep,
		prevStep,
		skipTour,
		dropCurrentStep
	} from '$lib/onboarding/tour.svelte';
	import { resolveStep } from '$lib/onboarding/steps';
	import { placeCard, type Box } from '$lib/onboarding/position';
	import { cycleTab } from '$lib/onboarding/focus';
	import { motion } from '$lib/onboarding/motion';

	/** Fixed card width; the height is measured, because translated bodies wrap
	 *  to different line counts and a guessed height mis-places the card. */
	const CARD_W = 348;
	/** Breathing room between the anchor's box and the ring. */
	const RING_PAD = 5;
	/** Soft corner for an anchor that has none of its own. */
	const RING_RADIUS_DEFAULT = '12px';

	const reduced = motion.reduced();

	let card = $state<HTMLElement | undefined>();
	let anchorBox = $state<Box | null>(null);
	let ringRadius = $state(RING_RADIUS_DEFAULT);
	let cardSize = $state({ width: CARD_W, height: 190 });
	let viewport = $state({ width: 1024, height: 768 });

	const step = $derived(onboarding.steps[onboarding.index] ?? null);
	const total = $derived(onboarding.steps.length);
	const isLast = $derived(onboarding.index === total - 1);
	const place = $derived(anchorBox ? placeCard(anchorBox, cardSize, viewport) : null);

	function measure(el: HTMLElement) {
		const r = el.getBoundingClientRect();
		anchorBox = { x: r.left, y: r.top, width: r.width, height: r.height };
		// The ring sits RING_PAD outside the anchor, so its corners must grow by
		// the same amount to stay concentric — a pill anchor gets a pill ring,
		// never a foreign rectangle. Multi-value/percentage radii pass through
		// unchanged (close enough at 5px out); a square anchor keeps a soft 12px.
		//
		// Order matters: `0px` matches the single-value pattern too, so the
		// square case has to be decided BEFORE the grow-by-padding branch —
		// otherwise a square anchor silently rings at RING_PAD (5px).
		const raw = getComputedStyle(el).borderRadius;
		const single = /^(\d+(?:\.\d+)?)px$/.exec(raw);
		if (single) {
			const corner = parseFloat(single[1]);
			ringRadius = corner > 0 ? `${corner + RING_PAD}px` : RING_RADIUS_DEFAULT;
		} else {
			ringRadius = raw ? raw : RING_RADIUS_DEFAULT;
		}
		viewport = { width: window.innerWidth, height: window.innerHeight };
		if (card) cardSize = { width: card.offsetWidth, height: card.offsetHeight };
	}

	// One effect per step: resolve, scroll into view, then keep the geometry
	// current until the step changes.
	$effect(() => {
		if (!onboarding.running || !step) return;
		const el = resolveStep(step);
		if (!el) {
			// The anchor left the DOM between the tour starting and this step being
			// reached — drop the step rather than ring empty space.
			dropCurrentStep();
			return;
		}
		el.scrollIntoView({ block: 'center', inline: 'center', behavior: reduced ? 'auto' : 'smooth' });
		measure(el);
		card?.focus();

		const remeasure = () => measure(el);
		window.addEventListener('scroll', remeasure, { capture: true, passive: true });
		window.addEventListener('resize', remeasure);
		const ro = new ResizeObserver(remeasure);
		ro.observe(el);
		if (card) ro.observe(card);
		return () => {
			window.removeEventListener('scroll', remeasure, { capture: true });
			window.removeEventListener('resize', remeasure);
			ro.disconnect();
		};
	});

	function onkeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			e.preventDefault();
			e.stopPropagation();
			skipTour();
			return;
		}
		if (card && cycleTab(e, card)) e.preventDefault();
	}

	const ringTransition = reduced
		? 'transition:none;'
		: 'transition:left 220ms cubic-bezier(0.22,1,0.36,1),top 220ms cubic-bezier(0.22,1,0.36,1),width 220ms cubic-bezier(0.22,1,0.36,1),height 220ms cubic-bezier(0.22,1,0.36,1);';
</script>

{#if onboarding.running && step}
	{#if anchorBox}
		<!-- Decoration only: the cut-out dim and the ring carry no information the
		     card does not also say, so assistive tech never sees them. -->
		<div
			aria-hidden="true"
			data-testid="tour-spotlight"
			data-motion={reduced ? 'reduced' : 'full'}
			style="left:{anchorBox.x - RING_PAD}px;top:{anchorBox.y - RING_PAD}px;width:{anchorBox.width +
				RING_PAD * 2}px;height:{anchorBox.height +
				RING_PAD * 2}px;border-radius:{ringRadius};box-shadow:0 0 0 9999px rgb(0 0 0 / 0.45);{ringTransition}"
			class="pointer-events-none fixed z-[60] border-2 border-accent-ink"
		>
			{#if !reduced}
				<!-- The pulse coincides with the main border (inset -2px = the border's
				     own box) so it reads as one glowing ring, never a second outline. -->
				<span
					class="absolute inset-[-2px] animate-pulse border-2 border-accent-ink/70"
					style="border-radius:{ringRadius};"
				></span>
			{/if}
		</div>
	{/if}

	<div
		bind:this={card}
		role="dialog"
		aria-modal="true"
		aria-labelledby="onboarding-tour-title"
		aria-describedby="onboarding-tour-body"
		tabindex="-1"
		data-testid="tour-card"
		data-step={step.id}
		data-side={place?.side ?? 'bottom'}
		data-motion={reduced ? 'reduced' : 'full'}
		{onkeydown}
		style="left:{place?.x ?? 0}px;top:{place?.y ?? 0}px;width:{CARD_W}px;max-width:calc(100vw - 24px);"
		class="fixed z-[61] rounded-[14px] border bg-surface p-4 text-foreground shadow-pill outline-none"
	>
		<p data-testid="tour-counter" class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint">
			{m.onboarding_tour_step_counter({ current: onboarding.index + 1, total })}
		</p>
		<h2 id="onboarding-tour-title" class="mt-1 text-base font-semibold tracking-tight">
			{step.title()}
		</h2>
		<p id="onboarding-tour-body" class="mt-1.5 text-sm leading-relaxed text-muted-foreground">
			{step.body()}
		</p>

		<div class="mt-4 flex items-center gap-2">
			<button
				type="button"
				data-testid="tour-skip"
				onclick={skipTour}
				class="rounded-full px-3 py-1.5 text-sm text-muted-foreground hover:bg-accent/50"
			>
				{m.onboarding_tour_skip()}
			</button>
			<span class="flex-1"></span>
			<button
				type="button"
				data-testid="tour-back"
				disabled={onboarding.index === 0}
				onclick={prevStep}
				class="rounded-full border px-3 py-1.5 text-sm text-muted-foreground hover:bg-accent/50 disabled:cursor-not-allowed disabled:opacity-40"
			>
				{m.onboarding_tour_back()}
			</button>
			<button
				type="button"
				data-testid="tour-next"
				onclick={nextStep}
				class="rounded-full bg-primary px-3.5 py-1.5 text-sm font-semibold text-primary-foreground hover:opacity-90"
			>
				{isLast ? m.onboarding_tour_done() : m.onboarding_tour_next()}
			</button>
		</div>

		<p class="mt-3 border-t border-hair pt-2 text-[11px] text-faint">
			{m.onboarding_tour_restart_hint()}
		</p>
	</div>
{/if}
