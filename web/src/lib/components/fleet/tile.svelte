<script lang="ts">
	// One device as a square status tile (Shell & Fleet round 2, movement A).
	//
	// STATUS IS NEVER COLOUR-ALONE. Every tone that carries meaning also carries
	// a shape rendered as a REAL child element, not a ::before/::after, so the
	// guarantee is assertable in the DOM and survives a stylesheet regression:
	//   warn → dot bottom-left · crit → corner notch top-right · idle → hollow
	//   dashed outline (no fill at all).
	// Heartbeat age is opacity decay on the fill; the converging ring means an
	// operation is still landing.
	import * as m from '$lib/paraglide/messages';
	import { TONE_FILL, TONE_LABEL, type FleetTone } from './tone';

	let {
		tone = 'ok',
		age = 0,
		converging = false,
		selected = false,
		label,
		onclick,
		reducedMotion = prefersReducedMotion()
	}: {
		tone?: FleetTone;
		/** Heartbeat decay step: 0 fresh … 3 stalest. */
		age?: 0 | 1 | 2 | 3;
		converging?: boolean;
		selected?: boolean;
		/** Device name, prefixed to the accessible status text. */
		label?: string;
		onclick?: () => void;
		/** Test/override seam; defaults to the media query. */
		reducedMotion?: boolean;
	} = $props();

	function prefersReducedMotion(): boolean {
		return (
			typeof window !== 'undefined' &&
			!!window.matchMedia &&
			window.matchMedia('(prefers-reduced-motion: reduce)').matches
		);
	}

	// Decay steps from the concept: 1 / .78 / .55 / .34.
	const AGE_OPACITY = ['1', '0.78', '0.55', '0.34'];

	const shape = $derived(tone === 'warn' ? 'dot' : tone === 'crit' ? 'notch' : tone === 'idle' ? 'hollow' : 'none');
	const statusText = $derived(TONE_LABEL[tone]());
	const name = $derived(
		[label, statusText, converging ? m.fleet_tile_converging() : null].filter(Boolean).join(' · ')
	);
	// Idle is hollow — an age fade on nothing would be meaningless.
	const fillOpacity = $derived(tone === 'idle' ? '1' : AGE_OPACITY[age] ?? '1');
	const base = $derived(
		'relative block aspect-square w-full min-w-[14px] rounded-[4px] p-0 ' +
			(tone === 'idle' ? 'border-[1.5px] border-dashed border-idle bg-transparent ' : TONE_FILL[tone] + ' border-0 ') +
			(selected ? 'outline-2 outline-offset-1 outline-foreground ' : '') +
			(onclick ? 'cursor-pointer focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-ring' : '')
	);
</script>

{#snippet markers()}
	{#if shape === 'dot'}
		<!-- warning: a dark dot in the lower-left corner -->
		<span data-marker="dot" class="pointer-events-none absolute bottom-[3px] left-[3px] h-1 w-1 rounded-full bg-marker"></span>
	{:else if shape === 'notch'}
		<!-- critical: a clipped triangle out of the top-right corner -->
		<span data-marker="notch" class="pointer-events-none absolute right-0 top-0 h-2 w-2 bg-marker-strong [clip-path:polygon(100%_0,0_0,100%_100%)]"></span>
	{/if}
	{#if converging}
		<span
			data-marker="ring"
			data-motion={reducedMotion ? 'static' : 'pulse'}
			class="pointer-events-none absolute -inset-[3px] rounded-[6px] border-2 border-primary {reducedMotion ? 'opacity-70' : 'animate-conv-pulse'}"
		></span>
	{/if}
{/snippet}

{#if onclick}
	<button
		type="button"
		{onclick}
		aria-label={name}
		title={name}
		data-testid="fleet-tile"
		data-tone={tone}
		data-shape={shape}
		data-age={age}
		data-selected={selected ? 'true' : undefined}
		class={base}
		style="opacity:{fillOpacity}"
	>
		{@render markers()}
	</button>
{:else}
	<span
		role="img"
		aria-label={name}
		title={name}
		data-testid="fleet-tile"
		data-tone={tone}
		data-shape={shape}
		data-age={age}
		data-selected={selected ? 'true' : undefined}
		class={base}
		style="opacity:{fillOpacity}"
	>
		{@render markers()}
	</span>
{/if}
