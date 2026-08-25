// The morph bar is prop-driven: the layout passes permission-filtered
// section/overflow tables (plain data, adaptor seam); the chrome renders what
// it is given. AC-3 primary + overflow, AC-4 filtering-by-omission, and — since
// search was absorbed into the pill — AC-7 as DELEGATION: the bar owns the
// morph, the layout's palette snippet owns every search row.
import { describe, it, expect, beforeEach } from 'vitest';
import { createRawSnippet } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import { Monitor, Send, ScrollText, Users } from '@lucide/svelte';
import MorphBar from './morph-bar.svelte';
import { shell, resetShell } from '$lib/shell/shell.svelte';

beforeEach(() => resetShell());

// Labels are message ACCESSORS (see $lib/shell/nav): the chrome calls them
// while rendering so the pill follows the active locale. Fixtures are plain
// thunks — the bar must not care whether paraglide or a test produced them.
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
		// filtering by omission: the props carry no Compliance section, so no
		// Compliance link may render (the old built-in NAV_SECTIONS had one —
		// this is the discriminator that the pill renders PROPS, not a table)
		expect(document.querySelectorAll('a').length).toBe(SECTIONS.length);
		await expect.element(page.getByRole('link', { name: 'Compliance' })).not.toBeInTheDocument();
	});

	it('renders a label by CALLING its accessor, so a locale switch reaches the pill', async () => {
		// The bug this guards: nav.ts held resolved English strings, so the pill
		// read text that was frozen at import. A German label here can only appear
		// if the bar invokes the accessor while rendering.
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

	// Search is ONE surface now: the bar renders the layout's palette snippet and
	// keeps no row model of its own. The discriminator is that nothing
	// search-shaped appears unless the snippet put it there.
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
		// The bar contributes the morph and the dismiss layer only — no input, no
		// rows: the jump list it used to own moved into the palette wholesale.
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
