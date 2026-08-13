// Pill state-machine tests: mode precedence, commit gating, dirty-cancel
// gating, the stash → draft → restore round trip, and the ⌘S / Esc semantics
// at the store seam (the chrome only forwards keys to `handlePillKey`).
import { describe, it, expect, beforeEach, vi } from 'vitest';
import {
	shell,
	resetShell,
	pillMode,
	pillSubtext,
	setPillSubtext,
	enterSelection,
	updateSelection,
	clearSelection,
	exitSelection,
	runPillAction,
	enterContext,
	updateContext,
	commitContext,
	requestCancelContext,
	confirmCancelContext,
	dismissCancelConfirm,
	stashContext,
	leaveContext,
	restoreDraft,
	discardDraft,
	claimDraft,
	draftIdFor,
	setShellPath,
	shellPreviousPath,
	handlePillKey,
	notifyNavigated,
	type ContextState,
	type SelectionState
} from './shell.svelte';

beforeEach(() => resetShell());

function selection(over: Partial<SelectionState> = {}): SelectionState {
	return { count: 12, actions: [], ...over };
}

function context(over: Partial<ContextState> = {}): ContextState {
	return {
		id: 'assign-1',
		route: '/assign',
		title: 'Assign · 12 devices',
		dirty: false,
		valid: true,
		commitLabel: 'Assign to 12 →',
		onCommit: () => {},
		...over
	};
}

describe('pill mode precedence', () => {
	it('rests in nav', () => {
		expect(pillMode()).toBe('nav');
	});

	it('a live selection morphs the pill to selection mode', () => {
		enterSelection(selection());
		expect(pillMode()).toBe('selection');
	});

	it('context outranks selection — a committable surface keeps its commit on screen', () => {
		enterSelection(selection());
		enterContext(context());
		expect(pillMode()).toBe('context');
	});

	it('an explicit ⌘K outranks everything, and releasing it falls back to context', () => {
		enterSelection(selection());
		enterContext(context());
		shell.paletteOpen = true;
		expect(pillMode()).toBe('search');

		shell.paletteOpen = false;
		expect(pillMode()).toBe('context');
	});
});

describe('selection mode', () => {
	it('runs an action by id and leaves the selection standing', () => {
		const run = vi.fn();
		enterSelection(selection({ actions: [{ id: 'assign', label: 'Assign', primary: true, onRun: run }] }));

		runPillAction('assign');
		expect(run).toHaveBeenCalledTimes(1);
		expect(pillMode()).toBe('selection');
	});

	it('an unknown action id is a no-op, not a throw', () => {
		enterSelection(selection({ actions: [{ id: 'assign', label: 'Assign', onRun: vi.fn() }] }));
		expect(() => runPillAction('nope')).not.toThrow();
	});

	it('the ✕ notifies the owner and returns the pill to nav', () => {
		const onClear = vi.fn();
		enterSelection(selection({ onClear }));

		clearSelection();
		expect(onClear).toHaveBeenCalledTimes(1);
		expect(pillMode()).toBe('nav');
	});

	it('owner-driven teardown drops the mode WITHOUT calling onClear', () => {
		const onClear = vi.fn();
		enterSelection(selection({ onClear }));

		exitSelection();
		expect(onClear).not.toHaveBeenCalled();
		expect(pillMode()).toBe('nav');
	});

	it('updates the live count in place', () => {
		enterSelection(selection({ count: 12 }));
		updateSelection({ count: 47 });
		expect(shell.pill.selection?.count).toBe(47);
	});
});

describe('context commit gating', () => {
	it('commits a valid context, runs onCommit, and frees the pill', () => {
		const onCommit = vi.fn();
		// A committable context has changes AND passes validation; the clean
		// resting state an edit page now holds is deliberately not committable.
		enterContext(context({ valid: true, dirty: true, onCommit }));

		expect(commitContext()).toBe(true);
		expect(onCommit).toHaveBeenCalledTimes(1);
		expect(pillMode()).toBe('nav');
	});

	it('an INVALID context can never commit — the guard is in the store, not only the disabled attribute', () => {
		const onCommit = vi.fn();
		enterContext(context({ valid: false, onCommit }));

		expect(commitContext()).toBe(false);
		expect(onCommit).not.toHaveBeenCalled();
		expect(pillMode()).toBe('context');
	});

	it('becomes committable when validation flips', () => {
		const onCommit = vi.fn();
		enterContext(context({ valid: false, dirty: true, onCommit }));
		expect(commitContext()).toBe(false);

		updateContext({ valid: true });
		expect(commitContext()).toBe(true);
		expect(onCommit).toHaveBeenCalledTimes(1);
	});
});

