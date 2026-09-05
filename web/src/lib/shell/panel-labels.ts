

import * as m from '$lib/paraglide/messages';

export const PANEL_KIND_PLURAL: Record<string, () => string> = {
	device: m.nav_devices,
	window: m.shell_windows
};

export const PANEL_KIND_SINGULAR: Record<string, () => string> = {
	device: m.shell_kind_device,
	window: m.shell_kind_window
};

export function panelKindPlural(kind: string): string {
	return PANEL_KIND_PLURAL[kind]?.() ?? kind.charAt(0).toUpperCase() + kind.slice(1) + 's';
}

export function panelKindSingular(kind: string): string {
	return PANEL_KIND_SINGULAR[kind]?.() ?? kind;
}
