
import { describe, it, expect, beforeEach } from 'vitest';
import {
	shell,
	resetShell,
	openPanel,
	minimizePanel,
	restorePanel,
	closePanel,
	stagedByKind,
	openTerminal,
	focusSession,
	closeSession,
	toggleTerminal,
	notifyNavigated
} from './shell.svelte';

beforeEach(() => resetShell());

describe('windows and stage', () => {
	it('reopening a window restores the same panel instead of duplicating it', () => {
		const id = openPanel('device', 'device-1', 'edge-01');
		minimizePanel(id);

		expect(openPanel('device', 'device-1', 'edge-01')).toBe(id);
		expect(shell.panels).toHaveLength(1);
		expect(shell.panels[0].minimized).toBe(false);
	});

	it('minimized windows are grouped by category and can be restored', () => {
		const first = openPanel('device', 'device-1', 'edge-01');
		const second = openPanel('device', 'device-2', 'edge-02');
		const other = openPanel('action', 'action-1', 'Patch');
		[first, second, other].forEach(minimizePanel);

		const staged = stagedByKind();
		expect(staged.find((group) => group.kind === 'device')?.panels).toHaveLength(2);
		expect(staged.find((group) => group.kind === 'action')?.panels).toHaveLength(1);

		restorePanel(first);
		expect(shell.panels.find((panel) => panel.id === first)?.minimized).toBe(false);
	});

	it('closing a window removes it', () => {
		const id = openPanel('device', 'device-1', 'edge-01');
		closePanel(id);
		expect(shell.panels).toHaveLength(0);
	});
});

describe('persistent terminals', () => {
	it('deduplicates by device identity and refreshes the display name', () => {
		const first = openTerminal('device-1', 'old-hostname');
		const second = openTerminal('device-1', 'edge-01');

		expect(second).toBe(first);
		expect(shell.terminal.sessions).toEqual([
			expect.objectContaining({ id: first, deviceId: 'device-1', name: 'edge-01' })
		]);
		expect(shell.terminal.open).toBe(true);
	});

	it('survives navigation and tracks route changes while connected', () => {
		notifyNavigated();
		expect(shell.terminal.navsSinceOpen).toBe(0);

		openTerminal('device-1', 'edge-01');
		notifyNavigated();
		notifyNavigated();
		expect(shell.terminal.navsSinceOpen).toBe(2);
	});

	it('minimizes without destroying the session and restores by focus', () => {
		const id = openTerminal('device-1', 'edge-01');
		toggleTerminal();
		expect(shell.terminal.open).toBe(false);
		expect(shell.terminal.sessions).toHaveLength(1);

		focusSession(id);
		expect(shell.terminal.open).toBe(true);
		expect(shell.terminal.activeId).toBe(id);
	});

	it('closing the last session hides the drawer', () => {
		const id = openTerminal('device-1', 'edge-01');
		closeSession(id);
		expect(shell.terminal.sessions).toHaveLength(0);
		expect(shell.terminal.open).toBe(false);
	});
});
