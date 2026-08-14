import { create } from '@bufbuild/protobuf';
import { TimestampSchema } from '@bufbuild/protobuf/wkt';
import { ActionType, ShellParamsSchema } from '$contract/cadestro/v1/actions_pb';
import { DesiredState, DeviceStatus } from '$contract/cadestro/v1/common_pb';
import {
	type ManagedAction,
	ManagedActionSchema,
	type ActionSet,
	ActionSetSchema,
	type Definition,
	DefinitionSchema,
	type CompliancePolicy,
	CompliancePolicySchema,
	type DeviceGroup,
	DeviceGroupSchema,
	type UserGroup,
	UserGroupSchema,
	type User,
	UserSchema,
	type Device,
	DeviceSchema,
	type ActionExecution,
	ActionExecutionSchema,
	type AuditEvent,
	AuditEventSchema,
	type SearchResult
} from '$contract/cadestro/v1/control_pb';

// List pages receive SearchResult.fields as map<string, string>. These adapters
// reconstruct only the typed fields needed by list rendering. Detail pages call
// Get<Entity> for the complete record; search results never stand in for it.

function timestampFromSeconds(seconds: string | undefined) {
	if (!seconds) return undefined;
	const n = BigInt(seconds);
	if (n === 0n) return undefined;
	return create(TimestampSchema, { seconds: n, nanos: 0 });
}

function intOr<T extends number>(s: string | undefined, fallback: T): T | number {
	if (!s) return fallback;
	const n = parseInt(s, 10);
	return Number.isFinite(n) ? n : fallback;
}

/**
 * Reconstruct a ManagedAction-shaped object from a SearchResult.
 *
 * Only the fields the list-page table renders are populated:
 *   id, name, description, type, createdAt, updatedAt, and a
 *   minimal `params` oneof carrying isCompliance for the
 *   compliance-check badge logic. Everything else (script, package
 *   names, schedule, etc.) is left as the proto default — list pages
 *   don't render those.
 *
 * The compliance flag's path is preserved so existing
 * isComplianceAction(a) checks (`a.params.case === 'shell' &&
 * a.params.value.isCompliance`) keep working without page changes.
 */
export function searchResultToManagedAction(r: SearchResult): ManagedAction {
	const f = r.fields;
	const type = intOr(f['type'], ActionType.UNSPECIFIED) as ActionType;
	const isCompliance = f['is_compliance'] === 'true';

	const action = create(ManagedActionSchema, {
		id: r.id,
		name: f['name'] ?? r.name,
		description: f['description'] ?? r.description,
		type,
		// From the document, never assumed: hardcoding PRESENT made every
		// remove-action read "Install" in the list. Proto3 JSON omits zero values,
		// so an absent field legitimately means PRESENT.
		desiredState: intOr(f['desired_state'], DesiredState.PRESENT) as DesiredState,
		createdAt: timestampFromSeconds(f['created_at']),
		updatedAt: timestampFromSeconds(f['updated_at'])
	});

	// Only set params when it has meaningful content for rendering.
	// SHELL + isCompliance flips the "compliance check" type badge in
	// getDisplayInfo(); other types render off `type` alone, so an
	// empty params oneof is fine.
	if (type === ActionType.SHELL && isCompliance) {
		action.params = {
			case: 'shell',
			value: create(ShellParamsSchema, { isCompliance: true })
		};
	}

	return action;
}

export function searchResultToActionSet(r: SearchResult): ActionSet {
	const f = r.fields;
	return create(ActionSetSchema, {
		id: r.id,
		name: f['name'] ?? r.name,
		description: f['description'] ?? r.description,
		memberCount: intOr(f['member_count'], 0),
		createdAt: timestampFromSeconds(f['created_at']),
		updatedAt: timestampFromSeconds(f['updated_at'])
	});
}

export function searchResultToDefinition(r: SearchResult): Definition {
	const f = r.fields;
	return create(DefinitionSchema, {
		id: r.id,
		name: f['name'] ?? r.name,
		description: f['description'] ?? r.description,
		memberCount: intOr(f['member_count'], 0),
		createdAt: timestampFromSeconds(f['created_at']),
		updatedAt: timestampFromSeconds(f['updated_at'])
	});
}

export function searchResultToCompliancePolicy(r: SearchResult): CompliancePolicy {
	const f = r.fields;
	return create(CompliancePolicySchema, {
		id: r.id,
		name: f['name'] ?? r.name,
		description: f['description'] ?? r.description
	});
}

export function searchResultToDeviceGroup(r: SearchResult): DeviceGroup {
	const f = r.fields;
	return create(DeviceGroupSchema, {
		id: r.id,
		name: f['name'] ?? r.name,
		description: f['description'] ?? r.description,
		isDynamic: f['is_dynamic'] === 'true',
		memberCount: intOr(f['member_count'], 0),
		createdAt: timestampFromSeconds(f['created_at'])
	});
}

export function searchResultToUserGroup(r: SearchResult): UserGroup {
	const f = r.fields;
	return create(UserGroupSchema, {
		id: r.id,
		name: f['name'] ?? r.name,
		description: f['description'] ?? r.description,
		isDynamic: f['is_dynamic'] === 'true',
		// Gates the list's delete guard — from the document (server 031605d),
		// never a constant.
		isScimManaged: f['is_scim_managed'] === 'true',
		memberCount: intOr(f['member_count'], 0),
		createdAt: timestampFromSeconds(f['created_at'])
	});
}

