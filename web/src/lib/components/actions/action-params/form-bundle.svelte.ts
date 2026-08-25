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
import type { FormStateByKey } from '../forms/types';

export interface FormBundle {
	/** Reactive map of per-FormKey form state. Mutate via
	 *  `bundle.params[key] = newValue` or via `bind:params={bundle.params[key]}`.
	 *  Defaults are populated lazily on access via the registry. */
	params: FormStateByKey;
	/** Per-FormKey validation handle. */
	validations: { [K in FormKey]: FormValidation<FormStateByKey[K]> };
	/** Reset all per-type validation error state. */
	clearAllErrors(): void;
	/** Validate the form state for the given FormKey. */
	validate(key: FormKey): boolean;
	/** Replace the form state for `key`. */
	set<K extends FormKey>(key: K, value: FormStateByKey[K]): void;
	clearFieldError(key: FormKey, field: string): void;
}

export function createFormBundle(): FormBundle {
	// Per-FormKey state. We seed every entry up-front — there are only 19
	// of them and the seed cost is one default-factory call each.
	const params = $state({} as FormStateByKey);
	function setParam<K extends FormKey>(key: K, value: FormStateByKey[K]) {
		params[key] = value;
	}
	for (const key of FORM_KEYS) {
		setParam(key, ACTION_REGISTRY[key].defaultForm() as FormStateByKey[typeof key]);
	}

	const validations = {} as FormBundle['validations'];
	function setValidation<K extends FormKey>(key: K, value: FormBundle['validations'][K]) {
		validations[key] = value;
	}
	for (const key of FORM_KEYS) {
		setValidation(key, createFormValidation(ACTION_REGISTRY[key].schema) as FormBundle['validations'][typeof key]);
	}

	return {
		params,
		validations,
		clearAllErrors() {
			for (const key of FORM_KEYS) {
				validations[key].clearErrors();
			}
		},
		validate<K extends FormKey>(key: K) {
			return validations[key].validate(params[key] as never);
		},
		set<K extends FormKey>(key: K, value: FormStateByKey[K]) {
			setParam(key, value);
		},
		clearFieldError(key: FormKey, field: string) {
			validations[key].clearFieldError(field as never);
		}
	};
}
