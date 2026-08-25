<script lang="ts">

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

	const CARD_W = 348;

	const RING_PAD = 5;

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

	$effect(() => {
		if (!onboarding.running || !step) return;
		const el = resolveStep(step);
		if (!el) {

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
