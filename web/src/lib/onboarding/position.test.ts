

import { describe, it, expect } from 'vitest';
import { placeCard, isOnScreen, VIEWPORT_MARGIN, type Box } from './position';

const VP = { width: 1000, height: 800 };
const CARD = { width: 300, height: 200 };
const box = (x: number, y: number, width: number, height: number): Box => ({ x, y, width, height });

function cardBox(anchor: Box, card = CARD, vp = VP): Box {
	const p = placeCard(anchor, card, vp);
	return { x: p.x, y: p.y, width: card.width, height: card.height };
}

function overlaps(a: Box, b: Box): boolean {
	return (
		a.x < b.x + b.width && b.x < a.x + a.width && a.y < b.y + b.height && b.y < a.y + a.height
	);
}

describe('placeCard', () => {
	it('sits below an anchor with room underneath', () => {
		const anchor = box(400, 100, 120, 40);
		const p = placeCard(anchor, CARD, VP);
		expect(p.side).toBe('bottom');
		expect(p.overlaps).toBe(false);
		expect(p.y).toBeGreaterThan(anchor.y + anchor.height);
	});

	it('flips above an anchor pinned to the bottom edge rather than covering it', () => {
		const anchor = box(400, 740, 120, 40);
		const p = placeCard(anchor, CARD, VP);
		expect(p.side).toBe('top');
		expect(p.overlaps).toBe(false);
		expect(overlaps(cardBox(anchor), anchor)).toBe(false);
	});

	it.each([
		['right edge', box(960, 300, 40, 40)],
		['left edge', box(0, 300, 40, 40)],
		['top-left corner', box(0, 0, 40, 40)],
		['bottom-right corner', box(960, 760, 40, 40)],
		['zero-size anchor', box(500, 400, 0, 0)]
	])('keeps the card fully inside the viewport for an anchor at the %s', (_label, anchor) => {
		const placed = cardBox(anchor);
		expect(isOnScreen(placed, VP), JSON.stringify(placed)).toBe(true);
		expect(placed.x).toBeGreaterThanOrEqual(VIEWPORT_MARGIN);
		expect(placed.y).toBeGreaterThanOrEqual(VIEWPORT_MARGIN);
	});

	it.each([
		['right edge', box(960, 300, 40, 40)],
		['left edge', box(0, 300, 40, 40)],
		['top-left corner', box(0, 0, 40, 40)],
		['bottom-right corner', box(960, 760, 40, 40)]
	])('never covers its own anchor at the %s', (_label, anchor) => {
		expect(overlaps(cardBox(anchor), anchor)).toBe(false);
	});

	it('stays on screen and admits the overlap when the anchor fills the viewport', () => {
		const anchor = box(0, 0, VP.width, VP.height);
		const p = placeCard(anchor, CARD, VP);

		expect(p.overlaps).toBe(true);
		expect(isOnScreen({ x: p.x, y: p.y, ...CARD }, VP)).toBe(true);
	});

	it('pins the card start on screen when it is wider than the viewport', () => {
		const narrow = { width: 320, height: 200 };
		const p = placeCard(box(150, 100, 20, 20), narrow, { width: 300, height: 800 });

		expect(p.x).toBe(VIEWPORT_MARGIN);
	});
});
