

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render } from 'vitest-browser-svelte';

const mocks = vi.hoisted(() => ({
	user: { id: 'u1', email: 'ops@example.test' } as { id: string; email: string } | null,
	serverUrl: 'https://ctl.example.test'
}));

vi.mock('$app/paths', () => ({ base: '', assets: '' }));
vi.mock('$app/navigation', () => ({
	goto: vi.fn(),
	afterNavigate: vi.fn(),
	beforeNavigate: vi.fn(),
	pushState: vi.fn(),
	replaceState: vi.fn()
}));

vi.mock('$lib/sdk', () => ({
	authStore: {
		get user() {
			return mocks.user;
		}
	},
	configStore: {
		get serverUrl() {
			return mocks.serverUrl;
		}
	},
	apiClient: {
		listDevices: vi.fn().mockResolvedValue({ devices: [], totalCount: 0, nextPageToken: '' }),
		listTokens: vi.fn().mockResolvedValue({ tokens: [] }),
		listActions: vi.fn().mockResolvedValue({ actions: [] }),
		listAssignments: vi.fn().mockResolvedValue({ assignments: [] }),
		listUsers: vi.fn().mockResolvedValue({ users: [] }),
		listIdentityProviders: vi.fn().mockResolvedValue({ providers: [] })
	}
}));

import OnboardingHost from './onboarding-host.svelte';
import { onboarding, resetOnboarding, startTour, storageKey, onboardingScope, motion } from '$lib/onboarding';

const realReduced = motion.reduced;

interface AnchorOpts {
	pill?: boolean;
	grid?: boolean;
	zoom?: boolean;
	legend?: boolean;
	summary?: boolean;

	corner?: boolean;
}

function fixed(el: HTMLElement, left: number, top: number, width: number, height: number) {
	el.style.cssText = `position:fixed;left:${left}px;top:${top}px;width:${width}px;height:${height}px;background:#888`;
}

function mountAnchors(opts: AnchorOpts = {}) {
	const root = document.createElement('div');
	root.id = 'tour-anchors';
	document.body.appendChild(root);

	if (opts.pill !== false) {
		const pill = document.createElement('div');
		pill.dataset.testid = 'pill';
		fixed(pill, 520, 12, 240, 44);
		const more = document.createElement('button');
		more.type = 'button';
		more.textContent = 'More';
		more.setAttribute('data-tour', 'nav-pill-overflow');
		more.style.cssText = 'width:60px;height:28px';
		pill.appendChild(more);
		root.appendChild(pill);
	}
	if (opts.grid) {
		const grid = document.createElement('div');
		grid.setAttribute('data-tour', 'fleet-grid');
		fixed(grid, 40, 220, 600, 300);
		root.appendChild(grid);
	}
	if (opts.zoom) {
		const zoom = document.createElement('div');
		zoom.setAttribute('data-tour', 'fleet-zoom');
		fixed(zoom, 700, 120, 120, 32);
		root.appendChild(zoom);
	}
	if (opts.legend) {
		const legend = document.createElement('div');
		legend.setAttribute('data-tour', 'fleet-legend');
		fixed(legend, 40, 560, 400, 24);
		root.appendChild(legend);
	}
	if (opts.summary) {
		const summary = document.createElement('div');
		summary.setAttribute('data-tour', 'fleet-summary');
		fixed(summary, 40, 160, 420, 40);
		root.appendChild(summary);
	}
	if (opts.corner) {
		const corner = document.createElement('div');
		corner.setAttribute('data-tour', 'fleet-zoom');
		fixed(corner, window.innerWidth - 100, window.innerHeight - 60, 90, 50);
		root.appendChild(corner);
	}
	return root;
}

const q = <T extends HTMLElement>(sel: string) => document.querySelector<T>(sel);
const card = () => q<HTMLElement>('[data-testid="tour-card"]');
const counter = () => q('[data-testid="tour-counter"]')?.textContent?.trim() ?? '';
const live = () => q('[data-testid="onboarding-live"]')?.textContent?.trim() ?? '';

function press(el: HTMLElement, key: string, init: KeyboardEventInit = {}) {
	el.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...init }));
}

let mounted: { unmount?: () => void } | null = null;

function mountHost() {
	mounted = render(OnboardingHost) as { unmount?: () => void };
	return mounted;
}

beforeEach(() => {
	localStorage.clear();
	resetOnboarding();
	motion.reduced = realReduced;
	mocks.user = { id: 'u1', email: 'ops@example.test' };
	document.body.innerHTML = '';
});

afterEach(() => {
	mounted?.unmount?.();
	mounted = null;
	motion.reduced = realReduced;
});

