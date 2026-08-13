// Regression: the spotlight must hug its anchor's real shape. A fully-rounded
// pill anchor got a hardcoded 12px rectangle plus a second border nested 2px
// inside it — two visibly separate outlines around one pill.
import { describe, expect, it, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { resetOnboarding, startTour } from '$lib/onboarding/tour.svelte';
import type { TourStep } from '$lib/onboarding/steps';
import TourOverlay from './tour-overlay.svelte';

const STEP: TourStep = {
	id: 'ring-probe',
	anchors: [{ sel: '[data-tour="ring-probe"]' }],
	title: () => 'probe',
	body: () => 'probe'
};

function mountAnchor(borderRadius: string) {
	const el = document.createElement('div');
	el.dataset.tour = 'ring-probe';
	el.style.cssText = `position:fixed;left:120px;top:80px;width:320px;height:44px;border-radius:${borderRadius};`;
	document.body.appendChild(el);
	return el;
}

async function spotlight(): Promise<HTMLElement> {
	let el: HTMLElement | null = null;
	await expect
		.poll(() => (el = document.querySelector('[data-testid="tour-spotlight"]')))
		.not.toBeNull();
	return el!;
}

describe('tour spotlight ring geometry', () => {
	beforeEach(() => {
		resetOnboarding();
		document.querySelectorAll('[data-tour="ring-probe"]').forEach((n) => n.remove());
	});

	it('grows the anchor radius by the ring padding — a pill gets a pill ring', async () => {
		mountAnchor('26px');
		render(TourOverlay);
		expect(startTour([STEP])).toBe(true);
		const ring = await spotlight();
		// RING_PAD is 5: 26px anchor → 31px ring, concentric curves.
		expect(getComputedStyle(ring).borderRadius).toBe('31px');
	});

	it('renders the pulse on the main border, never as a second nested outline', async () => {
		mountAnchor('26px');
		render(TourOverlay);
		expect(startTour([STEP])).toBe(true);
		const ring = await spotlight();
		const pulse = ring.querySelector('span');
		if (!pulse) return; // reduced-motion environments render no pulse at all
		expect(getComputedStyle(pulse).borderRadius).toBe(getComputedStyle(ring).borderRadius);
		const rr = ring.getBoundingClientRect();
		const pr = pulse.getBoundingClientRect();
		// Coincides with the ring's own border box (2px outward inset), so the
		// two borders overlay into one glowing edge.
		expect(Math.abs(pr.left - rr.left)).toBeLessThanOrEqual(1);
		expect(Math.abs(pr.top - rr.top)).toBeLessThanOrEqual(1);
		expect(Math.abs(pr.width - rr.width)).toBeLessThanOrEqual(2);
	});

	it('keeps a soft default for square anchors', async () => {
		mountAnchor('0px');
		render(TourOverlay);
		expect(startTour([STEP])).toBe(true);
		const ring = await spotlight();
		expect(getComputedStyle(ring).borderRadius).toBe('12px');
	});
});
