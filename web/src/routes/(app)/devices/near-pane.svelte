<script lang="ts">
	// The near pane: the same devices the far pane summarises, resolved into
	// legible dense rows — worst first, healthy rows folded away behind an
	// expander so triage is what the operator lands on.
	import { Tile, Chip, TONE_FILL, type FleetTone } from '$lib/components/fleet';
	import { ExternalLink } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';
	import type { FleetDevice } from './fleet-model';

	let {
		rows,
		caption,
		selected,
		onToggle,
		onOpen,
		openLabel,
		collapseHealthy = true
	}: {
		/** Already ordered worst-first by the caller. */
		rows: FleetDevice[];
		caption: string;
		selected: Set<string>;
		onToggle: (index: number, shift: boolean) => void;
		onOpen: (device: FleetDevice) => void;
		openLabel: string;
		collapseHealthy?: boolean;
	} = $props();

	let expanded = $state(false);
	let shiftHeld = $state(false);

	// Worst-first ordering puts every 'ok' row at the tail, so the collapse is a
	// suffix cut — never a filter that could reorder what stays on screen.
	const healthyFrom = $derived(rows.findIndex((d) => d.tone === 'ok'));
	const healthyCount = $derived(healthyFrom >= 0 ? rows.length - healthyFrom : 0);
	const cut = $derived(collapseHealthy && !expanded && healthyFrom >= 0 ? healthyFrom : rows.length);
	const hiddenCount = $derived(rows.length - cut);
	// Count only what is selected IN THIS PANE — a global count here would claim
	// devices the operator cannot see from where they are standing.
	const selectedHere = $derived(rows.filter((d) => selected.has(d.id)).length);

	const STATUS_LABEL: Record<FleetTone, () => string> = {
		ok: m.devices_status_online,
		warn: m.fleet_status_drift,
		crit: m.devices_status_offline,
		info: m.fleet_tile_info,
		idle: m.fleet_tile_idle
	};
</script>

<div class="px-1.5 py-1">
	<div class="flex items-center justify-between px-1 pb-1 font-mono text-[0.62rem] uppercase tracking-[0.08em] text-faint">
		<span data-testid="fleet-near-caption">{caption}</span>
		{#if selectedHere > 0}
			<span>{m.fleet_near_selected({ count: selectedHere })}</span>
		{/if}
	</div>

	<div
		onpointerdowncapture={(e) => (shiftHeld = e.shiftKey)}
		onkeydowncapture={(e) => (shiftHeld = e.shiftKey)}
	>
		{#each rows.slice(0, cut) as d, i (d.id)}
			<div
				data-testid="fleet-row"
				data-device-id={d.id}
				data-tone={d.tone}
				class="flex items-center gap-2 border-t border-hair px-1 py-1.5 text-sm first:border-t-0"
			>
				<span class="w-3.5 shrink-0">
					<Tile
						tone={d.tone}
						age={d.age}
						label={d.hostname}
						selected={selected.has(d.id)}
						onclick={() => onToggle(i, shiftHeld)}
					/>
				</span>
				<span aria-hidden="true" class="h-1.5 w-1.5 shrink-0 rounded-full {TONE_FILL[d.tone]}"></span>
				<span class="truncate font-mono text-[0.8rem]">{d.hostname}</span>
				<Chip tone={d.tone} label={STATUS_LABEL[d.tone]()} />
				<button
					type="button"
					data-testid="fleet-row-open"
					aria-label="{openLabel}: {d.hostname}"
					title={openLabel}
					onclick={() => onOpen(d)}
					class="ml-auto shrink-0 rounded-md border px-1.5 py-0.5 text-faint hover:text-foreground"
				>
					<ExternalLink class="h-3 w-3" />
				</button>
			</div>
		{/each}
	</div>

	{#if rows.length === 0}
		<p class="px-1 py-3 text-sm text-muted-foreground">{m.fleet_near_empty()}</p>
	{:else if hiddenCount > 0 || expanded}
		<div class="flex items-center justify-between px-1 pt-1.5 text-[0.7rem] text-faint">
			<button
				type="button"
				data-testid="fleet-healthy-toggle"
				onclick={() => (expanded = !expanded)}
				class="hover:text-foreground"
			>
				{expanded
					? m.fleet_near_expanded({ count: healthyCount })
					: m.fleet_near_collapsed({ count: hiddenCount })}
			</button>
			<span class="font-mono">{m.fleet_near_hint()}</span>
		</div>
	{/if}
</div>
