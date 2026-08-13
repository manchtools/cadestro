// The guided-tour script.
//
// Every step is bound to an anchor that ANOTHER lane placed in the real UI
// (`data-tour="…"`, with a couple of stable `data-testid` fallbacks for chrome
// that carries no tour attribute). Nothing here invents a target: a step whose
// anchors are all absent from the DOM is dropped before the tour starts, so the
// counter reads "Step 3 of 5" over the steps that can actually be shown.
//
// The order teaches the deliberately unconventional parts first — the pill IS
// the navigation, ⌘K is the way to everything else — then the fleet surface,
// then how work is carried and parked.
import * as m from '$lib/paraglide/messages';

export interface TourAnchor {
	/** Selector the step is bound to; the step runs only if this resolves. */
	sel: string;
	/** When set, the spotlight lifts from the matched element to this enclosing
	 *  element — the pill's overflow button is the only tour anchor inside the
	 *  pill, but the step is about the whole pill. */
	lift?: string;
}

export interface TourStep {
	id: string;
	/** Tried in order; the first anchor present in the DOM wins. */
	anchors: TourAnchor[];
	title: () => string;
	body: () => string;
}

const PILL = '[data-testid="pill"]';
const OVERFLOW = '[data-tour="nav-pill-overflow"]';

export const TOUR_STEPS: TourStep[] = [
	{
		id: 'pill',
		anchors: [{ sel: OVERFLOW, lift: PILL }, { sel: PILL }],
		title: m.onboarding_step_pill_title,
		body: m.onboarding_step_pill_body
	},
	{
		id: 'search',
		// The palette anchors exist only while ⌘K is open; the pill hosts the
		// control that opens it, so it is the honest fallback target.
		anchors: [
			{ sel: '[data-tour="palette-input"]' },
			{ sel: '[data-tour="palette-facets"]' },
			{ sel: OVERFLOW, lift: PILL }
		],
		title: m.onboarding_step_search_title,
		body: m.onboarding_step_search_body
	},
	{
		id: 'tiles',
		anchors: [{ sel: '[data-tour="fleet-legend"]' }, { sel: '[data-tour="fleet-grid"]' }],
		title: m.onboarding_step_tiles_title,
		body: m.onboarding_step_tiles_body
	},
	{
		// The summary strip is the only place the fleet states itself in numbers,
		// and it sits directly above the tiles the previous step just explained.
		id: 'summary',
		anchors: [{ sel: '[data-tour="fleet-summary"]' }],
		title: m.onboarding_step_summary_title,
		body: m.onboarding_step_summary_body
	},
	{
		id: 'zoom',
		anchors: [{ sel: '[data-tour="fleet-zoom"]' }],
		title: m.onboarding_step_zoom_title,
		body: m.onboarding_step_zoom_body
	},
	{
		id: 'selection',
		anchors: [{ sel: '[data-tour="fleet-grid"]' }, { sel: '[data-tour="assign-carried"]' }],
		title: m.onboarding_step_selection_title,
		body: m.onboarding_step_selection_body
	},
	{
		id: 'stage',
		// The rail renders only while something is parked on it; the fleet grid is
		// where a window is opened from, so it explains the same mechanic.
		anchors: [
			{ sel: '[data-tour="stage-rail"]' },
			{ sel: '[data-testid="stage-rail"]' },
			{ sel: '[data-tour="fleet-grid"]' }
		],
		title: m.onboarding_step_stage_title,
		body: m.onboarding_step_stage_body
	},
	{
		id: 'more',
		anchors: [{ sel: OVERFLOW }],
		title: m.onboarding_step_more_title,
		body: m.onboarding_step_more_body
	}
];

function visible(el: Element): el is HTMLElement {
	if (!(el instanceof HTMLElement)) return false;
	const r = el.getBoundingClientRect();
	return r.width > 0 && r.height > 0;
}

/** Resolve a step to the element the spotlight should ring, or null when none
 *  of its anchors is on the page. */
export function resolveStep(step: TourStep, root: ParentNode = document): HTMLElement | null {
	for (const anchor of step.anchors) {
		const el = root.querySelector(anchor.sel);
		if (!el || !visible(el)) continue;
		if (!anchor.lift) return el;
		const lifted = el.closest<HTMLElement>(anchor.lift);
		return lifted && visible(lifted) ? lifted : el;
	}
	return null;
}

/** The steps that can actually be shown right now, in script order. */
export function presentSteps(steps: TourStep[] = TOUR_STEPS, root: ParentNode = document): TourStep[] {
	return steps.filter((s) => resolveStep(s, root) !== null);
}