describe('dirty-cancel gating', () => {
	it('a CLEAN context cancels immediately — no confirmation', () => {
		const onCancel = vi.fn();
		enterContext(context({ dirty: false, onCancel }));

		requestCancelContext();
		expect(shell.pill.cancelPending).toBe(false);
		expect(onCancel).toHaveBeenCalledTimes(1);
		expect(pillMode()).toBe('nav');
	});

	it('a DIRTY context asks first and discards nothing until confirmed', () => {
		const onCancel = vi.fn();
		enterContext(context({ dirty: true, onCancel }));

		requestCancelContext();
		expect(shell.pill.cancelPending).toBe(true);
		expect(onCancel).not.toHaveBeenCalled();
		expect(pillMode()).toBe('context');

		confirmCancelContext();
		expect(onCancel).toHaveBeenCalledTimes(1);
		expect(pillMode()).toBe('nav');
	});

	it('dismissing the confirmation keeps the context and its edits', () => {
		const onCancel = vi.fn();
		enterContext(context({ dirty: true, onCancel }));
		requestCancelContext();

		dismissCancelConfirm();
		expect(shell.pill.cancelPending).toBe(false);
		expect(onCancel).not.toHaveBeenCalled();
		expect(pillMode()).toBe('context');
	});
});

describe('keyboard seam', () => {
	it('⌘S commits a valid context and reports the key consumed', () => {
		const onCommit = vi.fn();
		enterContext(context({ valid: true, dirty: true, onCommit }));

		expect(handlePillKey({ key: 's', metaKey: true })).toBe(true);
		expect(onCommit).toHaveBeenCalledTimes(1);
	});

	it('Ctrl+S is the same commit path', () => {
		const onCommit = vi.fn();
		enterContext(context({ valid: true, dirty: true, onCommit }));

		expect(handlePillKey({ key: 'S', ctrlKey: true })).toBe(true);
		expect(onCommit).toHaveBeenCalledTimes(1);
	});

	it('⌘S on an INVALID context is still consumed — the browser Save dialog must not appear over a draft that cannot ship', () => {
		const onCommit = vi.fn();
		enterContext(context({ valid: false, onCommit }));

		expect(handlePillKey({ key: 's', metaKey: true })).toBe(true);
		expect(onCommit).not.toHaveBeenCalled();
		expect(pillMode()).toBe('context');
	});

	it('a bare "s" is never a commit', () => {
		const onCommit = vi.fn();
		enterContext(context({ valid: true, onCommit }));

		expect(handlePillKey({ key: 's' })).toBe(false);
		expect(onCommit).not.toHaveBeenCalled();
	});

	it('Esc on a dirty context raises the confirmation; a second Esc dismisses it rather than discarding', () => {
		const onCancel = vi.fn();
		enterContext(context({ dirty: true, onCancel }));

		expect(handlePillKey({ key: 'Escape' })).toBe(true);
		expect(shell.pill.cancelPending).toBe(true);

		expect(handlePillKey({ key: 'Escape' })).toBe(true);
		expect(shell.pill.cancelPending).toBe(false);
		expect(onCancel).not.toHaveBeenCalled();
		expect(pillMode()).toBe('context');
	});

	it('Esc clears a selection', () => {
		const onClear = vi.fn();
		enterSelection(selection({ onClear }));

		expect(handlePillKey({ key: 'Escape' })).toBe(true);
		expect(onClear).toHaveBeenCalledTimes(1);
		expect(pillMode()).toBe('nav');
	});

	it('consumes nothing in nav mode, and nothing in search mode (the palette owns its own keys)', () => {
		expect(handlePillKey({ key: 'Escape' })).toBe(false);

		enterContext(context());
		shell.paletteOpen = true;
		expect(handlePillKey({ key: 'Escape' })).toBe(false);
		expect(handlePillKey({ key: 's', metaKey: true })).toBe(false);
	});
});

