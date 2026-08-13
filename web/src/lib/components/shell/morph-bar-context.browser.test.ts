// Context mode + the detached subtext caption, rendered for real.
// Round 2 supersedes round 1 here: the caption is its OWN surface below the
// pill — a real gap, its own border and shadow — never tucked behind it.
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page, userEvent } from 'vitest/browser';
import { Monitor, Send } from '@lucide/svelte';
import MorphBar from './morph-bar.svelte';
import {
	shell,
	resetShell,
	enterContext,
	enterSelection,
	updateContext,
	setShellPath,
	type ContextState
} from '$lib/shell/shell.svelte';
import { goto } from '$lib/navigation';

vi.mock('$lib/navigation', () => ({ goto: vi.fn() }));

beforeEach(() => {
	resetShell();
	vi.mocked(goto).mockClear();
});

const SECTIONS = [
	{ href: '/devices', label: () => 'Devices', icon: Monitor },
	{ href: '/actions', label: () => 'Actions', icon: Send }
];

function mount() {
	render(MorphBar, { pathname: '/devices', sections: SECTIONS });
}

function ctx(over: Partial<ContextState> = {}): ContextState {
	return {
		id: 'assign-1',
		// Stash is only offered by a context that says where it lives — a parked
		// draft has to be able to navigate back to its surface.
		route: '/assign',
		title: 'Assign · 12 devices',
		dirty: true,
		valid: true,
		commitLabel: 'Assign to 12 →',
		onCommit: () => {},
		...over
	};
}

