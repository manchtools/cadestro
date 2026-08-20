// Pure library derivations for the actions surface's overview zoom (concept A4,
// applied to the action library instead of the fleet). Everything here is
// computed from fields the control contract actually returns on ManagedAction —
// `type`, `desired_state` and the shell params' `is_compliance` — so no state is
// invented and no bucket exists that the page's own type filter cannot name.
//
// There is deliberately no health tone: nothing on a ManagedAction says whether
// it succeeded anywhere. A library row's only real binary is its desired state,
// which is exactly the ok/crit chip the list row already draws, so the overview
// and the list can never disagree about an action.
import { ActionType } from '$contract/cadestro/v1/actions_pb';
import { DesiredState } from '$contract/cadestro/v1/common_pb';
import type { ManagedAction } from '$contract/cadestro/v1/control_pb';

/** The virtual bucket for SHELL actions carrying `is_compliance`. Its id is the
 *  page's own `COMPLIANCE_FILTER_ID`, so clicking the bubble drives the real
 *  `?types=compliance` filter rather than a lookalike. */
export const COMPLIANCE_BUCKET = 'compliance';

/** Bucket id for an ActionType the page's type filter has no slug for (the
 *  filter menu omits SCRIPT_RUN / WIFI). Such a bucket is
 *  counted honestly and rendered, but it is NOT clickable — narrowing the list
 *  to it is something this page's filter genuinely cannot express. */
export const UNFILTERABLE_PREFIX = 'type:';

/** A compliance check is a SHELL action carrying the flag, not a distinct
 *  ActionType — the same rule the list row's display info applies. */
export function isCompliance(a: ManagedAction): boolean {
	return a.type === ActionType.SHELL && a.params.case === 'shell' && a.params.value.isCompliance;
}

/**
 * The bucket an action is counted in. `slugByType` is the exact inverse of the
 * map the page's `filterToTags` reads, so a filterable bucket id is always a
 * slug the server-side type filter understands.
 */
export function bucketOf(a: ManagedAction, slugByType: ReadonlyMap<number, string>): string {
	if (isCompliance(a)) return COMPLIANCE_BUCKET;
	return slugByType.get(a.type) ?? UNFILTERABLE_PREFIX + a.type;
}

/** One action as a tile: its name and the single real binary it carries. */
export interface LibraryAction {
	id: string;
	name: string;
	/** desired_state === ABSENT — the list row's "Remove" chip. */
	absent: boolean;
}

export interface LibraryBubble {
	/** Filter slug, `compliance`, or `type:<n>` when the filter cannot name it. */
	id: string;
	/** ActionType behind the bucket — the icon and label source. */
	type: ActionType;
	/** True for the virtual compliance-check bucket. */
	compliance: boolean;
	/** False when clicking this bubble could not produce an honest filter. */
	filterable: boolean;
	actions: LibraryAction[];
	/** Members whose desired state is ABSENT. */
	remove: number;
}

function toLibraryAction(a: ManagedAction): LibraryAction {
	return { id: a.id, name: a.name, absent: a.desiredState === DesiredState.ABSENT };
}

/** Removals first, then by name, then by id — deterministic and locale-stable
 *  enough that the same snapshot always paints the same grid. */
function orderActions(actions: LibraryAction[]): LibraryAction[] {
	return [...actions].sort(
		(a, b) =>
			Number(b.absent) - Number(a.absent) || a.name.localeCompare(b.name) || a.id.localeCompare(b.id)
	);
}

/**
 * One bubble per bucket PRESENT in the snapshot — never one per bucket the
 * enum could hold, so an empty bubble can't imply a type nobody uses exists.
 * Filterable buckets lead (biggest first); the buckets the filter cannot name
 * trail, mirroring the fleet's trailing "ungrouped" bubble.
 */
export function buildBubbles(
	actions: ManagedAction[],
	slugByType: ReadonlyMap<number, string>
): LibraryBubble[] {
	const byBucket = new Map<string, LibraryBubble>();
	for (const a of actions) {
		const id = bucketOf(a, slugByType);
		let bubble = byBucket.get(id);
		if (!bubble) {
			bubble = {
				id,
				type: a.type,
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

/** Counts per desired state. DesiredState holds exactly PRESENT and ABSENT, and
 *  the two branches below are exclusive by construction, so
 *  `install + remove === total` is an invariant rather than a coincidence. */
export function summarize(actions: ManagedAction[]): LibrarySummary {
	const out: LibrarySummary = { total: actions.length, install: 0, remove: 0 };
	for (const a of actions) {
		if (a.desiredState === DesiredState.ABSENT) out.remove++;
		else out.install++;
	}
	return out;
}
