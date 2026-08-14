// Re-export everything from the Svelte 5 wrappers.
// This file exists because TypeScript module resolution does not resolve
// `.svelte.ts` files for directory index imports (`$lib/sdk`).
export {
	authStore,
	configStore,
	offlineStore,
	apiClient,
	useDraft,
	formatTimestamp,
	formatTimestampDateTime,
	formatDuration
} from './wrappers.svelte';

export type {
	DraftType,
	ServerConfig,
	User,
	Device,
	RegistrationToken,
	ManagedAction,
	ActionSet,
	Definition,
	DeviceGroup,
	Assignment,
	ActionExecution,
	AuditEvent,
	InventoryTableResult,
	Role,
	PermissionInfo,
	UserGroup,
	UserGroupMember,
	IdentityProvider,
	IdentityLink,
	LpsPassword,
	LuksKey
} from './wrappers.svelte';

export * from '$contract/cadestro/v1/control_pb';
export * from '$contract/cadestro/v1/actions_pb';
export * from '$contract/cadestro/v1/common_pb';

// Picker pagination helper — see ./paginate.ts.
export { fetchAllPages, type PageResult, type PaginateOptions } from './paginate';
