import { describe, it, expect, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import Panel from './panel.svelte';
import { shell, resetShell, openPanel } from '$lib/shell/shell.svelte';

beforeEach(() => resetShell());

function mountPanel() {
	const id = openPanel('window', 'w1', 'nginx-prod');
	const panel = shell.panels.find((p) => p.id === id)!;
	render(Panel, { panel });
	return id;
}

describe('Panel header controls (AC-8 regression)', () => {

	it('Minimise sends the window to the stage', async () => {
		const id = mountPanel();
		await expect.element(page.getByText('nginx-prod')).toBeVisible();

		await page.getByRole('button', { name: 'Minimise' }).click();
		expect(shell.panels.find((p) => p.id === id)?.minimized).toBe(true);
	});

	it('Close removes the window entirely', async () => {
		const id = mountPanel();
		await page.getByRole('button', { name: 'Close' }).click();
		expect(shell.panels.find((p) => p.id === id)).toBeUndefined();
	});

});
