// Per-orchestrator helpers that turn the ACTION_REGISTRY into a uniform
// set of per-FormKey state + validation handles. Both action-create-form
// and edit-params-dialog use the same bundle; the bundle is the only thing
// they need to know about the 19-action-type ladder.
//
// Implementation note: this file uses Svelte 5 `$state` runes, so it must
// be `.svelte.ts` to participate in the runes compiler.

import { createFormValidation, type FormValidation } from '$lib/forms';
import {
	ACTION_REGISTRY,
	FORM_KEYS,
	type FormKey
} from '../registry';

export type FormState = Record<string, unknown>;

export interface FormBundle {
	/** Reactive map of per-FormKey form state. Mutate via
	 *  `bundle.params[key] = newValue` or via `bind:params={bundle.params[key]}`.
	 *  Defaults are populated lazily on access via the registry. */
	params: Record<FormKey, FormState>;
	/** Per-FormKey validation handle. */
	validations: Record<FormKey, FormValidation<FormState>>;
	/** Reset all per-type validation error state. */
	clearAllErrors(): void;
	/** Validate the form state for the given FormKey. */
	validate(key: FormKey): boolean;
	/** Replace the form state for `key`. */
	set(key: FormKey, value: FormState): void;
}

export function createFormBundle(): FormBundle {
	// Per-FormKey state. We seed every entry up-front — there are only 19
	// of them and the seed cost is one default-factory call each.
	const params = $state({} as Record<FormKey, FormState>);
	for (const key of FORM_KEYS) {
		params[key] = ACTION_REGISTRY[key].defaultForm() as FormState;
	}

	const validations = {} as Record<FormKey, FormValidation<FormState>>;
	for (const key of FORM_KEYS) {
		validations[key] = createFormValidation(ACTION_REGISTRY[key].schema) as FormValidation<FormState>;
	}

	return {
		params,
		validations,
		clearAllErrors() {
			for (const key of FORM_KEYS) {
				validations[key].clearErrors();
			}
		},
		validate(key: FormKey) {
			return validations[key].validate(params[key]);
		},
		set(key: FormKey, value: FormState) {
			params[key] = value;
		}
	};
}
