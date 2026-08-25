<script lang="ts">

	import { Tile } from '$lib/components/fleet';
	import * as m from '$lib/paraglide/messages';
	import type { FleetBubble } from './fleet-model';

	let {
		bubble,
		selected,
		focused = false,
		onZoom,
		onToggle
	}: {
		bubble: FleetBubble;

		selected: Set<string>;
		focused?: boolean;
		onZoom: () => void;
		onToggle: (index: number, shift: boolean) => void;
	} = $props();

	let shiftHeld = $state(false);
</script>

<div
	data-testid="fleet-bubble"
	data-group-id={bubble.id}
	data-focused={focused ? 'true' : undefined}
	class="rounded-[10px] border bg-surface p-2 {focused ? 'outline-2 outline-offset-1 outline-accent-ink' : ''}"
>
	<button
		type="button"
		data-testid="fleet-bubble-header"
		onclick={onZoom}
		class="mb-1.5 flex w-full items-baseline justify-between gap-2 rounded-[6px] text-left focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
	>
		<span class="truncate font-mono text-[0.7rem] font-semibold">{bubble.name}</span>
		<span class="shrink-0 font-mono text-[0.6rem] text-faint">
			{m.fleet_bubble_meta({ count: bubble.devices.length, down: bubble.down })}
		</span>
	</button>

	<div
		class="grid grid-cols-[repeat(auto-fill,minmax(14px,1fr))] gap-[3px]"
		onpointerdowncapture={(e) => (shiftHeld = e.shiftKey)}
		onkeydowncapture={(e) => (shiftHeld = e.shiftKey)}
	>
		{#each bubble.devices as d, i (d.id)}
			<Tile
				tone={d.tone}
				age={d.age}
				label={d.hostname}
				selected={selected.has(d.id)}
				onclick={() => onToggle(i, shiftHeld)}
			/>
		{/each}
	</div>
</div>