describe('MorphBar — context mode', () => {
	it('morphs to context, names the target, and lights the dirty dot', async () => {
		mount();
		enterContext(ctx({ dirty: true }));

		await expect.element(page.getByTestId('morph-bar')).toHaveAttribute('data-mode', 'context');
		await expect.element(page.getByText('Assign · 12 devices')).toBeVisible();
		await expect.element(page.getByTestId('pill-dirty')).toBeVisible();
		await expect.element(page.getByLabelText('Unsaved changes')).toBeVisible();
	});

	it('the home glyph parks unsaved work — and ONLY unsaved work', async () => {
		mount();
		enterContext(ctx({ dirty: true }));

		await page.getByTestId('pill-home').click();
		await vi.waitFor(() => expect(shell.drafts).toHaveLength(1));
		expect(vi.mocked(goto)).toHaveBeenCalledWith('/devices');

		// A clean context is a resting state, not work. The pill is held for the
		// WHOLE visit now (so the entity's actions have a home), so parking on the
		// way out would litter the stage with cards for pages the operator only
		// looked at.
		resetShell();
		vi.mocked(goto).mockClear();
		enterContext(ctx({ dirty: false }));

		await page.getByTestId('pill-home').click();
		expect(vi.mocked(goto)).toHaveBeenCalledWith('/devices');
		expect(shell.drafts, 'nothing to park').toHaveLength(0);
	});

	it('shows no dirty dot on a clean context', async () => {
		mount();
		enterContext(ctx({ dirty: false }));

		await expect.element(page.getByTestId('pill-commit')).toBeVisible();
		expect(page.getByTestId('pill-dirty').elements()).toHaveLength(0);
	});

	it('DISABLES the commit while the context is invalid, and enables it when validation passes', async () => {
		const onCommit = vi.fn();
		mount();
		enterContext(ctx({ valid: false, onCommit }));

		const commit = page.getByTestId('pill-commit');
		await expect.element(commit).toBeDisabled();
		expect(getComputedStyle(commit.element()).opacity).toBe('0.4');

		updateContext({ valid: true });
		await expect.element(commit).toBeEnabled();
		expect(getComputedStyle(commit.element()).opacity).toBe('1');

		await commit.click();
		expect(onCommit).toHaveBeenCalledTimes(1);
		await expect.element(page.getByTestId('morph-bar')).toHaveAttribute('data-mode', 'nav');
	});

	it('Esc on a dirty context confirms before discarding; discarding cancels for real', async () => {
		const onCancel = vi.fn();
		mount();
		enterContext(ctx({ dirty: true, onCancel }));

		await userEvent.keyboard('{Escape}');
		await expect.element(page.getByTestId('pill-cancel-confirm')).toBeVisible();
		expect(onCancel).not.toHaveBeenCalled();
		await expect.element(page.getByTestId('morph-bar')).toHaveAttribute('data-mode', 'context');

		await page.getByTestId('pill-discard').click();
		expect(onCancel).toHaveBeenCalledTimes(1);
		await expect.element(page.getByTestId('morph-bar')).toHaveAttribute('data-mode', 'nav');
	});

	it('Stash parks the context on the stage and frees the pill back to nav', async () => {
		const onStash = vi.fn();
		mount();
		enterContext(ctx({ onStash }));

		await page.getByTestId('pill-stash').click();
		expect(onStash).toHaveBeenCalledTimes(1);
		await expect.element(page.getByTestId('morph-bar')).toHaveAttribute('data-mode', 'nav');
	});

	// The reported bug: stashing left the operator stranded on the now-empty
	// create/edit surface. Stash must return them to where they opened it from.
	it('Stash returns the operator to the page they opened the editor from', async () => {
		setShellPath('/roles'); // the list they were browsing…
		setShellPath('/roles/new'); // …then opened the create editor
		mount();
		enterContext(ctx({ id: 'role:new', route: '/roles/new' }));

		await page.getByTestId('pill-stash').click();
		expect(goto).toHaveBeenCalledWith('/roles');
	});

	it('a deep-linked editor with no prior path leaves the operator put on Stash (no navigation)', async () => {
		setShellPath('/roles/new'); // landed straight here — no origin to return to
		mount();
		enterContext(ctx({ id: 'role:new', route: '/roles/new' }));

		await page.getByTestId('pill-stash').click();
		expect(goto).not.toHaveBeenCalled();
	});

	// A destructive pill action drawn like the neutral ones beside it is a trap:
	// Schedule, Assign and Delete would be three identical grey buttons.
	it('marks a destructive action apart from the neutral ones beside it', async () => {
		mount();
		enterContext(
			ctx({
				extraActions: [
					{ id: 'schedule', label: 'Schedule', onRun: () => {} },
					{ id: 'delete', label: 'Delete', tone: 'danger', onRun: () => {} }
				]
			})
		);

		const neutral = page.getByTestId('pill-action').and(page.getByText('Schedule'));
		const danger = page.getByTestId('pill-action').and(page.getByText('Delete'));
		await expect.element(danger).toHaveAttribute('data-tone', 'danger');
		await expect.element(neutral).toHaveAttribute('data-tone', 'neutral');
		// not colour-alone: the destructive one is the only one carrying an icon
		expect(danger.element().querySelector('svg')).not.toBeNull();
		expect(neutral.element().querySelector('svg')).toBeNull();
		expect(getComputedStyle(danger.element()).color).not.toBe(
			getComputedStyle(neutral.element()).color
		);
	});

	it('offers NO Stash to a context with no home route — an exit that could never come back is not offered', async () => {
		mount();
		enterContext(ctx({ route: undefined }));

		await expect.element(page.getByTestId('pill-commit')).toBeVisible();
		expect(page.getByTestId('pill-stash').elements()).toHaveLength(0);
	});
});

