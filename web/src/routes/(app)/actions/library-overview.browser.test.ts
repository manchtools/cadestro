

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { userEvent } from 'vitest/browser';
import { ActionType } from '$contract/cadestro/v1/actions_pb';
import { getActionTypeLabel } from '$lib/components/actions';
import * as m from '$lib/paraglide/messages';

import LibraryOverview from './library-overview.svelte';
import type { LibraryBubble } from './library-model';
import { COMPLIANCE_BUCKET } from './library-model';

const BUBBLES: LibraryBubble[] = [
	{
		id: 'package',
		type: ActionType.PACKAGE,
		compliance: false,
		filterable: true,
		remove: 1,
		actions: [
			{ id: 'p2', name: 'Drop telnet', absent: true },
			{ id: 'p1', name: 'Install Firefox', absent: false }
		]
	},
	{
		id: COMPLIANCE_BUCKET,
		type: ActionType.SHELL,
		compliance: true,
		filterable: true,
		remove: 0,
		actions: [{ id: 'c1', name: 'Check LUKS', absent: false }]
	}
];

const tiles = () =>
	Array.from(document.querySelectorAll<HTMLElement>('[data-testid="library-tile"]'));
const tile = (actionId: string) =>
	document.querySelector<HTMLElement>(`[data-testid="library-tile"][data-action-id="${actionId}"]`)!;
const peekOf = (bucket: string) =>
	document.querySelector<HTMLElement>(`[data-testid="library-peek"][data-bucket="${bucket}"]`)!;

function peekedNames(): string[] {
	return Array.from(document.querySelectorAll<HTMLElement>('[data-testid="library-peek"]')).map(
		(slot) => slot.querySelector('[data-testid="library-peek-name"]')?.textContent?.trim() ?? ''
	);
}

beforeEach(async () => {
	document.body.innerHTML = '';

	const park = document.createElement('div');
	park.style.cssText = 'position:fixed;right:0;bottom:0;width:24px;height:24px';
	document.body.appendChild(park);
	await userEvent.hover(park);
	park.remove();
});

describe('hovering a tile peeks the action it stands for', () => {
	it('names the hovered action — name, type and desired state — in its own bubble', async () => {
		render(LibraryOverview, { bubbles: BUBBLES, onFocus: vi.fn() });
		await vi.waitFor(() => expect(tiles().length).toBe(3));

		expect(peekedNames()).toEqual(['', '']);

		await userEvent.hover(tile('p2'));

		await vi.waitFor(() => expect(peekedNames()).toEqual(['Drop telnet', '']));
		const peek = peekOf('package');
		expect(peek.textContent).toContain('Drop telnet');
		expect(peek.textContent).toContain(getActionTypeLabel(ActionType.PACKAGE));
		expect(peek.textContent).toContain(m.desired_state_absent());

		await vi.waitFor(() => expect(getComputedStyle(peek).opacity).toBe('1'));
	});

	it('clears the peek when the pointer leaves the tile', async () => {
		render(LibraryOverview, { bubbles: BUBBLES, onFocus: vi.fn() });
		await vi.waitFor(() => expect(tiles().length).toBe(3));

		await userEvent.hover(tile('p1'));
		await vi.waitFor(() => expect(peekedNames()).toEqual(['Install Firefox', '']));

		await userEvent.unhover(tile('p1'));

		await vi.waitFor(() => expect(peekedNames()).toEqual(['', '']));
		await vi.waitFor(() => expect(getComputedStyle(peekOf('package')).opacity).toBe('0'));
	});

	it('swaps rather than clears when the pointer crosses to the next tile', async () => {

		render(LibraryOverview, { bubbles: BUBBLES, onFocus: vi.fn() });
		await vi.waitFor(() => expect(tiles().length).toBe(3));

		await userEvent.hover(tile('p2'));
		await vi.waitFor(() => expect(peekedNames()).toEqual(['Drop telnet', '']));

		await userEvent.hover(tile('c1'));

		await vi.waitFor(() => expect(peekedNames()).toEqual(['', 'Check LUKS']));
		expect(peekOf(COMPLIANCE_BUCKET).textContent).toContain(m.desired_state_present());
	});
});

describe('the peek is not a mouse-only promise', () => {
	it('reveals the same peek on keyboard focus and drops it on blur', async () => {
		render(LibraryOverview, { bubbles: BUBBLES, onFocus: vi.fn() });
		await vi.waitFor(() => expect(tiles().length).toBe(3));

		for (const t of tiles()) expect(t.tabIndex).toBeGreaterThanOrEqual(0);

		tile('c1').focus();

		await vi.waitFor(() => expect(peekedNames()).toEqual(['', 'Check LUKS']));
		const peek = peekOf(COMPLIANCE_BUCKET);
		expect(peek.textContent).toContain('Check LUKS');
		expect(peek.textContent).toContain(m.desired_state_present());

		tile('c1').blur();

		await vi.waitFor(() => expect(peekedNames()).toEqual(['', '']));
	});
});

describe('a tile peeks — it never navigates', () => {
	it('activating a tile shows the peek and leaves the page’s type filter alone', async () => {
		const onFocus = vi.fn();
		render(LibraryOverview, { bubbles: BUBBLES, onFocus });
		await vi.waitFor(() => expect(tiles().length).toBe(3));

		tile('p1').click();

		await vi.waitFor(() => expect(peekedNames()).toEqual(['Install Firefox', '']));
		expect(onFocus).not.toHaveBeenCalled();

		document
			.querySelector<HTMLButtonElement>(
				'[data-bucket="package"] [data-testid="library-bubble-header"]'
			)!
			.click();
		expect(onFocus).toHaveBeenCalledWith('package');
	});
});
