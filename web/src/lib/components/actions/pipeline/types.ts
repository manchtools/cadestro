

import type { Component } from 'svelte';

export type StepState = 'present' | 'absent' | 'run';

export interface RailStep {

	key: string;
	title: string;

	summary: string;
	icon?: Component;

	state?: StepState;

	stateOptions?: StepState[];

	error?: string;
}

export interface PaletteEntry {
	id: string;
	label: string;

	hint?: string;
	icon?: Component;
}

export interface SetOption {
	id: string;
	name: string;
	memberCount: number;
}

export const STEP_DND_TYPE = 'application/x-cadestro-step';