describe('stash → draft → restore', () => {
	it('parks the context on the stage with its home route and frees the pill', () => {
		const onStash = vi.fn();
		const ctx = context({ dirty: true, valid: false, subtext: '1 error blocks Save', onStash });
		enterContext(ctx);

		const draftId = stashContext();
		expect(draftId).toBe('draft:assign-1');
		expect(onStash).toHaveBeenCalledTimes(1);
		expect(pillMode()).toBe('nav');
		expect(shell.drafts).toEqual([
			expect.objectContaining({
				id: 'draft:assign-1',
				kind: 'draft',
				title: 'Assign · 12 devices',
				subtitle: '1 error blocks Save',
				route: '/assign'
			})
		]);
	});

	it('restores IN PLACE while the owner is still the mounted surface', () => {
		setShellPath('/assign');
		const onRestore = vi.fn();
		const ctx = context({ dirty: true, onRestore });
		enterContext(ctx);
		stashContext();

		expect(restoreDraft('draft:assign-1')).toBeNull(); // nothing for the chrome to navigate
		expect(onRestore).toHaveBeenCalledTimes(1);
		expect(pillMode()).toBe('context');
		// the exact object round-trips, so the mounted surface's buffers survive
		expect(shell.pill.context).toBe(ctx);
		expect(shell.pill.context?.dirty).toBe(true);
		expect(shell.drafts).toEqual([]);
	});

	// THE REPORTED BUG. Stash on /actions/new, navigate away, restore: the old
	// store re-entered the parked ContextState, whose onCommit/onCancel closed
	// over an unmounted surface — the pill came back unable to see or do
	// anything. Restoring from elsewhere must hand the route to the chrome and
	// NOT re-enter the dead context; the surface re-enters with live closures
	// once it mounts and claims the staged payload.
	it('cross-route restore hands the route to the chrome and does NOT re-enter the dead context', () => {
		setShellPath('/actions/new');
		const onRestore = vi.fn();
		const ctx = context({ id: 'action:new', route: '/actions/new', dirty: true, onRestore });
		enterContext(ctx);
		stashContext();

		setShellPath('/devices'); // the operator navigated; /actions/new unmounted

		expect(restoreDraft('draft:action:new')).toBe('/actions/new');
		expect(pillMode()).toBe('nav');
		expect(onRestore).not.toHaveBeenCalled();
		// The card leaves the rail on the click (the payload is staged for the
		// owner instead) — a card that lingered until some later claim was the
		// "restoring does not pop it reliably" bug.
		expect(shell.drafts).toEqual([]);
	});

	it('the owning surface claims its draft on mount — payload once, then the card is gone', () => {
		setShellPath('/roles/r1');
		enterContext(
			context({
				id: 'role:r1',
				route: '/roles/r1',
				dirty: true,
				stashPayload: () => ({ name: 'Auditor', permissions: ['audit.read'] })
			})
		);
		stashContext();

		setShellPath('/devices');
		expect(restoreDraft(draftIdFor('role:r1'))).toBe('/roles/r1');

		// …the chrome navigates, the surface mounts and takes its buffer back.
		setShellPath('/roles/r1');
		expect(claimDraft('role:r1')).toEqual({ name: 'Auditor', permissions: ['audit.read'] });
		expect(shell.drafts).toEqual([]);
		// read-and-remove: a second claim can never resurrect an orphaned copy
		expect(claimDraft('role:r1')).toBeUndefined();
	});

	it('claiming with nothing parked is a no-op, so a surface can claim unconditionally on mount', () => {
		expect(claimDraft('never-stashed')).toBeUndefined();
		expect(shell.drafts).toEqual([]);
	});

	it('REFUSES to stash a context that declares no route, loudly — a draft with no home could never restore', () => {
		const error = vi.spyOn(console, 'error').mockImplementation(() => {});
		const ctx = context({ route: undefined, dirty: true });
		enterContext(ctx);

		expect(stashContext()).toBeNull();
		expect(shell.drafts).toEqual([]);
		expect(pillMode()).toBe('context'); // the context is KEPT, not silently dropped
		expect(error).toHaveBeenCalledTimes(1);
		error.mockRestore();
	});

	it('stashing twice from the same context id keeps one card, not a pile', () => {
		setShellPath('/assign');
		const ctx = context();
		enterContext(ctx);
		stashContext();
		restoreDraft('draft:assign-1');
		stashContext();

		expect(shell.drafts).toHaveLength(1);
	});

	it('stashing with no context is a no-op', () => {
		expect(stashContext()).toBeNull();
		expect(shell.drafts).toEqual([]);
	});

	it('restoring an unknown draft id is a no-op, not a throw', () => {
		expect(() => restoreDraft('draft:gone')).not.toThrow();
		expect(restoreDraft('draft:gone')).toBeNull();
	});
});

