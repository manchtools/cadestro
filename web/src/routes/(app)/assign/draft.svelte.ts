

import type { AssignSchedule } from './assign-data';

export type TargetMode = 'carried' | 'rule';

export interface AssignDraft {
	mode: TargetMode;
	setId: string | null;
	schedule: AssignSchedule;

	query: string;

	groupName: string;
}

let stashed = $state<AssignDraft | null>(null);

export function stashAssignDraft(draft: AssignDraft) {
	stashed = { ...draft };
}

export function takeAssignDraft(): AssignDraft | null {
	const draft = stashed;
	stashed = null;
	return draft;
}

export function clearAssignDraft() {
	stashed = null;
}
