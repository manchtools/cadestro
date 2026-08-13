// The stashed assign draft.
//
// Stash parks the context on the stage rail and frees the pill; the operator
// may then navigate away, which unmounts /assign. Module state (not component
// state) is therefore where the choice has to live, so restoring the stage card
// can navigate back and find the draft intact. It is deliberately NOT persisted
// through `useDraft`: the SDK's DraftType union has no 'assign' member, and
// widening it is the SDK's lane, not this route's.
//
// The draft covers BOTH targeting modes: which mode was open, the set and
// schedule, and — for a rule target — the compiled query and the group name.
// A restore that dropped the rule would silently hand the operator back a
// different assignment from the one they parked.

import type { AssignSchedule } from './assign-data';

/** Which stage the assign surface is targeting through. */
export type TargetMode = 'carried' | 'rule';

export interface AssignDraft {
	mode: TargetMode;
	setId: string | null;
	schedule: AssignSchedule;
	/** The compiled rule string, empty in carried mode. */
	query: string;
	/** Name for the dynamic group a rule target creates. */
	groupName: string;
}

let stashed = $state<AssignDraft | null>(null);

export function stashAssignDraft(draft: AssignDraft) {
	stashed = { ...draft };
}

/** Read-and-clear: the stage card is removed by the same restore, so a draft
 *  that survived the read would be a second, orphaned copy. */
export function takeAssignDraft(): AssignDraft | null {
	const draft = stashed;
	stashed = null;
	return draft;
}

export function clearAssignDraft() {
	stashed = null;
}