// leaveContext is the auto-stash-on-navigate rule. THE DATA-LOSS BUG: a surface
// leaves (its editor unmounts on navigation — e.g. because the operator restored
// a DIFFERENT parked draft) while it still holds a dirty context. The old
// single-slot teardown called exitContext(), discarding the open work with no
// stage card. leaveContext must park it instead, exactly like the Stash button.
describe('leaveContext — auto-stash-on-navigate', () => {
	it('parks a dirty context on leave so restoring elsewhere can never destroy open work', () => {
		setShellPath('/roles/r1');
		const onStash = vi.fn();
		enterContext(
			context({
				id: 'role:r1',
				route: '/roles/r1',
				dirty: true,
				title: 'Auditor',
				stashPayload: () => ({ name: 'Auditor' }),
				onStash
			})
		);

		leaveContext('role:r1'); // the surface unmounted (operator restored another draft)

		expect(pillMode()).toBe('nav'); // pill freed
		expect(onStash).toHaveBeenCalledTimes(1); // parked exactly like the Stash button
		expect(shell.drafts).toEqual([
			expect.objectContaining({ id: 'draft:role:r1', route: '/roles/r1', title: 'Auditor' })
		]);
		// the buffer rode onto the card, so the unsaved edits survive the leave
		expect(claimDraft('role:r1')).toEqual({ name: 'Auditor' });
	});

	it('drops a CLEAN context on leave without parking a card — untouched forms do not pile up', () => {
		enterContext(context({ id: 'clean', route: '/x', dirty: false }));
		leaveContext('clean');
		expect(pillMode()).toBe('nav');
		expect(shell.drafts).toEqual([]);
	});

	it('a routeless dirty context cannot be parked, so leave just releases it (no throw, no card)', () => {
		enterContext(context({ id: 'homeless', route: undefined, dirty: true }));
		expect(() => leaveContext('homeless')).not.toThrow();
		expect(pillMode()).toBe('nav');
		expect(shell.drafts).toEqual([]);
	});

	// THE ONE-SLOT TRAP, seen in the running app as a pill that flashed the
	// entity's actions and then fell back to nav. Two owners on one page both
	// bound `action:<id>`: the page entered its context, the embedded editor
	// overwrote it, and the editor's own "nothing to edit" teardown then dropped
	// the slot — taking the page's actions with it. One id, one owner.
	it('a second enterContext on the same id REPLACES the first — the slot holds exactly one owner', () => {
		const pageActions = [{ id: 'delete', label: 'Delete', onRun: () => {} }];
		enterContext(context({ id: 'action:a1', extraActions: pageActions }));
		expect(shell.pill.context?.extraActions).toHaveLength(1);

		// the embedded editor claims the same id and wins…
		enterContext(context({ id: 'action:a1', title: 'editor', extraActions: [] }));
		expect(shell.pill.context?.title).toBe('editor');
		expect(shell.pill.context?.extraActions).toHaveLength(0);

		// …and its teardown then frees the slot entirely, so the page's actions are
		// gone too. The fix is that the editor carries them, not that both enter.
		leaveContext('action:a1');
		expect(pillMode()).toBe('nav');
	});

	it('leave for a DIFFERENT id leaves the live context untouched — a sibling teardown cannot evict it', () => {
		enterContext(context({ id: 'a', route: '/a', dirty: true }));
		leaveContext('b');
		expect(pillMode()).toBe('context');
		expect(shell.pill.context?.id).toBe('a');
		expect(shell.drafts).toEqual([]);
	});

	// REPORTED: cancelling parked the work anyway. Cancel is an explicit discard;
	// the navigation it triggers must not be mistaken for "leaving with unsaved
	// work". Same for commit — the buffer was saved, not abandoned.
	it('CANCEL discards: the leave that follows must not park the work', () => {
		const ctx = context({ id: 'role:new', route: '/roles/new', dirty: true });
		enterContext(ctx);

		confirmCancelContext();
		// the builder's effect re-enters the still-dirty context, then unmounts
		enterContext(ctx);
		leaveContext('role:new');

		expect(shell.drafts).toEqual([]);
		expect(pillMode()).toBe('nav');
	});

	it('COMMIT resolves: the leave that follows must not park the saved work', () => {
		const ctx = context({ id: 'role:new', route: '/roles/new', dirty: true, valid: true });
		enterContext(ctx);

		expect(commitContext()).toBe(true);
		enterContext(ctx);
		leaveContext('role:new');

		expect(shell.drafts).toEqual([]);
	});

	it('the suppression is one-shot: a LATER genuine leave of the same id still parks', () => {
		const ctx = context({ id: 'role:new', route: '/roles/new', dirty: true });
		enterContext(ctx);
		confirmCancelContext();
		enterContext(ctx);
		leaveContext('role:new'); // consumes the suppression
		expect(shell.drafts).toEqual([]);

		enterContext(ctx); // a fresh edit later
		leaveContext('role:new'); // a real navigate-away
		expect(shell.drafts).toHaveLength(1);
	});
});