describe('MorphBar — detached subtext caption', () => {
	it('renders only when there is something to say', async () => {
		mount();
		enterContext(ctx({ subtext: undefined }));
		await expect.element(page.getByTestId('pill-commit')).toBeVisible();
		expect(page.getByTestId('pill-subtext').elements()).toHaveLength(0);

		updateContext({ subtext: '→ Patch & reboot v7 · 9 apply now' });
		await expect.element(page.getByTestId('pill-subtext')).toBeVisible();
	});

	it('sits DETACHED below the pill: a real gap, its own border and its own shadow', async () => {
		mount();
		enterContext(ctx({ subtext: '→ Patch & reboot v7 · 9 apply now · 2 update in place · 1 queued' }));

		const note = page.getByTestId('pill-subtext');
		await expect.element(note).toBeVisible();

		// geometry settles after the pill's width/height morph
		await vi.waitFor(() => {
			const pillBox = page.getByTestId('pill').element().getBoundingClientRect();
			const noteBox = note.element().getBoundingClientRect();
			expect(noteBox.top).toBeGreaterThanOrEqual(pillBox.bottom + 8);
			// capped to the pill's width — the caption never widens the column
			expect(noteBox.width).toBeLessThanOrEqual(pillBox.width + 1);
		});

		const style = getComputedStyle(note.element());
		expect(style.borderTopStyle).not.toBe('none');
		expect(parseFloat(style.borderTopWidth)).toBeGreaterThan(0);
		expect(style.boxShadow).not.toBe('none');
		// two-line clamp survives from round 1
		expect(style.getPropertyValue('-webkit-line-clamp')).toBe('2');
	});

	// A three-word validation message stretched across the whole pill read as a
	// banner, not a caption. It hugs its text now — and is still capped above.
	it('hugs its own text instead of always spanning the pill', async () => {
		mount();
		enterContext(ctx({ subtext: 'Name is required', subtextTone: 'warn' }));

		const note = page.getByTestId('pill-subtext');
		await expect.element(note).toBeVisible();

		await vi.waitFor(() => {
			const pillBox = page.getByTestId('pill').element().getBoundingClientRect();
			const noteBox = note.element().getBoundingClientRect();
			expect(noteBox.width).toBeLessThan(pillBox.width * 0.9);
			// …and still centred under the pill
			const pillCentre = pillBox.left + pillBox.width / 2;
			const noteCentre = noteBox.left + noteBox.width / 2;
			expect(Math.abs(pillCentre - noteCentre)).toBeLessThanOrEqual(2);
		});
	});

	// THE OVERLAP BUG: the chrome is fixed, so the scrolling content can only
	// reserve room for it if the chrome says how tall it is. With a hard-coded
	// pill height the caption landed on top of the first card below.
	it('publishes its full height so the page can reserve room, and it grows with the caption', async () => {
		mount();
		enterContext(ctx({ subtext: undefined }));

		const read = () =>
			parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--pill-block'));

		let bare = 0;
		await vi.waitFor(() => {
			bare = read();
			expect(bare).toBeGreaterThan(0);
		});

		updateContext({ subtext: '⚠ 1 error blocks Save' });
		await expect.element(page.getByTestId('pill-subtext')).toBeVisible();

		await vi.waitFor(() => {
			const withCaption = read();
			// the reservation must cover the caption, not just the pill
			expect(withCaption).toBeGreaterThan(bare);
			const columnBottom = page
				.getByTestId('pill-subtext')
				.element()
				.getBoundingClientRect().bottom;
			expect(withCaption).toBeGreaterThanOrEqual(columnBottom - 12 - 1);
		});
	});

	it('carries a warn tone variant that is visibly different from the neutral one', async () => {
		mount();
		enterContext(ctx({ subtext: '⚠ 1 error blocks Save', subtextTone: 'warn' }));

		const note = page.getByTestId('pill-subtext');
		await expect.element(note).toHaveAttribute('data-tone', 'warn');
		const warned = getComputedStyle(note.element());
		const warnInk = warned.color;
		const warnPlate = warned.backgroundColor;

		updateContext({ subtextTone: 'neutral' });
		await expect.element(note).toHaveAttribute('data-tone', 'neutral');
		const neutral = getComputedStyle(note.element());
		expect(neutral.color).not.toBe(warnInk);
		expect(neutral.backgroundColor).not.toBe(warnPlate);
	});

	it('a selection caption uses the same one home', async () => {
		mount();
		enterSelection({ count: 12, subtext: 'across 3 groups · 1 offline will queue', actions: [] });

		await expect.element(page.getByTestId('morph-bar')).toHaveAttribute('data-mode', 'selection');
		await expect.element(page.getByTestId('pill-subtext')).toHaveTextContent('across 3 groups');
	});
});
