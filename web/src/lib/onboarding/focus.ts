

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

export function cycleTab(e: KeyboardEvent, root: HTMLElement): boolean {
	if (e.key !== 'Tab') return false;
	const items = focusables(root);
	if (items.length === 0) {

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