// REPORTED: clicking a stashed card sometimes left it on the rail — it only
// left "once". The card must pop on the click itself, with the payload staged
// for whenever the owning surface mounts (a reused [id] component or a
// load-gated claim must not strand it).
describe('draft cards pop on click, and can be thrown away', () => {
	it('a cross-route restore removes the card IMMEDIATELY and still hands the payload back', () => {
		setShellPath('/roles/r1');
		enterContext(
			context({ id: 'role:r1', route: '/roles/r1', dirty: true, stashPayload: () => ({ name: 'Auditor' }) })
		);
		stashContext();

		setShellPath('/devices');
		expect(restoreDraft(draftIdFor('role:r1'))).toBe('/roles/r1');
		expect(shell.drafts).toEqual([]); // gone from the rail on the click, not later

		// …and the buffer still reaches the surface whenever it mounts.
		setShellPath('/roles/r1');
		expect(claimDraft('role:r1')).toEqual({ name: 'Auditor' });
		expect(claimDraft('role:r1')).toBeUndefined(); // once only
	});

	it('discardDraft throws parked work away without restoring it, payload and all', () => {
		setShellPath('/roles/r1');
		enterContext(
			context({ id: 'role:r1', route: '/roles/r1', dirty: true, stashPayload: () => ({ name: 'Auditor' }) })
		);
		stashContext();

		discardDraft(draftIdFor('role:r1'));

		expect(shell.drafts).toEqual([]);
		// the buffer must not survive to resurrect on a later mount
		expect(claimDraft('role:r1')).toBeUndefined();
	});

	it('discarding an unknown card is a no-op, not a throw', () => {
		expect(() => discardDraft('draft:gone')).not.toThrow();
	});
});

describe('subtext strip', () => {
	it('renders nothing when there is nothing to say', () => {
		expect(pillSubtext()).toBeNull();
	});

	it('carries a mode-independent caption in nav mode', () => {
		setPillSubtext('408 online · 6 need attention');
		expect(pillSubtext()).toEqual({ text: '408 online · 6 need attention', tone: 'neutral' });

		setPillSubtext(null);
		expect(pillSubtext()).toBeNull();
	});

	it("the active mode's own caption wins, with its tone", () => {
		setPillSubtext('408 online');
		enterContext(context({ subtext: '⚠ 1 error blocks Save', subtextTone: 'warn' }));

		expect(pillSubtext()).toEqual({ text: '⚠ 1 error blocks Save', tone: 'warn' });
	});

	it('a selection caption defaults to the neutral tone and falls back to the strip when absent', () => {
		setPillSubtext('408 online');
		enterSelection(selection({ subtext: 'across 3 groups · 1 offline will queue' }));
		expect(pillSubtext()).toEqual({ text: 'across 3 groups · 1 offline will queue', tone: 'neutral' });

		updateSelection({ subtext: undefined });
		expect(pillSubtext()).toEqual({ text: '408 online', tone: 'neutral' });
	});
});

describe('navigation', () => {
	it('selection, context and drafts survive a route change exactly like panels do', () => {
		enterSelection(selection({ count: 12 }));
		const ctx = context({ id: 'draft-me' });
		enterContext(ctx);
		stashContext();
		enterSelection(selection({ count: 12 }));

		notifyNavigated();
		notifyNavigated();

		expect(pillMode()).toBe('selection');
		expect(shell.pill.selection?.count).toBe(12);
		expect(shell.drafts).toHaveLength(1);
	});

	// The origin Stash returns to: where the operator was BEFORE the editor.
	it('remembers the prior path so Stash can return the operator to where they opened the editor', () => {
		setShellPath('/actions');
		setShellPath('/actions/new');
		expect(shellPreviousPath()).toBe('/actions');
	});

	it('a repeated path does not clobber the origin — re-publishing the same route is idempotent', () => {
		setShellPath('/actions');
		setShellPath('/actions/new');
		setShellPath('/actions/new'); // layout may re-publish the same path
		expect(shellPreviousPath()).toBe('/actions');
	});
});
