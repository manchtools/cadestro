// Pill DOM stability under state pushes — the flicker investigation's pins.
//
// Every create/edit surface pushes a freshly built ContextState (new object,
// new closures, new extraActions array) into the store on every reactive tick,
// even when the RENDERED content is identical. These tests pin the invariant
// that such equal-content pushes are INERT at the DOM: no node is added,
// removed or re-attributed anywhere in the pill, and the measured
// `--pill-block` reservation is not republished (a republish would restart the
// layout's padding reservation). A real content change stays a real update —
// the positive control.
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

/** Exactly what a /new page pushes every reactive tick: same rendered content,
 *  all-new object/closure/array identities. */
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
		// Let the nav→context crossfade FINISH: the outgoing nav branch is removed
		// when its out-transition completes (~110ms after enterContext), and that
		// removal is mode-change debris, not push behaviour. Observe only once the
		// morph grid holds exactly the one settled branch.
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
		// --pill-block republication is a style-attribute write on <html>; a
		// republish under equal content would restart the layout's reservation.
		const rootRecords: string[] = [];
		const rootMo = new MutationObserver((list) => {
			for (const r of list) rootRecords.push(describeRecord(r));
		});
		rootMo.observe(document.documentElement, { attributes: true, attributeFilter: ['style'] });

		// Ten pushes, the way a keystroke storm hits the store.
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

		// Positive control: a REAL content change still updates the rendered pill.
		updateContext({ title: 'Munich laptops', subtext: 'One field left' });
		await expect.element(page.getByText('Munich laptops')).toBeVisible();
		await expect.element(page.getByTestId('pill-subtext')).toHaveTextContent('One field left');
	});
});
