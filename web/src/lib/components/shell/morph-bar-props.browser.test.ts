

import { describe, it, expect, beforeEach } from 'vitest';
import { createRawSnippet } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import { Monitor, Send, ScrollText, Users } from '@lucide/svelte';
import MorphBar from './morph-bar.svelte';
import { shell, resetShell } from '$lib/shell/shell.svelte';

beforeEach(() => resetShell());

const SECTIONS = [
	{ href: '/devices', label: () => 'Devices', icon: Monitor },
	{ href: '/actions', label: () => 'Actions', icon: Send },
	{ href: '/audit', label: () => 'Audit', icon: ScrollText }
];
const OVERFLOW = [
	{ group: () => 'Management', items: [{ href: '/device-groups', label: () => 'Device groups', icon: Monitor }] },
	{ group: () => 'Admin', items: [{ href: '/users', label: () => 'Users', icon: Users }] }
];

function mount(pathname = '/devices', overflow = OVERFLOW) {
	render(MorphBar, { pathname, sections: SECTIONS, overflow });
}

describe('prop-driven navigation', () => {
	it('renders exactly the given sections with the pathname-derived active one', async () => {
		mount('/actions');
		await expect.element(page.getByRole('link', { name: 'Devices' })).toBeVisible();
		const actions = page.getByRole('link', { name: 'Actions' });
		await expect.element(actions).toHaveAttribute('aria-current', 'page');

		expect(document.querySelectorAll('a').length).toBe(SECTIONS.length);
		await expect.element(page.getByRole('link', { name: 'Compliance' })).not.toBeInTheDocument();
	});

	it('renders a label by CALLING its accessor, so a locale switch reaches the pill', async () => {

		let calls = 0;
		render(MorphBar, {
			pathname: '/devices',
			sections: [{ href: '/devices', label: () => (calls++, 'Geräte'), icon: Monitor }]
		});
		await expect.element(page.getByRole('link', { name: 'Geräte' })).toBeVisible();
		expect(calls).toBeGreaterThan(0);
	});

	it('More ▾ opens the overflow groups and lists their items', async () => {
		mount();
		await page.getByRole('button', { name: 'More' }).click();
		await expect.element(page.getByRole('menuitem', { name: 'Users' })).toBeVisible();
		await expect.element(page.getByRole('menuitem', { name: 'Device groups' })).toBeVisible();
	});

	it('closes the overflow menu when the morph leaves nav mode', async () => {
		mount();
		await page.getByRole('button', { name: 'More' }).click();
		await expect.element(page.getByRole('menuitem', { name: 'Users' })).toBeVisible();
		shell.paletteOpen = true;
		await expect.element(page.getByTestId('pill-more-menu')).not.toBeInTheDocument();
		shell.paletteOpen = false;
		await expect.element(page.getByRole('button', { name: 'More' })).toBeVisible();
		await page.getByRole('button', { name: 'More' }).click();
		await expect.element(page.getByRole('menuitem', { name: 'Users' })).toBeVisible();
	});

	it('hides More ▾ entirely when the overflow is empty', () => {
		mount('/devices', []);
		expect(document.querySelector('[data-testid="pill-more"]')).toBeNull();
	});

	it('search mode renders the handed-in surface and nothing search-shaped of its own', async () => {
		const surface = createRawSnippet(() => ({
			render: () => '<div data-testid="palette-probe">palette</div>'
		}));
		render(MorphBar, {
			pathname: '/devices',
			sections: SECTIONS,
			overflow: OVERFLOW,
			searchSurface: surface
		});

		expect(document.querySelector('[data-testid="palette-probe"]')).toBeNull();

		shell.paletteOpen = true;
		await expect.element(page.getByTestId('palette-probe')).toBeVisible();

		expect(document.querySelectorAll('[data-testid="pill-search"] input')).toHaveLength(0);
		await expect.element(page.getByTestId('pill-search-dismiss')).toBeVisible();
	});

	it('a click on the dismiss layer leaves search mode', async () => {
		const surface = createRawSnippet(() => ({
			render: () => '<div data-testid="palette-probe">palette</div>'
		}));
		render(MorphBar, { pathname: '/devices', sections: SECTIONS, searchSurface: surface });
		shell.paletteOpen = true;
		await page.getByTestId('pill-search-dismiss').click();
		expect(shell.paletteOpen).toBe(false);
	});
});
