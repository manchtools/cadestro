

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import { Monitor, Send } from '@lucide/svelte';
import MorphBar from './morph-bar.svelte';
import {
	resetShell,
	enterContext,
	updateContext,
	type ContextState
} from '$lib/shell/shell.svelte';

vi.mock('$lib/navigation', () => ({ goto: vi.fn() }));

beforeEach(() => {
	resetShell();
});

const SECTIONS = [
	{ href: '/devices', label: () => 'Devices', icon: Monitor },
	{ href: '/actions', label: () => 'Actions', icon: Send }
];

function freshSnapshot(): ContextState {
	return {
		id: 'group:create',
		route: '/device-groups/new',
		title: 'Berlin laptops',
		dirty: true,
		valid: true,
		commitLabel: 'Create',
		subtext: 'Ready to create',
		subtextTone: 'neutral',
		stashSubtitle: 'Draft device group',
		onCommit: () => {},
		onCancel: () => {},
		onStash: () => {},
		onRestore: () => {},
		stashPayload: () => ({}),
		extraActions: [{ id: 'delete', label: 'Delete', tone: 'danger', onRun: () => {} }]
	};
}

const settle = () =>
	new Promise<void>((r) => requestAnimationFrame(() => requestAnimationFrame(() => r())));

function describeRecord(r: MutationRecord): string {
	const t = r.target as Element;
	const name =
		t.nodeType === 1 ? ((t as Element).getAttribute?.('data-testid') ?? t.nodeName) : t.nodeName;
	return `${r.type}:${name}:${r.attributeName ?? ''}:${r.addedNodes.length}+/${r.removedNodes.length}-`;
}

describe('pill DOM stability — equal-content pushes are inert', () => {
	it('ten identity-churning but equal-content pushes mutate nothing, and a real change still lands', async () => {
		render(MorphBar, { pathname: '/device-groups/new', sections: SECTIONS });
		enterContext(freshSnapshot());
		await expect.element(page.getByTestId('morph-bar')).toHaveAttribute('data-mode', 'context');
		await expect.element(page.getByTestId('pill-subtext')).toBeVisible();

		await vi.waitFor(() => {
			const grid = page.getByTestId('pill').element().firstElementChild;
			expect(grid?.childElementCount).toBe(1);
		});
		await settle();
		await settle();

		const pillRecords: string[] = [];
		const pillMo = new MutationObserver((list) => {
			for (const r of list) pillRecords.push(describeRecord(r));
		});
		pillMo.observe(page.getByTestId('morph-bar').element(), {
			subtree: true,
			childList: true,
			attributes: true,
			characterData: true
		});

		const rootRecords: string[] = [];
		const rootMo = new MutationObserver((list) => {
			for (const r of list) rootRecords.push(describeRecord(r));
		});
		rootMo.observe(document.documentElement, { attributes: true, attributeFilter: ['style'] });

		for (let i = 0; i < 10; i++) {
			const { id: _id, ...patch } = freshSnapshot();
			updateContext(patch);
		}
		await settle();
		await settle();
		pillMo.disconnect();
		rootMo.disconnect();

		expect(pillRecords, 'equal-content pushes must not touch the pill DOM').toEqual([]);
		expect(rootRecords, 'equal-content pushes must not republish --pill-block').toEqual([]);

		updateContext({ title: 'Munich laptops', subtext: 'One field left' });
		await expect.element(page.getByText('Munich laptops')).toBeVisible();
		await expect.element(page.getByTestId('pill-subtext')).toHaveTextContent('One field left');
	});
});
