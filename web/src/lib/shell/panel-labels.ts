// Stage-rail captions for parked panels. `ShellPanel.kind` is an OPEN string
// (shell.svelte.ts hands `openPanel` whatever the caller names), so these are
// lookups with an honest fallback rather than a closed enum: a kind nobody has
// named yet degrades to its own name instead of rendering a wrong caption.
//
// Like the nav tables, the entries hold message FUNCTIONS: the rail resolves a
// caption while it renders, in the locale that is active then, never at import.
import * as m from '$lib/paraglide/messages';

/** Stack caption — a category standing in for several parked windows. */
export const PANEL_KIND_PLURAL: Record<string, () => string> = {
	device: m.nav_devices,
	execution: m.nav_executions,
	window: m.shell_windows
};

/** Card subtitle — the single panel's own kind. */
export const PANEL_KIND_SINGULAR: Record<string, () => string> = {
	device: m.shell_kind_device,
	execution: m.shell_kind_execution,
	window: m.shell_kind_window
};

/** "Devices". An unnamed kind keeps the English pluralisation the rail used
 *  before it was translated — visible and obviously untranslated beats blank. */
export function panelKindPlural(kind: string): string {
	return PANEL_KIND_PLURAL[kind]?.() ?? kind.charAt(0).toUpperCase() + kind.slice(1) + 's';
}

/** "device". An unnamed kind shows raw, exactly as it did before. */
export function panelKindSingular(kind: string): string {
	return PANEL_KIND_SINGULAR[kind]?.() ?? kind;
}
