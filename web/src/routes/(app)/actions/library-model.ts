

import { actionChoice } from '$lib/components/actions/action-type';
import { DesiredState } from '$contract/cadestro/v1/common_pb';
import type { ManagedAction } from '$contract/cadestro/v1/control_pb';

export const COMPLIANCE_BUCKET = 'compliance';

export const UNFILTERABLE_PREFIX = 'type:';

export function isCompliance(a: ManagedAction): boolean {
	return a.params.case === 'shell' && a.params.value.isCompliance;
}

export function bucketOf(a: ManagedAction): string { return isCompliance(a) ? COMPLIANCE_BUCKET : a.params.case ?? ''; }

export interface LibraryAction {
	id: string;
	name: string;

	absent: boolean;
}

export interface LibraryBubble {

	id: string;

	type: string;

	compliance: boolean;

	filterable: boolean;
	actions: LibraryAction[];

	remove: number;
}

function toLibraryAction(a: ManagedAction): LibraryAction {
	return { id: (a.id?.value ?? ''), name: a.name, absent: a.desiredState === DesiredState.ABSENT };
}

function orderActions(actions: LibraryAction[]): LibraryAction[] {
	return [...actions].sort(
		(a, b) =>
			Number(b.absent) - Number(a.absent) || a.name.localeCompare(b.name) || a.id.localeCompare(b.id)
	);
}

export function buildBubbles(
	actions: ManagedAction[]
): LibraryBubble[] {
	const byBucket = new Map<string, LibraryBubble>();
	for (const a of actions) {
		const id = bucketOf(a);
		let bubble = byBucket.get(id);
		if (!bubble) {
			bubble = {
				id,
				type: actionChoice(a),
				compliance: id === COMPLIANCE_BUCKET,
				filterable: !id.startsWith(UNFILTERABLE_PREFIX),
				actions: [],
				remove: 0
			};
			byBucket.set(id, bubble);
		}
		const row = toLibraryAction(a);
		bubble.actions.push(row);
		if (row.absent) bubble.remove++;
	}

	const bubbles = [...byBucket.values()];
	for (const b of bubbles) b.actions = orderActions(b.actions);
	bubbles.sort(
		(a, b) =>
			Number(b.filterable) - Number(a.filterable) ||
			b.actions.length - a.actions.length ||
			a.id.localeCompare(b.id)
	);
	return bubbles;
}

export interface LibrarySummary {
	total: number;
	install: number;
	remove: number;
}

export function summarize(actions: ManagedAction[]): LibrarySummary {
	const out: LibrarySummary = { total: actions.length, install: 0, remove: 0 };
	for (const a of actions) {
		if (a.desiredState === DesiredState.ABSENT) out.remove++;
		else out.install++;
	}
	return out;
}
