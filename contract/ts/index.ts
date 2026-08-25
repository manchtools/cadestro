

import type { Timestamp } from '@bufbuild/protobuf/wkt';
import { timestampDate } from '@bufbuild/protobuf/wkt';

export { ApiClient, type ClientOptions } from './client';
export type {
	User, Device, RegistrationToken, ManagedAction, ActionSet, Definition,
	DeviceGroup, Assignment, AuditEvent, InventoryTableResult,
	Role, PermissionInfo, UserGroup, UserGroupMember, IdentityProvider, IdentityLink,
	LpsPassword, LuksKey, CreateActionRequest, UpdateActionParamsRequest
} from './client';
export { AuthStore, parseAuth, serializeAuth, type StoredAuth, type RefreshResult } from './auth';
export { ConfigStore, type ServerConfig } from './config';
export { OfflineStore, type DraftType } from './offline';
export { getActionTypeEnum, actionTypeToString, ACTION_TYPE_OPTIONS } from './action-types';
export * from './errors';

export * from '../gen/ts/cadestro/v1/control_pb';
export * from '../gen/ts/cadestro/v1/actions_pb';
export * from '../gen/ts/cadestro/v1/common_pb';

export function formatTimestamp(timestamp: Timestamp | undefined): string {
	if (!timestamp) return 'Never';
	return timestampDate(timestamp).toLocaleDateString();
}

export function formatTimestampDateTime(timestamp: Timestamp | undefined): string {
	if (!timestamp) return 'Never';
	return timestampDate(timestamp).toLocaleString();
}
