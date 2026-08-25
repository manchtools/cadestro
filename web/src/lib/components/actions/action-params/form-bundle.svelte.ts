

import { createFormValidation, type FormValidation } from '$lib/forms';
import {
	ACTION_REGISTRY,
	FORM_KEYS,
	type FormKey
} from '../registry';
import type { FormStateByKey } from '../forms/types';

export interface FormBundle {

	params: FormStateByKey;

	validations: { [K in FormKey]: FormValidation<FormStateByKey[K]> };

	clearAllErrors(): void;

	validate(key: FormKey): boolean;

	set<K extends FormKey>(key: K, value: FormStateByKey[K]): void;
	clearFieldError(key: FormKey, field: string): void;
}

export function createFormBundle(): FormBundle {

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
