// The keep-alive terminal drawer is a FIXED-WIDTH, clipping box: 48rem wide at
// most, `overflow-hidden`. Its session tabs are the only way to reach a running
// session, so anything the tab strip pushes past that edge is a session the
// operator can neither read, focus, nor disconnect. These tests pin the strip's
// containment: it scrolls on its own axis, every tab stays inside the drawer,
// the minimise control is never displaced by the tabs, and the tab for the
// session actually on screen is brought into the strip's window.
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import * as m from '$lib/paraglide/messages';

const api = vi.hoisted(() => ({
	startTerminal: vi.fn(),
	stopTerminal: vi.fn()
}));

// The drawer's whole RPC surface. `startTerminal` never settles, so the strip is
// exercised without a websocket — the tabs render from store state alone.
vi.mock('$lib/sdk', () => ({ apiClient: api }));

import Drawer from './persistent-terminal-drawer.svelte';
import { shell, openTerminal, focusSession, resetShell } from '$lib/shell/shell.svelte';

/** Long enough that `max-w-44` really bites — the fleet's device names are FQDNs. */
const NAME = (i: number) => `workstation-nuremberg-lab-${i}.example.internal`;

const drawer = () => document.querySelector<HTMLElement>('[data-testid="terminal"]')!;
const strip = () => document.querySelector<HTMLElement>('[data-testid="terminal-tabs"]')!;
const tabs = () => Array.from(document.querySelectorAll<HTMLElement>('[data-session-tab]'));
const minimise = () =>
	Array.from(drawer().querySelectorAll<HTMLElement>('button')).find(
		(b) => b.getAttribute('aria-label') === m.shell_minimise_terminal()
	)!;

async function mountWith(count: number) {
	for (let i = 0; i < count; i++) openTerminal(`device-${i}`, NAME(i));
	shell.terminal.open = true;
	render(Drawer);
	await vi.waitFor(() => expect(tabs().length).toBe(count));
}

beforeEach(() => {
	document.body.innerHTML = '';
	api.startTerminal.mockReset();
	api.startTerminal.mockImplementation(() => new Promise(() => {}));
	api.stopTerminal.mockReset();
	api.stopTerminal.mockResolvedValue({});
	resetShell();
});

describe('the session tab strip stays inside the drawer', () => {
	it('scrolls its own axis instead of pushing tabs past the drawer’s clipped edge', async () => {
		await mountWith(8);

		// the premise: eight FQDN tabs really are wider than the drawer
		expect(strip().scrollWidth).toBeGreaterThan(strip().clientWidth);
		// …and the strip, not the drawer, is what absorbs that
		expect(getComputedStyle(strip()).overflowX).toBe('auto');
		expect(strip().clientWidth).toBeLessThanOrEqual(Math.ceil(drawer().clientWidth));

		// every tab is reachable: scrolling the strip to either end lands it whole
		// inside the drawer's box.
		const inside = (el: HTMLElement) => {
			const box = drawer().getBoundingClientRect();
			const r = el.getBoundingClientRect();
			return r.left >= box.left - 1 && r.right <= box.right + 1;
		};

		strip().scrollLeft = 0;
		await vi.waitFor(() => expect(inside(tabs()[0])).toBe(true));

		strip().scrollLeft = strip().scrollWidth;
		await vi.waitFor(() => expect(inside(tabs().at(-1)!)).toBe(true));
	});

	it('never lets the tabs displace the minimise control', async () => {
		await mountWith(8);

		const control = minimise().getBoundingClientRect();
		const box = drawer().getBoundingClientRect();
		expect(control.right).toBeLessThanOrEqual(box.right + 1);
		expect(control.left).toBeGreaterThanOrEqual(box.left - 1);
		// it sits after the strip, not among the tabs
		expect(control.left).toBeGreaterThanOrEqual(strip().getBoundingClientRect().right - 1);
	});

	it('keeps a strip that fits from scrolling at all', async () => {
		await mountWith(2);

		expect(strip().scrollWidth).toBeLessThanOrEqual(strip().clientWidth + 1);
		expect(tabs().every((t) => t.getBoundingClientRect().right <= drawer().getBoundingClientRect().right + 1)).toBe(true);
	});
});

describe('the strip follows the session the drawer is showing', () => {
	it('brings the active tab into the strip’s window when it is out of view', async () => {
		await mountWith(8);

		// the last-opened session is the active one, and the strip scrolled to it
		const last = tabs().at(-1)!;
		await vi.waitFor(() => {
			const stripBox = strip().getBoundingClientRect();
			const r = last.getBoundingClientRect();
			expect(r.right).toBeLessThanOrEqual(stripBox.right + 1);
			expect(r.left).toBeGreaterThanOrEqual(stripBox.left - 1);
		});

		// focusing a session at the other end pulls its tab back into view
		focusSession(shell.terminal.sessions[0].id);
		await vi.waitFor(() => {
			const stripBox = strip().getBoundingClientRect();
			const r = tabs()[0].getBoundingClientRect();
			expect(r.left).toBeGreaterThanOrEqual(stripBox.left - 1);
			expect(r.right).toBeLessThanOrEqual(stripBox.right + 1);
		});
	});
});
