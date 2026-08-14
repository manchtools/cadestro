// The /actions/new type wall, derived — never hand-listed.
//
// Two facts already live in the codebase and this module only joins them:
//
//   - the SDK's ACTION_TYPE_OPTIONS is the complete set of orchestrator-level
//     action-type strings (plus the synthetic COMPLIANCE_CHECK key, which is a
//     stricter SHELL and has no proto enum of its own);
//   - ACTION_REGISTRY is the only place that says which of those are actually
//     creatable — a type with no adapter has no params form, no schema and no
//     proto conversion, so it cannot be created from a form at all.
//
// Intersecting them means a new registry entry appears on the wall the moment it
// is registered, and REBOOT / SYNC / SCRIPT_RUN (no adapter) stay off it without
// anyone maintaining an exclusion list. `tileFormKeys()` lets a test assert the
// wall covers the registry exactly, in both directions.

import { ACTION_REGISTRY, formKeyFromString, type FormKey } from '$lib/components/actions/registry';
import { ACTION_TYPE_OPTIONS } from '$contractClient/action-types';

/** COMPLIANCE_CHECK has no proto enum — it is a SHELL action the registry
 *  validates more strictly — so it is named here rather than discovered. */
export const COMPLIANCE_KEY = 'COMPLIANCE_CHECK';

/** Resolve the registry adapter behind an orchestrator-level type string.
 *  Returns null for a string no adapter claims. */
export function formKeyForTypeValue(value: string): FormKey | null {
	if (value === COMPLIANCE_KEY) return COMPLIANCE_KEY;
	const key = formKeyFromString(value);
	return key && key in ACTION_REGISTRY ? key : null;
}

/** Every action type the operator can create, as orchestrator-level strings.
 *  APP_IMAGE / DEB / RPM all resolve to the shared APP adapter and stay three
 *  distinct tiles — they are three different action types with one params form,
 *  and collapsing them would silently drop two of them from the surface. */
export const TILE_VALUES: readonly string[] = [
	...ACTION_TYPE_OPTIONS.map((o) => o.value),
	COMPLIANCE_KEY
].filter((value) => formKeyForTypeValue(value) !== null);

/** The registry entries the wall reaches. A test compares this against
 *  ACTION_REGISTRY's own keys, so an adapter added without a reachable tile
 *  fails loudly instead of being quietly uncreatable. */
export function tileFormKeys(): FormKey[] {
	const keys = new Set<FormKey>();
	for (const value of TILE_VALUES) {
		const key = formKeyForTypeValue(value);
		if (key) keys.add(key);
	}
	return [...keys];
}
