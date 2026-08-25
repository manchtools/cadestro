

import * as m from '$lib/paraglide/messages';
import { readFlags, writeFlags, type OnboardingFlags } from './storage';
import { TOUR_STEPS, presentSteps, type TourStep } from './steps';

interface OnboardingState {

	scope: string | null;
	flags: OnboardingFlags;
	welcomeOpen: boolean;

	steps: TourStep[];
	index: number;
	running: boolean;

	announcement: string;

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

export function resetOnboarding() {
	Object.assign(onboarding, initial());
}

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

export function skipTour() {
	if (!onboarding.running) return;
	end(m.onboarding_tour_skipped());
}

export function finishTour() {
	if (!onboarding.running) return;
	persist({ tourCompleted: true });
	end(m.onboarding_tour_finished());
}

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
