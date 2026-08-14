// Shape contract between the two pipeline builders (action sets, definitions)
// and the generic rail/palette chrome they share. Deliberately free of proto
// types: the rail renders already-resolved display text so it never has to know
// whether a step is a ManagedAction or an ActionSet.

import type { Component } from 'svelte';

/** The desired-state toggle on a collapsed step (B1). `run` is the single-state
 *  posture for action types that have no PRESENT/ABSENT axis (scripts, updates). */
export type StepState = 'present' | 'absent' | 'run';

export interface RailStep {
	/** Stable client-side identity — survives reorder, unlike the index. */
	key: string;
	title: string;
	/** One-line mono summary under the title. */
	summary: string;
	icon?: Component;
	/** Omitted for steps that have no desired-state axis at all (action sets). */
	state?: StepState;
	/** Which toggle segments render; `[state]` for a single-posture step. */
	stateOptions?: StepState[];
	/** Non-empty means the step blocks the commit and gets the crit treatment. */
	error?: string;
}

/** A draggable / keyboard-insertable palette entry. */
export interface PaletteEntry {
	id: string;
	label: string;
	/** Secondary line (type description, member count, …). */
	hint?: string;
	icon?: Component;
}

/** A palette row in the Movement C set-picker idiom (definitions). */
export interface SetOption {
	id: string;
	name: string;
	memberCount: number;
}

/** MIME type for palette→pipeline drags. A private type keeps foreign drags
 *  (files, text selections) from being read as step inserts. */
export const STEP_DND_TYPE = 'application/x-cadestro-step';
