// Moving-window tests: slots, cap, and stage overflow.
// AC-2 clamp, AC-3/4 slot resolution + snap, AC-5 (keyboard uses movePanel),
// AC-6 LRU cap + announcement, AC-7 restore semantics.
import { beforeEach, describe, expect, it } from 'vitest';
import {
	shell,
	resetShell,
	openPanel,
	minimizePanel,
	restorePanel,
	touchPanel,
	movePanel,
	snapPanel,
	slotForCenter,
	setShellBounds,
	WINDOW_CAP,
	PANEL_W
} from './shell.svelte';

const live = () => shell.panels.filter((p) => !p.minimized).map((p) => p.id);

beforeEach(() => {
	resetShell();
	setShellBounds(1280, 800);
});

describe('cap + LRU auto-stash (AC-6, AC-7)', () => {
	it('caps live windows at WINDOW_CAP by stashing the least-recently-touched', () => {
		expect(WINDOW_CAP).toBe(3);
		const a = openPanel('window', 'a', 'A');
		const b = openPanel('window', 'b', 'B');
		const c = openPanel('window', 'c', 'C');
		// interact: a is oldest, then b, then c; touch a → b becomes LRU
		touchPanel(a);
		const d = openPanel('window', 'd', 'D');
		expect(live()).toHaveLength(WINDOW_CAP);
		expect(live()).toContain(d);
		expect(shell.panels.find((p) => p.id === b)?.minimized).toBe(true);
		expect(shell.panels.find((p) => p.id === a)?.minimized).toBe(false);
		expect(shell.panels.find((p) => p.id === c)?.minimized).toBe(false);
	});

	it('announces an auto-stash by title; manual minimize does not announce', () => {
		openPanel('window', 'a', 'Window A');
		openPanel('window', 'b', 'Window B');
		openPanel('window', 'c', 'Window C');
		expect(shell.announcement).toBe('');
		openPanel('window', 'd', 'Window D');
		expect(shell.announcement).toContain('Window A');
		shell.announcement = '';
		minimizePanel('window:d');
		expect(shell.announcement).toBe('');
	});

	it('restore counts against the cap and never evicts the just-restored panel (AC-7)', () => {
		const a = openPanel('window', 'a', 'A');
		minimizePanel(a);
		const b = openPanel('window', 'b', 'B');
		const c = openPanel('window', 'c', 'C');
		const d = openPanel('window', 'd', 'D');
		expect(live()).toEqual(expect.arrayContaining([b, c, d]));
		restorePanel(a);
		expect(shell.panels.find((p) => p.id === a)?.minimized).toBe(false);
		// b was the least-recently-touched live panel → it parks, a stays live
		expect(shell.panels.find((p) => p.id === b)?.minimized).toBe(true);
		expect(live()).toHaveLength(WINDOW_CAP);
	});

	it('re-opening an existing id refocuses without duplicating (001 semantics kept)', () => {
		openPanel('window', 'a', 'A');
		openPanel('window', 'a', 'A');
		expect(shell.panels).toHaveLength(1);
	});
});

describe('movePanel clamps to bounds (AC-2)', () => {
	it('clamps x and y so the header stays reachable', () => {
		const a = openPanel('window', 'a', 'A');
		movePanel(a, -500, -500);
		let p = shell.panels.find((x) => x.id === a)!;
		expect(p.x).toBe(8);
		expect(p.y).toBe(88);
		movePanel(a, 5000, 5000);
		p = shell.panels.find((x) => x.id === a)!;
		expect(p.x).toBe(1280 - PANEL_W - 8);
		expect(p.y).toBe(800 - 56);
	});

	it('a free move resets the slot to free', () => {
		const a = openPanel('window', 'a', 'A');
		snapPanel(a, 'left');
		movePanel(a, 300, 300);
		expect(shell.panels.find((x) => x.id === a)?.slot).toBe('free');
	});
});

describe('slot resolution + snap (AC-3, AC-4)', () => {
	it('resolves left / right / corner by edge proximity, else null', () => {
		// bounds 1280x800: left < 256; right > 1024 with cy <= 480; corner > 1024 with cy > 480
		expect(slotForCenter(200, 300)).toBe('left');
		expect(slotForCenter(1100, 300)).toBe('right');
		expect(slotForCenter(1100, 700)).toBe('corner');
		expect(slotForCenter(400, 300)).toBeNull();
		expect(slotForCenter(600, 300)).toBeNull();
	});

	it('snapPanel docks to the slot geometry and records the slot', () => {
		const a = openPanel('window', 'a', 'A');
		snapPanel(a, 'left');
		let p = shell.panels.find((x) => x.id === a)!;
		expect(p.slot).toBe('left');
		expect(p.x).toBe(16);
		snapPanel(a, 'right');
		p = shell.panels.find((x) => x.id === a)!;
		expect(p.slot).toBe('right');
		expect(p.x).toBe(1280 - PANEL_W - 16);
		snapPanel(a, 'corner');
		p = shell.panels.find((x) => x.id === a)!;
		expect(p.slot).toBe('corner');
		expect(p.y).toBeGreaterThan(400);
	});
});

describe('drag state + reset', () => {
	it('resetShell clears drag state and announcement', () => {
		openPanel('window', 'a', 'A');
		shell.drag = { panelId: 'window:a', slot: 'left' };
		shell.announcement = 'x';
		resetShell();
		expect(shell.drag).toEqual({ panelId: null, slot: null });
		expect(shell.announcement).toBe('');
	});
});
