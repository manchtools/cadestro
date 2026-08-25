

import * as m from '$lib/paraglide/messages';

export interface TourAnchor {

	sel: string;

	lift?: string;
}

export interface TourStep {
	id: string;

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

export function presentSteps(steps: TourStep[] = TOUR_STEPS, root: ParentNode = document): TourStep[] {
	return steps.filter((s) => resolveStep(s, root) !== null);
}
