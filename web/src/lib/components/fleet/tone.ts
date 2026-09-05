

import * as m from '$lib/paraglide/messages';

export type FleetTone = 'ok' | 'warn' | 'crit' | 'info' | 'idle';

export const TONE_LABEL: Record<FleetTone, () => string> = {
	ok: m.fleet_tile_ok,
	warn: m.fleet_tile_warn,
	crit: m.fleet_tile_crit,
	info: m.fleet_tile_info,
	idle: m.fleet_tile_idle
};

export const TONE_FILL: Record<FleetTone, string> = {
	ok: 'bg-ok',
	warn: 'bg-warn',
	crit: 'bg-crit',
	info: 'bg-info',
	idle: 'bg-idle'
};

export const TONE_SOFT: Record<FleetTone, string> = {
	ok: 'bg-ok-soft text-ok',
	warn: 'bg-warn-soft text-warn',
	crit: 'bg-crit-soft text-crit',
	info: 'bg-info-soft text-info',
	idle: 'bg-sunken text-muted-foreground'
};
