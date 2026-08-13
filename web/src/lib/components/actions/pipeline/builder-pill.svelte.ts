// Keeps the shell's context pill in lockstep with a builder's draft.
//
// B1's rule is that the card carries no button bar: the ONLY commit affordance
// is the pill. So the builder never renders Save/Cancel itself — it publishes a
// ContextState here and the pill owns ⌘S, Esc, Stash and the validation caption.
//
// `snapshot()` returns null when the pill must NOT hold this builder: the draft
// is clean, or it is parked on the stage. Returning null while parked is what
// makes Stash stick — otherwise this effect would re-enter the context the
// moment the store dropped it.

import {
	shell,
	enterContext,
	updateContext,
	exitContext,
	leaveContext,
	claimDraft
} from '$lib/shell/shell.svelte';
import type { ContextState } from '$lib/shell/shell.svelte';

export type BuilderContext = Omit<ContextState, 'id'>;

/** Binds the pill AND takes back whatever this builder parked on the stage.
 *
 *  The claim runs at component construction, before the first snapshot: a
 *  cross-route restore navigates here and leaves the card standing, so mounting
 *  is exactly when the owner must reclaim it. The returned payload is the
 *  buffer the context snapshotted through `stashPayload` — builders that
 *  rehydrate from their own `useDraft` autosave ignore it and just let the claim
 *  clear the card. */
export function bindBuilderContext(id: string, snapshot: () => BuilderContext | null): unknown {
	const claimed = claimDraft(id);

	$effect(() => {
		const next = snapshot();
		const held = shell.pill.context?.id === id;
		if (!next) {
			if (held) exitContext();
			return;
		}
		// enterContext replaces the whole state; updateContext patches the object
		// the stage draft already closed over, so a restored draft keeps working.
		if (held) updateContext(next);
		else enterContext({ id, ...next });
	});

	// Leaving the page must not strand a context pointing at an unmounted
	// builder. A STASHED draft is untouched — the store already dropped the
	// context, so the id no longer matches and the stage card survives.
	$effect(() => () => {
		// auto-stash-on-navigate: park a dirty builder instead of discarding it.
		leaveContext(id);
	});

	return claimed;
}