export function searchResultToUser(r: SearchResult): User {
	const f = r.fields;
	return create(UserSchema, {
		id: r.id,
		email: f['email'] ?? r.name,
		displayName: f['display_name'] ?? '',
		linuxUsername: f['linux_username'] ?? '',
		disabled: f['disabled'] === 'true',
		// '0' means never — timestampFromSeconds already maps 0 to undefined.
		lastLoginAt: timestampFromSeconds(f['last_login_at']),
		createdAt: timestampFromSeconds(f['created_at'])
	});
}

/** One role reference off the users search document — id plus display name. */
export interface SearchRoleRef {
	id: string;
	name: string;
}

/** Split a `', '`-joined ids list and pair each id with its positional name.
 *
 *  Ids are ULIDs, so the split is exact. A count mismatch between the ids list
 *  and its names list means a comma-bearing role name (the `action_names`
 *  limitation): the ids stay the source of truth for count and identity, and
 *  names pair positionally — an entry whose name fell victim shows its id,
 *  which is honest, instead of a neighbouring role's name fragment. */
function splitAligned(ids: string | undefined, names: string | undefined): SearchRoleRef[] {
	if (!ids) return [];
	const idList = ids.split(', ').filter(Boolean);
	const nameList = (names ?? '').split(', ');
	return idList.map((id, i) => ({ id, name: nameList[i]?.trim() || id }));
}

/** The users list's role chips, straight off the search document.
 *
 *  `role_ids`/`role_names` are DIRECT grants — one entry per grant, so a role
 *  held at several scopes appears once per grant, exactly like the grants it
 *  mirrors. `inherited_role_ids`/`inherited_role_names` are one entry per
 *  (role, group) pair; the list renders one chip per ROLE, so those are
 *  deduplicated by role id here.
 *
 *  Grant SCOPE KINDS are deliberately NOT reconstructed: the document does not
 *  carry them, and anything that needs them (revoke, unscoped-exclusion) must
 *  read the full GetUser record instead of fabricating UNSPECIFIED. */
export function searchResultUserRoles(r: SearchResult): {
	direct: SearchRoleRef[];
	inherited: SearchRoleRef[];
} {
	const f = r.fields;
	const direct = splitAligned(f['role_ids'], f['role_names']);
	const seen = new Set<string>();
	const inherited = splitAligned(f['inherited_role_ids'], f['inherited_role_names']).filter(
		(role) => {
			if (seen.has(role.id)) return false;
			seen.add(role.id);
			return true;
		}
	);
	return { direct, inherited };
}

/** Device.osName / osVersion / osArch / kernel live on a separate
 *  DeviceInventory record, not on Device. The search index stores
 *  os_arch (etc.) for free-text search but the typed Device proto
 *  only carries hostname / agent_version / status / labels / etc.
 *  Pages that need the OS string can read it off
 *  SearchResult.fields['os_name']. */
export function searchResultToDevice(r: SearchResult): Device {
	const f = r.fields;
	// Connectivity is derived from the last heartbeat and is not stored in the
	// search document. Match the control server's five-minute online window.
	const lastSeenSec = parseInt(f['last_seen_at'] ?? '0', 10);
	const nowSec = Math.floor(Date.now() / 1000);
	const status =
		lastSeenSec > 0 && nowSec - lastSeenSec < 5 * 60
			? DeviceStatus.ONLINE
			: DeviceStatus.OFFLINE;
	return create(DeviceSchema, {
		id: r.id,
		hostname: f['hostname'] ?? r.name,
		agentVersion: f['agent_version'] ?? '',
		status,
		complianceStatus: intOr(f['compliance_status'], 0),
		registeredAt: timestampFromSeconds(f['registered_at']),
		lastSeenAt: timestampFromSeconds(f['last_seen_at'])
	});
}

/** ActionExecution doesn't carry a deviceHostname field — the search
 *  index does (for free-text search), but the typed proto only has
 *  device_id. Pages that want the hostname inline can read it off
 *  SearchResult.fields['device_hostname']. */
export function searchResultToExecution(r: SearchResult): ActionExecution {
	const f = r.fields;
	return create(ActionExecutionSchema, {
		id: r.id,
		actionId: f['action_id'] ?? '',
		actionName: f['action_name'] ?? '',
		type: intOr(f['action_type'], ActionType.UNSPECIFIED) as ActionType,
		deviceId: f['device_id'] ?? '',
		status: intOr(f['status'], 0),
		createdAt: timestampFromSeconds(f['created_at'])
	});
}

export function searchResultToAuditEvent(r: SearchResult): AuditEvent {
	const f = r.fields;
	return create(AuditEventSchema, {
		id: r.id,
		streamType: f['stream_type'] ?? '',
		streamId: f['stream_id'] ?? '',
		eventType: f['event_type'] ?? '',
		actorType: f['actor_type'] ?? '',
		actorId: f['actor_id'] ?? '',
		occurredAt: timestampFromSeconds(f['occurred_at'])
	});
}
