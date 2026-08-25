import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import StageRail from './stage-rail.svelte';
import {
	shell,
	resetShell,
	openPanel,
	minimizePanel,
	openTerminal,
	toggleTerminal,
	enterContext,
	stashContext,
	setShellPath,
	pillMode
} from '$lib/shell/shell.svelte';

vi.mock('$app/paths', () => ({ base: '', assets: '' }));
vi.mock('$app/navigation', () => ({
	goto: vi.fn(),
	pushState: vi.fn(),
	replaceState: vi.fn()
}));
import { goto } from '$app/navigation';

beforeEach(() => {
	vi.clearAllMocks();
	resetShell();
});

describe('StageRail (AC-6 / AC-7)', () => {
	it('a single minimized window shows a restore card; clicking it floats the window again (AC-6)', async () => {
		const id = openPanel('window', 'w1', 'nginx-prod');
		minimizePanel(id);
		render(StageRail);

		const card = page.getByTestId('stage-card');
		await expect.element(card).toBeVisible();
		await expect.element(page.getByText('nginx-prod')).toBeVisible();

		await card.click();
		expect(shell.panels.find((p) => p.id === id)?.minimized).toBe(false);
	});

	it('≥2 minimized windows of one category collapse into a single stacked card (AC-7)', async () => {
		[openPanel('window', 'w1', 'W1'), openPanel('window', 'w2', 'W2')].forEach(minimizePanel);
		render(StageRail);

		const stack = page.getByTestId('stage-stack');
		await expect.element(stack).toBeVisible();
		await expect.element(stack).toHaveTextContent('2');
		expect(page.getByTestId('stage-card').elements()).toHaveLength(0);

		await stack.click();
		await expect.element(page.getByText('W1')).toBeVisible();
		await expect.element(page.getByText('W2')).toBeVisible();
	});

	function parkHardenSsh() {
		enterContext({
			id: 'harden-ssh',
			route: '/action-sets/harden-ssh',
			title: 'Harden SSH baseline',
			dirty: true,
			valid: false,
			commitLabel: 'Save',
			subtext: 'step 4 · verify script missing',
			onCommit: () => {}
		});
		stashContext();
	}

	it('a stashed draft is a dashed ✎ card that resumes the context it parked, in place', async () => {
		setShellPath('/action-sets/harden-ssh');
		parkHardenSsh();
		render(StageRail);

		const draft = page.getByTestId('stage-draft');
		await expect.element(draft).toBeVisible();
		await expect.element(page.getByText('Harden SSH baseline')).toBeVisible();
		await expect.element(page.getByText('step 4 · verify script missing')).toBeVisible();

		expect(getComputedStyle(draft.element()).borderTopStyle).toBe('dashed');

		await draft.click();
		expect(pillMode()).toBe('context');
		expect(shell.drafts).toHaveLength(0);
		expect(goto).not.toHaveBeenCalled();
	});

	it('the ✕ on a draft card discards it without restoring or navigating', async () => {
		setShellPath('/devices');
		parkHardenSsh();
		render(StageRail);

		await expect.element(page.getByTestId('stage-draft')).toBeVisible();
		await page.getByTestId('stage-draft-discard').click();

		expect(shell.drafts).toHaveLength(0);
		expect(pillMode()).toBe('nav');
		expect(goto).not.toHaveBeenCalled();
	});

	it('a card whose surface is gone NAVIGATES home instead of reviving a dead context', async () => {
		setShellPath('/action-sets/harden-ssh');
		parkHardenSsh();
		setShellPath('/devices');
		render(StageRail);

		await page.getByTestId('stage-draft').click();

		expect(vi.mocked(goto).mock.calls[0]?.[0]).toBe('/action-sets/harden-ssh');

		expect(pillMode()).toBe('nav');

		expect(shell.drafts).toHaveLength(0);
	});

	it('a stashed-but-alive terminal appears on the stage', async () => {
		openTerminal('device-1', 'edge-01');
		toggleTerminal();
		render(StageRail);

		await expect.element(page.getByText('Terminal')).toBeVisible();
		await expect.element(page.getByText('1 open')).toBeVisible();
	});
});
