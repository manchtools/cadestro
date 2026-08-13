// Onboarding state: the first-run welcome and the guided tour.
//
// A module-level `$state` singleton, like the shell store — the host is mounted
// once above the router outlet, so one instance is exactly right. This module
// stays free of the API client: it knows a scope string, never how the scope was
// derived, which keeps the state machine unit-testable without a server.
import * as m from '$lib/paraglide/messages';
import { readFlags, writeFlags, type OnboardingFlags } from './storage';
import { TOUR_STEPS, presentSteps, type TourStep } from './steps';

interface OnboardingState {
	/** `(server, user)` scope for persistence; null until the host resolves it. */
	scope: string | null;
	flags: OnboardingFlags;
	welcomeOpen: boolean;
	/** Steps for THIS run — only the ones whose anchors were on the page. */
	steps: TourStep[];
	index: number;
	running: boolean;
	/** Polite live-region text for step changes. */
	announcement: string;
	/** Set when the tour ends, so the operator learns it can be restarted. */
	endNote: string;
}

function initial(): OnboardingState {
	return {
		scope: null,
		flags: { welcomeSeen: false, tourCompleted: false, checklistDismissed: false },
		welcomeOpen: false,
		steps: [],
		index: 0,
		running: false,
		announcement: '',
		endNote: ''
	};
}

export const onboarding = $state<OnboardingState>(initial());

/** Test seam — restore a pristine store between tests. */
export function resetOnboarding() {
	Object.assign(onboarding, initial());
}

/**
 * Bind the store to a (server, user) scope and decide whether this is a first
 * run. Idempotent for a given scope, so a layout re-render never re-opens the
 * welcome. The welcome is marked seen the moment it is shown: it is a courtesy,
 * not a gate, and it must not reappear because a reload happened mid-read.
 */
export function initOnboarding(scope: string) {
	if (onboarding.scope === scope) return;
	onboarding.scope = scope;
	onboarding.flags = readFlags(scope);
	if (!onboarding.flags.welcomeSeen) {
		onboarding.welcomeOpen = true;
		onboarding.flags = writeFlags(scope, { welcomeSeen: true });
	}
}

function persist(patch: Partial<OnboardingFlags>) {
	if (!onboarding.scope) {
		onboarding.flags = { ...onboarding.flags, ...patch };
		return;
	}
	onboarding.flags = writeFlags(onboarding.scope, patch);
}

export function closeWelcome() {
	onboarding.welcomeOpen = false;
}

export function currentStep(): TourStep | null {
	return onboarding.steps[onboarding.index] ?? null;
}

function announce() {
	const step = currentStep();
	if (!step) return;
	onboarding.announcement = m.onboarding_tour_announce({
		current: onboarding.index + 1,
		total: onboarding.steps.length,
		title: step.title()
	});
}

/**
 * Start (or restart) the tour. The step list is resolved against the LIVE DOM,
 * so the run only ever contains steps that have something to point at; with no
 * resolvable anchor at all the tour simply does not open.
 *
 * This is the public entry point the Settings surface calls to replay the tour.
 */
export function startTour(steps: TourStep[] = TOUR_STEPS, root: ParentNode = document): boolean {
	const runnable = presentSteps(steps, root);
	onboarding.welcomeOpen = false;
	onboarding.endNote = '';
	if (runnable.length === 0) {
		onboarding.running = false;
		onboarding.steps = [];
		onboarding.announcement = m.onboarding_tour_unavailable();
		return false;
	}
	onboarding.steps = runnable;
	onboarding.index = 0;
	onboarding.running = true;
	announce();
	return true;
}

export function nextStep() {
	if (!onboarding.running) return;
	if (onboarding.index >= onboarding.steps.length - 1) {
		finishTour();
		return;
	}
	onboarding.index += 1;
	announce();
}

export function prevStep() {
	if (!onboarding.running || onboarding.index === 0) return;
	onboarding.index -= 1;
	announce();
}

function end(note: string) {
	onboarding.running = false;
	onboarding.steps = [];
	onboarding.index = 0;
	onboarding.announcement = note;
	onboarding.endNote = note;
}

/** Esc, "Skip tour", or the backdrop: stop without marking the tour completed. */
export function skipTour() {
	if (!onboarding.running) return;
	end(m.onboarding_tour_skipped());
}

/** "Done" on the last step. */
export function finishTour() {
	if (!onboarding.running) return;
	persist({ tourCompleted: true });
	end(m.onboarding_tour_finished());
}

/**
 * The current step's anchor left the DOM while the tour was open (a panel
 * closed, a list emptied). Drop that step instead of ringing empty space.
 */
export function dropCurrentStep() {
	if (!onboarding.running) return;
	const at = onboarding.index;
	onboarding.steps = onboarding.steps.filter((_, i) => i !== at);
	if (onboarding.steps.length === 0) {
		end(m.onboarding_tour_skipped());
		return;
	}
	if (onboarding.index > onboarding.steps.length - 1) onboarding.index = onboarding.steps.length - 1;
	announce();
}

export function dismissChecklist() {
	persist({ checklistDismissed: true });
}
