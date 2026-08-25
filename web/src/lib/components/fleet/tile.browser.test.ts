

import { describe, it, expect, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import Tile from './tile.svelte';

const tiles = () => Array.from(document.querySelectorAll<HTMLElement>('[data-testid="fleet-tile"]'));

describe('Tile — shape encoding (not colour-alone)', () => {
	it('warning carries a dot, critical a corner notch, never-seen a hollow dashed outline', () => {
		render(Tile, { tone: 'warn' });
		render(Tile, { tone: 'crit' });
		render(Tile, { tone: 'idle' });
		const [warn, crit, idle] = tiles();

		expect(warn.dataset.shape).toBe('dot');
		expect(warn.querySelector('[data-marker="dot"]')).not.toBeNull();

		expect(crit.dataset.shape).toBe('notch');
		expect(crit.querySelector('[data-marker="notch"]')).not.toBeNull();

		expect(idle.dataset.shape).toBe('hollow');
		const idleStyle = getComputedStyle(idle);
		expect(idleStyle.borderTopStyle).toBe('dashed');
		expect(idleStyle.backgroundColor).toBe('rgba(0, 0, 0, 0)');
		expect(idle.querySelector('[data-marker]')).toBeNull();
	});

	it('a healthy tile is a plain filled square — the shapes are reserved for the states that need attention', () => {
		render(Tile, { tone: 'ok' });
		const [ok] = tiles();

		expect(ok.dataset.shape).toBe('none');
		expect(ok.querySelector('[data-marker]')).toBeNull();
		expect(getComputedStyle(ok).backgroundColor).not.toBe('rgba(0, 0, 0, 0)');
	});

	it('names its status for assistive tech, prefixed with the device', async () => {
		render(Tile, { tone: 'crit', label: 'web-prod-07' });
		await expect.element(page.getByRole('img', { name: 'web-prod-07 · Critical' })).toBeVisible();
	});
});

describe('Tile — heartbeat decay', () => {
	it('fades the fill through the four decay steps', () => {
		for (const age of [0, 1, 2, 3] as const) render(Tile, { tone: 'ok', age });

		expect(tiles().map((t) => getComputedStyle(t).opacity)).toEqual(['1', '0.78', '0.55', '0.34']);
	});

	it('never fades a hollow tile — decay on nothing would say nothing', () => {
		render(Tile, { tone: 'idle', age: 3 });
		expect(getComputedStyle(tiles()[0]).opacity).toBe('1');
	});
});

describe('Tile — converging ring', () => {
	it('pulses while an operation is landing', () => {
		render(Tile, { tone: 'ok', converging: true, reducedMotion: false });
		const ring = document.querySelector<HTMLElement>('[data-marker="ring"]');

		expect(ring).not.toBeNull();
		expect(ring!.dataset.motion).toBe('pulse');
		expect(getComputedStyle(ring!).animationName).toBe('conv-pulse');
	});

	it('holds the ring STATIC under prefers-reduced-motion — the state stays visible, the motion goes', () => {
		render(Tile, { tone: 'ok', converging: true, reducedMotion: true });
		const ring = document.querySelector<HTMLElement>('[data-marker="ring"]');

		expect(ring).not.toBeNull();
		expect(ring!.dataset.motion).toBe('static');
		expect(getComputedStyle(ring!).animationName).toBe('none');
		expect(Number(getComputedStyle(ring!).opacity)).toBeGreaterThan(0.5);
	});

	it('renders no ring at rest', () => {
		render(Tile, { tone: 'ok' });
		expect(document.querySelector('[data-marker="ring"]')).toBeNull();
	});
});

describe('Tile — interaction', () => {
	it('is a real button when given a handler, and a presentational span otherwise', async () => {
		const onclick = vi.fn();
		render(Tile, { tone: 'ok', label: 'api-prod-01', onclick });

		const tile = page.getByTestId('fleet-tile');
		expect(tile.element().tagName).toBe('BUTTON');
		await tile.click();
		expect(onclick).toHaveBeenCalledTimes(1);

		render(Tile, { tone: 'ok', label: 'api-prod-02' });
		expect(tiles()[1].tagName).toBe('SPAN');
	});

	it('marks selection with an outline, not with a colour swap', () => {
		render(Tile, { tone: 'ok' });
		render(Tile, { tone: 'ok', selected: true });
		const [plain, selected] = tiles();

		expect(selected.dataset.selected).toBe('true');
		expect(getComputedStyle(selected).outlineWidth).toBe('2px');
		expect(getComputedStyle(selected).outlineStyle).not.toBe('none');
		expect(getComputedStyle(plain).outlineStyle).toBe('none');

		expect(getComputedStyle(selected).backgroundColor).toBe(getComputedStyle(plain).backgroundColor);
	});
});