describe('the first-run welcome', () => {
	it('appears once for a scope and never again for the returning operator', async () => {
		mountHost();
		await vi.waitFor(() => expect(q('[data-testid="onboarding-welcome"]')).not.toBeNull());
		expect(localStorage.getItem(storageKey(onboardingScope(mocks.serverUrl, 'u1')))).toContain('welcomeSeen');

		mounted?.unmount?.();
		document.body.innerHTML = '';
		resetOnboarding();
		mountHost();
		await new Promise((r) => setTimeout(r, 60));
		expect(q('[data-testid="onboarding-welcome"]')).toBeNull();
	});

	it('is a fresh first run for a different user on the same server', async () => {
		mountHost();
		await vi.waitFor(() => expect(q('[data-testid="onboarding-welcome"]')).not.toBeNull());

		mounted?.unmount?.();
		document.body.innerHTML = '';
		resetOnboarding();
		mocks.user = { id: 'u2', email: 'other@example.test' };
		mountHost();
		await vi.waitFor(() => expect(q('[data-testid="onboarding-welcome"]')).not.toBeNull());
	});

	it('"Explore on my own" closes it and starts nothing', async () => {
		mountAnchors({ grid: true, zoom: true, legend: true });
		mountHost();
		await vi.waitFor(() => expect(q('[data-testid="onboarding-welcome"]')).not.toBeNull());

		q<HTMLButtonElement>('[data-testid="onboarding-welcome-dismiss"]')!.click();

		await vi.waitFor(() => expect(q('[data-testid="onboarding-welcome"]')).toBeNull());
		await new Promise((r) => setTimeout(r, 60));
		expect(onboarding.running).toBe(false);
		expect(card()).toBeNull();
	});

	it('"Take the tour" hands straight over to the first coach mark', async () => {
		mountAnchors({ grid: true, zoom: true, legend: true });
		mountHost();
		await vi.waitFor(() => expect(q('[data-testid="onboarding-welcome"]')).not.toBeNull());

		q<HTMLButtonElement>('[data-testid="onboarding-welcome-start"]')!.click();

		await vi.waitFor(() => expect(card()).not.toBeNull());
		expect(q('[data-testid="onboarding-welcome"]')).toBeNull();
		expect(card()!.dataset.step).toBe('pill');
	});
});

describe('the coach-mark tour', () => {
	it('walks forward and back over the steps whose anchors are present', async () => {
		mountAnchors({ grid: true, zoom: true, legend: true, summary: true });
		mountHost();
		startTour();

		await vi.waitFor(() => expect(card()).not.toBeNull());

		expect(onboarding.steps.map((s) => s.id)).toEqual([
			'pill',
			'search',
			'tiles',
			'summary',
			'zoom',
			'selection',
			'stage',
			'more'
		]);
		expect(counter()).toBe('Step 1 of 8');
		expect(q<HTMLButtonElement>('[data-testid="tour-back"]')!.disabled).toBe(true);

		q<HTMLButtonElement>('[data-testid="tour-next"]')!.click();
		await vi.waitFor(() => expect(card()!.dataset.step).toBe('search'));
		expect(counter()).toBe('Step 2 of 8');

		q<HTMLButtonElement>('[data-testid="tour-back"]')!.click();
		await vi.waitFor(() => expect(card()!.dataset.step).toBe('pill'));
		expect(counter()).toBe('Step 1 of 8');
	});

	it('reads Done on the last step and marks the tour completed', async () => {
		mountAnchors({ pill: false, zoom: true });
		mountHost();
		startTour();

		await vi.waitFor(() => expect(card()).not.toBeNull());
		expect(onboarding.steps.map((s) => s.id)).toEqual(['zoom']);
		expect(counter()).toBe('Step 1 of 1');
		expect(q('[data-testid="tour-next"]')!.textContent!.trim()).toBe('Got it');

		q<HTMLButtonElement>('[data-testid="tour-next"]')!.click();
		await vi.waitFor(() => expect(card()).toBeNull());
		expect(onboarding.flags.tourCompleted).toBe(true);
	});

	it('drops a step whose anchor is absent instead of pointing at nothing', async () => {

		mountAnchors({ grid: true });
		mountHost();
		startTour();

		await vi.waitFor(() => expect(card()).not.toBeNull());
		expect(onboarding.steps.map((s) => s.id)).toEqual([
			'pill',
			'search',
			'tiles',
			'selection',
			'stage',
			'more'
		]);
		expect(counter()).toBe('Step 1 of 6');

		const seen: string[] = [];
		for (let i = 0; i < 6; i++) {
			seen.push(card()!.dataset.step!);
			q<HTMLButtonElement>('[data-testid="tour-next"]')!.click();
			await new Promise((r) => setTimeout(r, 20));
		}
		expect(seen).not.toContain('zoom');
		expect(seen).not.toContain('summary');
		expect(card()).toBeNull();
	});

	it('does not open at all when nothing on the page can be pointed at', async () => {
		mountAnchors({ pill: false });
		mountHost();

		expect(startTour()).toBe(false);
		await new Promise((r) => setTimeout(r, 40));
		expect(card()).toBeNull();
		expect(onboarding.running).toBe(false);
	});

	it('skips out and says where to find the tour again', async () => {
		mountAnchors({ grid: true, zoom: true, legend: true });
		mountHost();
		startTour();
		await vi.waitFor(() => expect(card()).not.toBeNull());

		q<HTMLButtonElement>('[data-testid="tour-skip"]')!.click();

		await vi.waitFor(() => expect(card()).toBeNull());
		expect(onboarding.flags.tourCompleted).toBe(false);
		expect(live()).toContain('Settings');
	});
});

