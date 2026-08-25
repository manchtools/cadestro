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
	type AuditEvent,
	AuditEventSchema,
	type SearchResult
} from '$contract/cadestro/v1/control_pb';

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

export function searchResultToManagedAction(r: SearchResult): ManagedAction {
	const f = r.fields;
	const type = intOr(f['type'], ActionType.UNSPECIFIED) as ActionType;
	const isCompliance = f['is_compliance'] === 'true';

	const action = create(ManagedActionSchema, {
		id: { value: r.id?.value ?? '' },
		name: f['name'] ?? r.name,
		description: f['description'] ?? r.description,
		type,

		desiredState: intOr(f['desired_state'], DesiredState.PRESENT) as DesiredState,
		createdAt: timestampFromSeconds(f['created_at']),
		updatedAt: timestampFromSeconds(f['updated_at'])
	});

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
		id: { value: r.id?.value ?? '' },
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
		id: { value: r.id?.value ?? '' },
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
		id: { value: r.id?.value ?? '' },
		name: f['name'] ?? r.name,
		description: f['description'] ?? r.description
	});
}

export function searchResultToDeviceGroup(r: SearchResult): DeviceGroup {
	const f = r.fields;
	return create(DeviceGroupSchema, {
		id: { value: r.id?.value ?? '' },
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
		id: { value: r.id?.value ?? '' },
		name: f['name'] ?? r.name,
		description: f['description'] ?? r.description,
		isDynamic: f['is_dynamic'] === 'true',

		isScimManaged: f['is_scim_managed'] === 'true',
		memberCount: intOr(f['member_count'], 0),
		createdAt: timestampFromSeconds(f['created_at'])
	});
}

export function searchResultToUser(r: SearchResult): User {
	const f = r.fields;
	return create(UserSchema, {
		id: { value: r.id?.value ?? '' },
		email: f['email'] ?? r.name,
		displayName: f['display_name'] ?? '',
		linuxUsername: f['linux_username'] ?? '',
		disabled: f['disabled'] === 'true',

		lastLoginAt: timestampFromSeconds(f['last_login_at']),
		createdAt: timestampFromSeconds(f['created_at'])
	});
}

export interface SearchRoleRef {
	id: string;
	name: string;
}

function splitAligned(ids: string | undefined, names: string | undefined): SearchRoleRef[] {
	if (!ids) return [];
	const idList = ids.split(', ').filter(Boolean);
	const nameList = (names ?? '').split(', ');
	return idList.map((id, i) => ({ id, name: nameList[i]?.trim() || id }));
}

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

export function searchResultToDevice(r: SearchResult): Device {
	const f = r.fields;

	const lastSeenSec = parseInt(f['last_seen_at'] ?? '0', 10);
	const nowSec = Math.floor(Date.now() / 1000);
	const status =
		lastSeenSec > 0 && nowSec - lastSeenSec < 5 * 60
			? DeviceStatus.ONLINE
			: DeviceStatus.OFFLINE;
	return create(DeviceSchema, {
		id: { value: r.id?.value ?? '' },
		hostname: f['hostname'] ?? r.name,
		agentVersion: f['agent_version'] ?? '',
		status,
		complianceStatus: intOr(f['compliance_status'], 0),
		registeredAt: timestampFromSeconds(f['registered_at']),
		lastSeenAt: timestampFromSeconds(f['last_seen_at'])
	});
}

export function searchResultToAuditEvent(r: SearchResult): AuditEvent {
	const f = r.fields;
	return create(AuditEventSchema, {
		id: { value: r.id?.value ?? '' },
		streamType: f['stream_type'] ?? '',
		streamId: f['stream_id'] ? { value: f['stream_id'] } : undefined,
		eventType: f['event_type'] ?? '',
		actorType: f['actor_type'] ?? '',
		actorId: f['actor_id'] ? { value: f['actor_id'] } : undefined,
		occurredAt: timestampFromSeconds(f['occurred_at'])
	});
}
