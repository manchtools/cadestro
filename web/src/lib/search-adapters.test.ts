// The list surfaces are rendered from SEARCH documents, not from the List RPCs,
// so these adapters decide what the operator sees in a row. A field the row
// renders but the document does not carry has no honest value here — and the
// tempting fallback is a plausible constant, which is a fabrication the UI then
// states as fact.
//
// That is exactly what happened to `desiredState`: it was hardcoded to PRESENT,
// so every remove-action read "Install" in the actions list while its own detail
// page correctly said "Remove".
import { describe, it, expect } from 'vitest';
import { create } from '@bufbuild/protobuf';
import { SearchResultSchema } from '$contract/cadestro/v1/control_pb';
import { DesiredState } from '$contract/cadestro/v1/common_pb';
import { ActionType } from '$contract/cadestro/v1/actions_pb';

import {
	searchResultToManagedAction,
	searchResultToUser,
	searchResultToUserGroup,
	searchResultUserRoles
} from './search-adapters';

function result(fields: Record<string, string>) {
	return create(SearchResultSchema, {
		id: '01JQZZACTION0000000000000A',
		name: 'curl',
		description: '',
		fields
	});
}

describe('searchResultToManagedAction', () => {
	it('reads the desired state from the document instead of assuming install', () => {
		const remove = searchResultToManagedAction(
			result({ type: String(ActionType.PACKAGE), desired_state: '1' })
		);
		expect(remove.desiredState).toBe(DesiredState.ABSENT);

		const install = searchResultToManagedAction(
			result({ type: String(ActionType.PACKAGE), desired_state: '0' })
		);
		expect(install.desiredState).toBe(DesiredState.PRESENT);
	});

	it('falls back to install only when the document truly omits the field', () => {
		// Proto3 JSON omits zero values, so an absent field legitimately means
		// PRESENT. What it must NOT do is swallow a present-but-unparseable value.
		const omitted = searchResultToManagedAction(result({ type: String(ActionType.PACKAGE) }));
		expect(omitted.desiredState).toBe(DesiredState.PRESENT);

		const garbage = searchResultToManagedAction(
			result({ type: String(ActionType.PACKAGE), desired_state: 'not-a-number' })
		);
		expect(garbage.desiredState).toBe(DesiredState.PRESENT);
	});
});

describe('searchResultToUserGroup', () => {
	// The document emits is_scim_managed now (server 031605d); the flag gates the
	// list's delete guard, so it must come off the document, never a constant.
	it('reads is_scim_managed from the document', () => {
		expect(searchResultToUserGroup(result({ is_scim_managed: 'true' })).isScimManaged).toBe(true);
		expect(searchResultToUserGroup(result({ is_scim_managed: 'false' })).isScimManaged).toBe(false);
		// absent field = not SCIM-managed (proto3 JSON omits false)
		expect(searchResultToUserGroup(result({})).isScimManaged).toBe(false);
	});
});

describe('searchResultToUser', () => {
	it("reads last_login_at from the document, with '0' meaning never", () => {
		const seen = searchResultToUser(result({ last_login_at: '1750000000' }));
		expect(seen.lastLoginAt?.seconds).toBe(1750000000n);

		const never = searchResultToUser(result({ last_login_at: '0' }));
		expect(never.lastLoginAt).toBeUndefined();

		const absent = searchResultToUser(result({}));
		expect(absent.lastLoginAt).toBeUndefined();
	});
});

describe('searchResultUserRoles', () => {
	const ADMIN = '00000000000000000000000001';
	const HELP = '01JR0ROLEHELPDESK000000000';
	const VIEW = '01JR0ROLEVIEWER00000000000';

	it('splits the aligned id/name lists positionally', () => {
		const roles = searchResultUserRoles(
			result({ role_ids: `${ADMIN}, ${HELP}`, role_names: 'Admin, Helpdesk' })
		);
		expect(roles.direct).toEqual([
			{ id: ADMIN, name: 'Admin' },
			{ id: HELP, name: 'Helpdesk' }
		]);
		expect(roles.inherited).toEqual([]);
	});

	it('keeps one direct entry per GRANT — a twice-scoped role appears twice, like its grants', () => {
		const roles = searchResultUserRoles(
			result({ role_ids: `${HELP}, ${HELP}`, role_names: 'Helpdesk, Helpdesk' })
		);
		expect(roles.direct).toHaveLength(2);
	});

	it('dedupes inherited (role, group) pairs by role id', () => {
		const roles = searchResultUserRoles(
			result({
				// The same role through two live groups is ONE chip.
				inherited_role_ids: `${VIEW}, ${VIEW}, ${HELP}`,
				inherited_role_names: 'Viewer, Viewer, Helpdesk'
			})
		);
		expect(roles.inherited).toEqual([
			{ id: VIEW, name: 'Viewer' },
			{ id: HELP, name: 'Helpdesk' }
		]);
	});

	it('prefers the ids list on a count mismatch (comma-bearing role name)', () => {
		// "Ops, EMEA" split into two fragments: the ids stay the source of truth
		// for count and identity; the entry whose name is unrecoverable shows its
		// id instead of a neighbour's fragment.
		const roles = searchResultUserRoles(
			result({ role_ids: `${ADMIN}, ${HELP}`, role_names: 'Admin, Ops, EMEA' })
		);
		expect(roles.direct).toHaveLength(2);
		expect(roles.direct[0]).toEqual({ id: ADMIN, name: 'Admin' });
		expect(roles.direct[1].id).toBe(HELP);
	});

	it('returns empty sets when the document carries no role fields', () => {
		expect(searchResultUserRoles(result({}))).toEqual({ direct: [], inherited: [] });
	});
});
