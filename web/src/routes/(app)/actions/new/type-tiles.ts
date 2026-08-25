

import { ACTION_REGISTRY, formKeyFromString, type FormKey } from '$lib/components/actions/registry';
import { ACTION_TYPE_OPTIONS } from '$contractClient/action-types';

export const COMPLIANCE_KEY = 'COMPLIANCE_CHECK';

export function formKeyForTypeValue(value: string): FormKey | null {
	if (value === COMPLIANCE_KEY) return COMPLIANCE_KEY;
	const key = formKeyFromString(value);
	return key && key in ACTION_REGISTRY ? key : null;
}

export const TILE_VALUES: readonly string[] = [
	...ACTION_TYPE_OPTIONS.map((o) => o.value),
	COMPLIANCE_KEY
].filter((value) => formKeyForTypeValue(value) !== null);

export function tileFormKeys(): FormKey[] {
	const keys = new Set<FormKey>();
	for (const value of TILE_VALUES) {
		const key = formKeyForTypeValue(value);
		if (key) keys.add(key);
	}
	return [...keys];
}
