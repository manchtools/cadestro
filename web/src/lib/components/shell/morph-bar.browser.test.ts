import { describe, it, expect, vi, beforeEach } from 'vitest';
import { createRawSnippet } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import { Monitor, Send, ShieldCheck, ScrollText } from '@lucide/svelte';
import * as m from '$lib/paraglide/messages';
import MorphBar from './morph-bar.svelte';
import { shell, resetShell } from '$lib/shell/shell.svelte';

beforeEach(() => resetShell());

// The layout passes permission-filtered navigation; tests use a fixture set.
// Labels are message accessors, called at render time (see $lib/shell/nav).
const SECTIONS = [
	{ href: '/devices', label: () => 'Devices', icon: Monitor },
	{ href: '/actions', label: () => 'Actions', icon: Send },
	{ href: '/compliance-policies', label: () => 'Compliance', icon: ShieldCheck },
	{ href: '/audit', label: () => 'Audit', icon: ScrollText }
];

describe('MorphBar — nav mode (AC-5)', () => {
	it('renders exactly the given primary sections and no "Definitions" (AC-5 re-anchored)', async () => {
		render(MorphBar, { pathname: '/devices', sections: SECTIONS });

		for (const label of ['Devices', 'Actions', 'Compliance', 'Audit']) {
			await expect.element(page.getByRole('link', { name: label })).toBeVisible();
		}
		expect(page.getByRole('link', { name: 'Definitions' }).elements()).toHaveLength(0);
	});

	it('marks the current section as the active page', async () => {
		render(MorphBar, { pathname: '/compliance-policies', sections: SECTIONS });
		const active = page.getByRole('link', { name: 'Compliance' });
		await expect.element(active).toHaveAttribute('aria-current', 'page');
		await expect.element(active).toHaveAttribute('href', '/compliance-policies');
	});
});

describe('MorphBar — search mode (AC-3)', () => {
	const surface = () =>
		createRawSnippet(() => ({ render: () => '<div data-testid="palette-probe">palette</div>' }));

	it('morphs to search when the palette opens and back to nav when it closes', async () => {
		render(MorphBar, { pathname: '/devices', sections: SECTIONS, searchSurface: surface() });
		await expect.element(page.getByTestId('morph-bar')).toHaveAttribute('data-mode', 'nav');

		// ⌘K is wired in the layout; the bar reacts to the store flag.
		shell.paletteOpen = true;
		await expect.element(page.getByTestId('morph-bar')).toHaveAttribute('data-mode', 'search');
		await expect.element(page.getByTestId('palette-probe')).toBeVisible();
		// The nav links are gone once the morph settles (the outgoing branch is
		// held for its out-transition, so this waits rather than sampling).
		await vi.waitFor(() =>
			expect(page.getByRole('link', { name: 'Devices' }).elements()).toHaveLength(0)
		);

		// …and Esc, which the palette owns, restores nav through the same flag.
		shell.paletteOpen = false;
		await expect.element(page.getByTestId('morph-bar')).toHaveAttribute('data-mode', 'nav');
		await expect.element(page.getByRole('link', { name: 'Devices' })).toBeVisible();
	});

	it('opens search from the pill’s own ⌘K affordance', async () => {
		render(MorphBar, { pathname: '/devices', sections: SECTIONS, searchSurface: surface() });
		await page.getByRole('button', { name: m.shell_open_search() }).click();
		expect(shell.paletteOpen).toBe(true);
		await expect.element(page.getByTestId('palette-probe')).toBeVisible();
	});
});
