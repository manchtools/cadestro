

export interface Box {
	x: number;
	y: number;
	width: number;
	height: number;
}

export interface Size {
	width: number;
	height: number;
}

export type Side = 'bottom' | 'top' | 'right' | 'left';

export interface Placement {
	x: number;
	y: number;
	side: Side;

	overlaps: boolean;
}

export const ANCHOR_GAP = 14;

export const VIEWPORT_MARGIN = 12;

const SIDES: Side[] = ['bottom', 'top', 'right', 'left'];

function clamp(v: number, lo: number, hi: number): number {

	return Math.max(lo, Math.min(v, hi));
}

function overlapArea(a: Box, b: Box): number {
	const w = Math.min(a.x + a.width, b.x + b.width) - Math.max(a.x, b.x);
	const h = Math.min(a.y + a.height, b.y + b.height) - Math.max(a.y, b.y);
	return w > 0 && h > 0 ? w * h : 0;
}

function candidate(side: Side, anchor: Box, card: Size, viewport: Size): Box {
	const maxX = viewport.width - card.width - VIEWPORT_MARGIN;
	const maxY = viewport.height - card.height - VIEWPORT_MARGIN;
	let x: number;
	let y: number;
	if (side === 'bottom' || side === 'top') {
		x = anchor.x + anchor.width / 2 - card.width / 2;
		y = side === 'bottom' ? anchor.y + anchor.height + ANCHOR_GAP : anchor.y - ANCHOR_GAP - card.height;
	} else {
		x = side === 'right' ? anchor.x + anchor.width + ANCHOR_GAP : anchor.x - ANCHOR_GAP - card.width;
		y = anchor.y + anchor.height / 2 - card.height / 2;
	}
	return {
		x: clamp(x, VIEWPORT_MARGIN, maxX),
		y: clamp(y, VIEWPORT_MARGIN, maxY),
		width: card.width,
		height: card.height
	};
}

export function placeCard(anchor: Box, card: Size, viewport: Size): Placement {
	let best: { box: Box; side: Side; area: number } | null = null;
	for (const side of SIDES) {
		const box = candidate(side, anchor, card, viewport);
		const area = overlapArea(box, anchor);
		if (area === 0) return { x: box.x, y: box.y, side, overlaps: false };
		if (!best || area < best.area) best = { box, side, area };
	}

	return { x: best!.box.x, y: best!.box.y, side: best!.side, overlaps: true };
}

export function isOnScreen(box: Box, viewport: Size): boolean {
	return box.x >= 0 && box.y >= 0 && box.x + box.width <= viewport.width && box.y + box.height <= viewport.height;
}
