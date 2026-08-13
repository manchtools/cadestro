// Focus containment for the onboarding dialogs.
//
// Tab (and Shift+Tab) cycle inside the card. Focus is NOT stolen back on
// pointer interaction: the tour's backdrop is decoration and the app underneath
// stays usable, so yanking focus away from a click the operator made on purpose
// would be worse than the leak it prevents.

const FOCUSABLE = [
	'a[href]',
	'button:not([disabled])',
	'input:not([disabled]):not([type="hidden"])',
	'select:not([disabled])',
	'textarea:not([disabled])',
	'[tabindex]:not([tabindex="-1"])'
].join(',');

export function focusables(root: HTMLElement): HTMLElement[] {
	return Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
		(el) => el.getClientRects().length > 0
	);
}

/** Handle a Tab keydown inside `root`. Returns true when the event was consumed
 *  (the caller then preventDefault()s). */
export function cycleTab(e: KeyboardEvent, root: HTMLElement): boolean {
	if (e.key !== 'Tab') return false;
	const items = focusables(root);
	if (items.length === 0) {
		// Nothing to move to — keep focus on the card rather than letting Tab
		// wander into the page behind an open dialog.
		root.focus();
		return true;
	}
	const first = items[0];
	const last = items[items.length - 1];
	const active = document.activeElement;
	const inside = active instanceof HTMLElement && root.contains(active);
	if (!inside) {
		(e.shiftKey ? last : first).focus();
		return true;
	}
	if (e.shiftKey && active === first) {
		last.focus();
		return true;
	}
	if (!e.shiftKey && active === last) {
		first.focus();
		return true;
	}
	return false;
}
