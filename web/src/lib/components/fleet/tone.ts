// The five status tones the design concepts use across tiles, chips and stats.
// Status is never colour-alone: `tile.svelte` pairs each tone with a shape.
import * as m from '$lib/paraglide/messages';

export type FleetTone = 'ok' | 'warn' | 'crit' | 'info' | 'idle';

/** The WORD for a tone. Every surface that renders a tone as nothing but a
 *  colour — the fleet tile, the ⌘K palette's row dot — names it from here, so
 *  the colour is never the only carrier of the status. */
export const TONE_LABEL: Record<FleetTone, () => string> = {
	ok: m.fleet_tile_ok,
	warn: m.fleet_tile_warn,
	crit: m.fleet_tile_crit,
	info: m.fleet_tile_info,
	idle: m.fleet_tile_idle
};

/** Solid fill for a tone — the dot in a stat, the fill in a tile. */
export const TONE_FILL: Record<FleetTone, string> = {
	ok: 'bg-ok',
	warn: 'bg-warn',
	crit: 'bg-crit',
	info: 'bg-info',
	idle: 'bg-idle'
};

/** Soft plate + ink for a tone — chips and badges. */
export const TONE_SOFT: Record<FleetTone, string> = {
	ok: 'bg-ok-soft text-ok',
	warn: 'bg-warn-soft text-warn',
	crit: 'bg-crit-soft text-crit',
	info: 'bg-info-soft text-info',
	idle: 'bg-sunken text-muted-foreground'
};