describe('the coach-mark card is reachable', () => {
	it('is a labelled dialog that takes focus and cycles Tab inside itself', async () => {
		mountAnchors({ grid: true, zoom: true, legend: true });
		mountHost();
		startTour();
		await vi.waitFor(() => expect(card()).not.toBeNull());

		const c = card()!;
		expect(c.getAttribute('role')).toBe('dialog');
		expect(document.getElementById(c.getAttribute('aria-labelledby')!)).not.toBeNull();
		expect(document.getElementById(c.getAttribute('aria-describedby')!)).not.toBeNull();
		await vi.waitFor(() => expect(document.activeElement).toBe(c));

		const skip = q<HTMLButtonElement>('[data-testid="tour-skip"]')!;
		const next = q<HTMLButtonElement>('[data-testid="tour-next"]')!;

		next.focus();
		press(next, 'Tab');
		expect(document.activeElement).toBe(skip);

		press(skip, 'Tab', { shiftKey: true });
		expect(document.activeElement).toBe(next);
	});

	it('leaves the spotlight out of the accessibility tree', async () => {
		mountAnchors({ zoom: true, pill: false });
		mountHost();
		startTour();
		await vi.waitFor(() => expect(q('[data-testid="tour-spotlight"]')).not.toBeNull());

		const ring = q('[data-testid="tour-spotlight"]')!;
		expect(ring.getAttribute('aria-hidden')).toBe('true');

		expect(getComputedStyle(ring).pointerEvents).toBe('none');
	});

	it('Esc skips the tour and announces the restart hint', async () => {
		mountAnchors({ grid: true, zoom: true, legend: true });
		mountHost();
		startTour();
		await vi.waitFor(() => expect(card()).not.toBeNull());

		press(card()!, 'Escape');

		await vi.waitFor(() => expect(card()).toBeNull());
		expect(live()).toContain('Settings');
	});

	it('stays inside the viewport and off its own anchor for an anchor in the corner', async () => {
		mountAnchors({ pill: false, corner: true });
		mountHost();
		startTour();
		await vi.waitFor(() => expect(card()).not.toBeNull());

		await new Promise((r) => setTimeout(r, 80));

		const c = card()!.getBoundingClientRect();
		const a = q('[data-tour="fleet-zoom"]')!.getBoundingClientRect();
		expect(c.left, 'left').toBeGreaterThanOrEqual(0);
		expect(c.top, 'top').toBeGreaterThanOrEqual(0);
		expect(c.right, 'right').toBeLessThanOrEqual(window.innerWidth);
		expect(c.bottom, 'bottom').toBeLessThanOrEqual(window.innerHeight);
		const covers = c.left < a.right && a.left < c.right && c.top < a.bottom && a.top < c.bottom;
		expect(covers, 'card covers its own anchor').toBe(false);
	});
});

describe('prefers-reduced-motion', () => {
	it('animates the spotlight by default — the positive control', async () => {
		motion.reduced = () => false;
		mountAnchors({ zoom: true, pill: false });
		mountHost();
		startTour();
		await vi.waitFor(() => expect(q('[data-testid="tour-spotlight"]')).not.toBeNull());

		const ring = q<HTMLElement>('[data-testid="tour-spotlight"]')!;
		expect(ring.dataset.motion).toBe('full');

		expect(getComputedStyle(ring).transitionDuration).toContain('0.22s');
		expect(ring.querySelector('.animate-pulse')).not.toBeNull();
	});

	it('removes every transition and pulse when reduced motion is requested', async () => {
		motion.reduced = () => true;
		mountAnchors({ zoom: true, pill: false });
		mountHost();
		startTour();
		await vi.waitFor(() => expect(q('[data-testid="tour-spotlight"]')).not.toBeNull());

		const ring = q<HTMLElement>('[data-testid="tour-spotlight"]')!;
		expect(ring.dataset.motion).toBe('reduced');
		expect(ring.getAttribute('style')).toMatch(/transition:\s*none/);
		expect(ring.querySelector('.animate-pulse')).toBeNull();
		expect(card()!.dataset.motion).toBe('reduced');
		expect(getComputedStyle(ring).transitionDuration).toBe('0s');
	});
});
