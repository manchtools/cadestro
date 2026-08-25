<script lang="ts">

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

		age?: 0 | 1 | 2 | 3;
		converging?: boolean;
		selected?: boolean;

		label?: string;
		onclick?: () => void;

		reducedMotion?: boolean;
	} = $props();

	function prefersReducedMotion(): boolean {
		return (
			typeof window !== 'undefined' &&
			!!window.matchMedia &&
			window.matchMedia('(prefers-reduced-motion: reduce)').matches
		);
	}

	const AGE_OPACITY = ['1', '0.78', '0.55', '0.34'];

	const shape = $derived(tone === 'warn' ? 'dot' : tone === 'crit' ? 'notch' : tone === 'idle' ? 'hollow' : 'none');
	const statusText = $derived(TONE_LABEL[tone]());
	const name = $derived(
		[label, statusText, converging ? m.fleet_tile_converging() : null].filter(Boolean).join(' · ')
	);

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

		<span data-marker="dot" class="pointer-events-none absolute bottom-[3px] left-[3px] h-1 w-1 rounded-full bg-marker"></span>
	{:else if shape === 'notch'}

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
