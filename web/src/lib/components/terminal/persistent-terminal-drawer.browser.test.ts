

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import * as m from '$lib/paraglide/messages';

const api = vi.hoisted(() => ({
	startTerminal: vi.fn(),
	stopTerminal: vi.fn()
}));

vi.mock('$lib/sdk', () => ({ apiClient: api }));

import Drawer from './persistent-terminal-drawer.svelte';
import { shell, openTerminal, focusSession, resetShell } from '$lib/shell/shell.svelte';

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

		expect(strip().scrollWidth).toBeGreaterThan(strip().clientWidth);

		expect(getComputedStyle(strip()).overflowX).toBe('auto');
		expect(strip().clientWidth).toBeLessThanOrEqual(Math.ceil(drawer().clientWidth));

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

		const last = tabs().at(-1)!;
		await vi.waitFor(() => {
			const stripBox = strip().getBoundingClientRect();
			const r = last.getBoundingClientRect();
			expect(r.right).toBeLessThanOrEqual(stripBox.right + 1);
			expect(r.left).toBeGreaterThanOrEqual(stripBox.left - 1);
		});

		focusSession(shell.terminal.sessions[0].id);
		await vi.waitFor(() => {
			const stripBox = strip().getBoundingClientRect();
			const r = tabs()[0].getBoundingClientRect();
			expect(r.left).toBeGreaterThanOrEqual(stripBox.left - 1);
			expect(r.right).toBeLessThanOrEqual(stripBox.right + 1);
		});
	});
});
