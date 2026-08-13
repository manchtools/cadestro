// Moving-window interaction tests: header drag and keyboard movement.
// AC-1 header-only drag (body inert, buttons still fire — see panel.browser.test.ts),
// AC-2 clamped moves, AC-3 zone state while dragging, AC-4 dock on release,
// AC-5 arrows / Shift-arrows / Escape.
import { describe, it, expect, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import Panel from './panel.svelte';
import { shell, resetShell, openPanel, setShellBounds, PANEL_W } from '$lib/shell/shell.svelte';

beforeEach(() => {
	resetShell();
	setShellBounds(1280, 800);
});

function mountPanel() {
	const id = openPanel('window', 'w1', 'nginx-prod');
	const panel = shell.panels.find((p) => p.id === id)!;
	render(Panel, { panel });
	return id;
}

const header = () => document.querySelector('[data-testid="panel-header"]') as HTMLElement;
const p = (id: string) => shell.panels.find((x) => x.id === id)!;

function pt(el: Element, type: string, x: number, y: number) {
	el.dispatchEvent(new PointerEvent(type, { bubbles: true, pointerId: 1, clientX: x, clientY: y }));
}
function key(el: Element, k: string, shiftKey = false) {
	el.dispatchEvent(new KeyboardEvent('keydown', { key: k, shiftKey, bubbles: true }));
}

describe('header drag (AC-1, AC-2)', () => {
	it('dragging the header moves the panel by the pointer delta', () => {
		const id = mountPanel();
		const { x: x0, y: y0 } = p(id);
		const h = header();
		pt(h, 'pointerdown', 200, 200);
		pt(h, 'pointermove', 240, 230);
		pt(h, 'pointerup', 240, 230);
		expect(p(id).x).toBe(x0 + 40);
		expect(p(id).y).toBe(y0 + 30);
	});

	it('pressing and moving on the panel BODY does not drag', () => {
		const id = mountPanel();
		const { x: x0, y: y0 } = p(id);
		const body = document.querySelector('[data-testid="panel"] .overflow-y-auto') as HTMLElement;
		pt(body, 'pointerdown', 200, 300);
		pt(body, 'pointermove', 300, 400);
		pt(body, 'pointerup', 300, 400);
		expect(p(id).x).toBe(x0);
		expect(p(id).y).toBe(y0);
	});

	it('a drag past the viewport edge is clamped mid-drag, then docks on the edge release', () => {
		const id = mountPanel();
		const h = header();
		pt(h, 'pointerdown', 200, 200);
		pt(h, 'pointermove', -2000, -2000);
		// mid-drag: clamped to bounds, still free
		expect(p(id).x).toBe(8);
		expect(p(id).y).toBe(88);
		// releasing slammed against the left edge = deliberate edge gesture → docks
		pt(h, 'pointerup', -2000, -2000);
		expect(p(id).slot).toBe('left');
		expect(p(id).x).toBe(16);
	});
});

describe('snap zones (AC-3, AC-4)', () => {
	it('dragging into the left region sets the drag slot; release docks and clears', () => {
		const id = mountPanel();
		const x0 = p(id).x;
		// move so the panel center lands well inside the left region (< 40% of 1280)
		const dx = 20 - x0; // panel left edge → 20, center → 20 + PANEL_W/2 = 212
		const h = header();
		pt(h, 'pointerdown', 400, 300);
		pt(h, 'pointermove', 400 + dx, 300);
		expect(shell.drag.panelId).toBe(id);
		expect(shell.drag.slot).toBe('left');
		pt(h, 'pointerup', 400 + dx, 300);
		expect(p(id).slot).toBe('left');
		expect(p(id).x).toBe(16);
		expect(shell.drag).toEqual({ panelId: null, slot: null });
	});

	it('release outside any region keeps the free position', () => {
		const id = mountPanel();
		const x0 = p(id).x;
		// center around 600/300 → null region at 1280x800
		const dx = 600 - PANEL_W / 2 - x0;
		const h = header();
		pt(h, 'pointerdown', 400, 300);
		pt(h, 'pointermove', 400 + dx, 300);
		expect(shell.drag.slot).toBeNull();
		pt(h, 'pointerup', 400 + dx, 300);
		expect(p(id).slot).toBe('free');
		expect(p(id).x).toBe(x0 + dx);
	});
});

describe('keyboard movement (AC-5)', () => {
	it('arrows move 16px, Shift+arrows 48px, clamped', () => {
		const id = mountPanel();
		const { x: x0, y: y0 } = p(id);
		const h = header();
		key(h, 'ArrowRight');
		expect(p(id).x).toBe(x0 + 16);
		key(h, 'ArrowDown', true);
		expect(p(id).y).toBe(y0 + 48);
		key(h, 'ArrowLeft', true);
		key(h, 'ArrowLeft', true);
		key(h, 'ArrowLeft', true);
		key(h, 'ArrowLeft', true);
		key(h, 'ArrowLeft', true);
		expect(p(id).x).toBe(8); // clamped at the left edge
	});

	it('Escape parks the panel to the stage', () => {
		const id = mountPanel();
		key(header(), 'Escape');
		expect(p(id).minimized).toBe(true);
	});
});
