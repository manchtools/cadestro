

import { createClient, Code, ConnectError } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { create } from '@bufbuild/protobuf';
import { type Timestamp } from '@bufbuild/protobuf/wkt';

import {
	ControlService,
	RefreshTokenRequestSchema,
	LogoutRequestSchema,
	GetCurrentUserRequestSchema,
	EraseJITUserRequestSchema,
	GetUserRequestSchema,
	ListUsersRequestSchema,
	UpdateUserEmailRequestSchema,
	SetUserDisabledRequestSchema,
	UpdateUserProfileRequestSchema,
	UpdateUserLinuxUsernameRequestSchema,
	AddUserSshKeyRequestSchema,
	RemoveUserSshKeyRequestSchema,
	UpdateUserSshSettingsRequestSchema,
	type SshPublicKey,
	ListDevicesRequestSchema,
	GetDeviceRequestSchema,
	SetDeviceLabelRequestSchema,
	RemoveDeviceLabelRequestSchema,
	AssignDeviceRequestSchema,
	UnassignDeviceRequestSchema,
	ListDeviceAssigneesRequestSchema,
	DeleteDeviceRequestSchema,
	SetDeviceSyncIntervalRequestSchema,
	SetDeviceInventoryIntervalRequestSchema,
	CreateTokenRequestSchema,
	ListTokensRequestSchema,
	RenameTokenRequestSchema,
	SetTokenDisabledRequestSchema,
	DeleteTokenRequestSchema,

	CreateActionRequestSchema,
	GetActionRequestSchema,
	ListActionsRequestSchema,
	RenameActionRequestSchema,
	UpdateActionDescriptionRequestSchema,
	UpdateActionParamsRequestSchema,
	DeleteActionRequestSchema,

	CreateActionSetRequestSchema,
	GetActionSetRequestSchema,
	ListActionSetsRequestSchema,
	RenameActionSetRequestSchema,
	UpdateActionSetDescriptionRequestSchema,
	UpdateActionSetScheduleRequestSchema,
	DeleteActionSetRequestSchema,
	AddActionToSetRequestSchema,
	RemoveActionFromSetRequestSchema,
	ReorderActionInSetRequestSchema,

	CreateDefinitionRequestSchema,
	GetDefinitionRequestSchema,
	ListDefinitionsRequestSchema,
	RenameDefinitionRequestSchema,
	UpdateDefinitionDescriptionRequestSchema,
	UpdateDefinitionScheduleRequestSchema,
	DeleteDefinitionRequestSchema,
	AddActionSetToDefinitionRequestSchema,
	RemoveActionSetFromDefinitionRequestSchema,
	ReorderActionSetInDefinitionRequestSchema,

	CreateDeviceGroupRequestSchema,
	GetDeviceGroupRequestSchema,
	ListDeviceGroupsRequestSchema,
	ListDeviceGroupsForDeviceRequestSchema,
	RenameDeviceGroupRequestSchema,
	UpdateDeviceGroupDescriptionRequestSchema,
	UpdateDeviceGroupQueryRequestSchema,
	DeleteDeviceGroupRequestSchema,
	AddDeviceToGroupRequestSchema,
	RemoveDeviceFromGroupRequestSchema,
	ValidateDynamicQueryRequestSchema,
	EvaluateDynamicGroupRequestSchema,
	SetDeviceGroupSyncIntervalRequestSchema,
	SetDeviceGroupInventoryIntervalRequestSchema,
	SetDeviceGroupMaintenanceWindowRequestSchema,
	SetUserGroupMaintenanceWindowRequestSchema,

	CreateAssignmentRequestSchema,
	DeleteAssignmentRequestSchema,
	ListAssignmentsRequestSchema,
	GetDeviceAssignmentsRequestSchema,
	GetUserAssignmentsRequestSchema,

	ListAvailableActionsRequestSchema,
	SetUserSelectionRequestSchema,

	RebootDeviceRequestSchema,
	SyncDeviceRequestSchema,

	ListAuditEventsRequestSchema,
	ExportAuditEventsRequestSchema,

	ListLpsPasswordsRequestSchema,
	RevealLpsPasswordRequestSchema,

	ListLuksKeysRequestSchema,
	RevealLuksKeyRequestSchema,
	CreateLuksTokenRequestSchema,
	RevokeLuksDeviceKeyRequestSchema,

	DispatchOSQueryRequestSchema,
	GetOSQueryResultRequestSchema,
	GetDeviceInventoryRequestSchema,
	RefreshDeviceInventoryRequestSchema,

	QueryDeviceLogsRequestSchema,
	GetDeviceLogResultRequestSchema,

	CreateRoleRequestSchema,
	GetRoleRequestSchema,
	ListRolesRequestSchema,
	UpdateRoleRequestSchema,
	DeleteRoleRequestSchema,
	AssignRoleToUserRequestSchema,
	RevokeRoleFromUserRequestSchema,
	ListPermissionsRequestSchema,

	CreateUserGroupRequestSchema,
	GetUserGroupRequestSchema,
	ListUserGroupsRequestSchema,
	UpdateUserGroupRequestSchema,
	DeleteUserGroupRequestSchema,
	AddUserToGroupRequestSchema,
	RemoveUserFromGroupRequestSchema,
	AssignRoleToUserGroupRequestSchema,
	RevokeRoleFromUserGroupRequestSchema,
	ListUserGroupsForUserRequestSchema,
	UpdateUserGroupQueryRequestSchema,
	ValidateUserGroupQueryRequestSchema,
	EvaluateDynamicUserGroupRequestSchema,

	ListAuthMethodsRequestSchema,
	GetSSOLoginURLRequestSchema,
	SSOCallbackRequestSchema,
	CreateIdentityProviderRequestSchema,
	GetIdentityProviderRequestSchema,
	ListIdentityProvidersRequestSchema,
	UpdateIdentityProviderRequestSchema,
	DeleteIdentityProviderRequestSchema,
	ListIdentityLinksRequestSchema,
	UnlinkIdentityRequestSchema,
	EnableSCIMRequestSchema,
	DisableSCIMRequestSchema,
	RotateSCIMTokenRequestSchema,

	SearchRequestSchema,
	SearchDateFilterSchema,
	RebuildSearchIndexRequestSchema,

	GetServerSettingsRequestSchema,
	UpdateServerSettingsRequestSchema,

	SetUserProvisioningEnabledRequestSchema,

	type SearchResult,

	GetDeviceComplianceRequestSchema,
	type GetDeviceComplianceResponse,

	CreateCompliancePolicyRequestSchema,
	GetCompliancePolicyRequestSchema,
	ListCompliancePoliciesRequestSchema,
	RenameCompliancePolicyRequestSchema,
	UpdateCompliancePolicyDescriptionRequestSchema,
	DeleteCompliancePolicyRequestSchema,
	AddCompliancePolicyRuleRequestSchema,
	RemoveCompliancePolicyRuleRequestSchema,
	UpdateCompliancePolicyRuleRequestSchema,
	GetDeviceCompliancePolicyStatusRequestSchema,
	type CompliancePolicy,
	type CompliancePolicyRule,
	type DevicePolicyEvaluation,
	type GetDeviceCompliancePolicyStatusResponse,
	type IdentityProvider,
	type IdentityLink,
	type InventoryTableResult,
	type User,
	type Device,
	type RegistrationToken,
	type ManagedAction,
	type ActionSet,
	type Definition,
	type DeviceGroup,
	type Assignment,
	type AuditEvent,
	type CreateActionRequest,
	type CreateActionSetRequest,
	type CreateDefinitionRequest,
	type UpdateActionParamsRequest,
	type Role,
	type PermissionInfo,
	type UserGroup,
	type UserGroupMember,
	type LpsPassword,
	type LuksKey,
	type AvailableItem,
	type DeviceAssignee,
	type DeviceGroupMember,
	type InheritedRole,
	type ActionSetMember,
	type DefinitionMember,
	StartTerminalRequestSchema,
	StopTerminalRequestSchema,
	ListActiveTerminalSessionsRequestSchema,
	TerminateTerminalSessionRequestSchema,
	type StartTerminalResponse,
	type TerminalSessionInfo
} from '../gen/ts/cadestro/v1/control_pb';
import type { ActionType, ActionSchedule } from '../gen/ts/cadestro/v1/actions_pb';
import {
	ErrorCode,
	ErrorDetailSchema,
	type MaintenanceWindow,
	AssignmentSourceType,
	AssignmentTargetType,
	DeviceStatus,
	IdentityProviderType,
	SearchScope,
	SortField,
	SortDirection,
	RoleGrantScopeKind
} from '../gen/ts/cadestro/v1/common_pb';
import { timestampDate } from '@bufbuild/protobuf/wkt';

export interface ClientOptions {
	getServerUrl: () => string;
	getAccessToken: () => string | null;
	getRefreshToken: () => string | null;
	ensureValidToken: () => Promise<void>;
	refreshToken: () => Promise<boolean>;
	onUnauthenticated: () => void;
	onAuthResponse: (accessToken: string, refreshToken: string, expiresAt: Date, user: User) => void;
	onUserUpdated: (user: User) => void;
}

export class ApiClient {
	private opts: ClientOptions;

	private cachedTransport: ReturnType<typeof createConnectTransport> | null = null;
	private cachedTransportUrl: string | null = null;

	constructor(opts: ClientOptions) {
		this.opts = opts;
	}

	private getTransport() {
		const serverUrl = this.opts.getServerUrl();
		if (!serverUrl) {
			throw new Error('Server URL not configured');
		}
		if (this.cachedTransport && this.cachedTransportUrl === serverUrl) {
			return this.cachedTransport;
		}

		const transport = createConnectTransport({
			baseUrl: serverUrl,
			interceptors: [
				(next) => async (req) => {
					await this.opts.ensureValidToken();

					const token = this.opts.getAccessToken();
					if (token) {
						req.header.set('Authorization', `Bearer ${token}`);
					}

					try {
						return await next(req);
					} catch (error: unknown) {
						if (error instanceof ConnectError && error.code === Code.Unauthenticated) {
							const refreshed = await this.opts.refreshToken();
							if (refreshed) {
								const newToken = this.opts.getAccessToken();
								if (newToken) {
									req.header.set('Authorization', `Bearer ${newToken}`);
								}
								return await next(req);
							}
							this.opts.onUnauthenticated();
						}
						throw error;
					}
				}
			]
		});
		this.cachedTransport = transport;
		this.cachedTransportUrl = serverUrl;
		return transport;
	}

	private cachedClient: ReturnType<typeof createClient<typeof ControlService>> | null = null;
	private cachedClientTransport: ReturnType<typeof createConnectTransport> | null = null;

	private getClient() {
		const transport = this.getTransport();
		if (this.cachedClient && this.cachedClientTransport === transport) {
			return this.cachedClient;
		}
		this.cachedClient = createClient(ControlService, transport);
		this.cachedClientTransport = transport;
		return this.cachedClient;
	}

	private cachedAuthTransport: ReturnType<typeof createConnectTransport> | null = null;
	private cachedAuthTransportUrl: string | null = null;

	private getAuthTransport() {
		const serverUrl = this.opts.getServerUrl();
		if (!serverUrl) {
			throw new Error('Server URL not configured');
		}
		if (this.cachedAuthTransport && this.cachedAuthTransportUrl === serverUrl) {
			return this.cachedAuthTransport;
		}
		const transport = createConnectTransport({ baseUrl: serverUrl });
		this.cachedAuthTransport = transport;
		this.cachedAuthTransportUrl = serverUrl;
		return transport;
	}

	private cachedAuthClient: ReturnType<typeof createClient<typeof ControlService>> | null = null;
	private cachedAuthClientTransport: ReturnType<typeof createConnectTransport> | null = null;

	private getAuthClient() {
		const transport = this.getAuthTransport();
		if (this.cachedAuthClient && this.cachedAuthClientTransport === transport) {
			return this.cachedAuthClient;
		}
		this.cachedAuthClient = createClient(ControlService, transport);
		this.cachedAuthClientTransport = transport;
		return this.cachedAuthClient;
	}

	async refreshTokenRPC() {
		const client = this.getAuthClient();
		return client.refreshToken(
			create(RefreshTokenRequestSchema, { refreshToken: this.opts.getRefreshToken() ?? '' })
		);
	}

	async logoutRPC() {
		const client = this.getAuthClient();
		return client.logout(
			create(LogoutRequestSchema, { refreshToken: this.opts.getRefreshToken() ?? '' })
		);
	}

	async getCurrentUser() {
		const client = this.getClient();
		const response = await client.getCurrentUser(
			create(GetCurrentUserRequestSchema, {})
		);
		if (response.user) {
			this.opts.onUserUpdated(response.user);
		}
		return response.user;
	}

	async eraseJITUser(id: string) {
		const client = this.getClient();
		await client.eraseJITUser(create(EraseJITUserRequestSchema, { id: { value: id } }));
	}

	async getUser(id: string) {
		const client = this.getClient();
		const response = await client.getUser(create(GetUserRequestSchema, { id: { value: id } }));
		return response.user;
	}

	async listUsers(pageSize: number = 50, pageToken: string = '') {
		const client = this.getClient();
		return client.listUsers(
			create(ListUsersRequestSchema, { pageSize, pageToken })
		);
	}

	async updateUserEmail(id: string, email: string) {
		const client = this.getClient();
		const response = await client.updateUserEmail(
			create(UpdateUserEmailRequestSchema, { id: { value: id }, email })
		);
		return response.user;
	}

	async setUserDisabled(id: string, disabled: boolean) {
		const client = this.getClient();
		const response = await client.setUserDisabled(
			create(SetUserDisabledRequestSchema, { id: { value: id }, disabled })
		);
		return response.user;
	}

	async updateUserProfile(id: string, profile: {
		displayName?: string;
		givenName?: string;
		familyName?: string;
		preferredUsername?: string;
		picture?: string;
		locale?: string;
	}) {
		const client = this.getClient();
		const response = await client.updateUserProfile(
			create(UpdateUserProfileRequestSchema, { id: { value: id }, ...profile })
		);
		return response.user;
	}

	async updateUserLinuxUsername(userId: string, linuxUsername: string) {
		const client = this.getClient();
		const response = await client.updateUserLinuxUsername(
			create(UpdateUserLinuxUsernameRequestSchema, { userId: { value: userId }, linuxUsername })
		);
		return response.user;
	}

	async addUserSshKey(userId: string, publicKey: string, comment: string = '') {
		const client = this.getClient();
		const response = await client.addUserSshKey(
			create(AddUserSshKeyRequestSchema, { userId: { value: userId }, publicKey, comment })
		);
		return response.key;
	}

	async removeUserSshKey(userId: string, keyId: string) {
		const client = this.getClient();
		await client.removeUserSshKey(
			create(RemoveUserSshKeyRequestSchema, { userId: { value: userId }, keyId: { value: keyId } })
		);
	}

	async updateUserSshSettings(userId: string, settings: {
		sshAccessEnabled: boolean;
		sshAllowPubkey: boolean;
		sshAllowPassword: boolean;
	}) {
		const client = this.getClient();
		const response = await client.updateUserSshSettings(
			create(UpdateUserSshSettingsRequestSchema, { userId: { value: userId }, ...settings })
		);
		return response.user;
	}

	async listDevices(
		pageSize: number = 50,
		pageToken: string = '',
		statusFilter: DeviceStatus = DeviceStatus.UNSPECIFIED,
		labelFilter: Record<string, string> = {},
		myDevicesOnly: boolean = false
	) {
		const client = this.getClient();
		return client.listDevices(
			create(ListDevicesRequestSchema, { pageSize, pageToken, statusFilter, labelFilter, myDevicesOnly })
		);
	}

	async getDevice(id: string) {
		const client = this.getClient();
		const response = await client.getDevice(create(GetDeviceRequestSchema, { id: { value: id } }));
		return response.device;
	}

	async setDeviceLabel(id: string, key: string, value: string) {
		const client = this.getClient();
		const response = await client.setDeviceLabel(
			create(SetDeviceLabelRequestSchema, { id: { value: id }, key, value })
		);
		return response.device;
	}

	async removeDeviceLabel(id: string, key: string) {
		const client = this.getClient();
		const response = await client.removeDeviceLabel(
			create(RemoveDeviceLabelRequestSchema, { id: { value: id }, key })
		);
		return response.device;
	}

	async assignDevice(deviceId: string, userIds: string[], groupIds: string[]) {
		const client = this.getClient();
		const response = await client.assignDevice(
			create(AssignDeviceRequestSchema, { deviceId: { value: deviceId }, userIds: userIds.map((value) => ({ value })), groupIds: groupIds.map((value) => ({ value })) })
		);
		return response.device;
	}

	async unassignDevice(deviceId: string, userId?: string, groupId?: string) {
		const client = this.getClient();
		const response = await client.unassignDevice(
			create(UnassignDeviceRequestSchema, { deviceId: { value: deviceId }, userId: userId ? { value: userId } : undefined, groupId: groupId ? { value: groupId } : undefined })
		);
		return response.device;
	}

	async listDeviceAssignees(deviceId: string): Promise<DeviceAssignee[]> {
		const client = this.getClient();
		const response = await client.listDeviceAssignees(
			create(ListDeviceAssigneesRequestSchema, { deviceId: { value: deviceId } })
		);
		return [...response.assignees];
	}

	async deleteDevice(id: string) {
		const client = this.getClient();
		await client.deleteDevice(create(DeleteDeviceRequestSchema, { id: { value: id } }));
	}

	async setDeviceSyncInterval(id: string, syncIntervalMinutes: number) {
		const client = this.getClient();
		const response = await client.setDeviceSyncInterval(
			create(SetDeviceSyncIntervalRequestSchema, { id: { value: id }, syncIntervalMinutes })
		);
		return response.device;
	}

	async setDeviceInventoryInterval(id: string, inventoryIntervalMinutes: number) {
		const client = this.getClient();
		const response = await client.setDeviceInventoryInterval(
			create(SetDeviceInventoryIntervalRequestSchema, { id: { value: id }, inventoryIntervalMinutes })
		);
		return response.device;
	}

	async createToken(
		name: string,
		maxUses: number = 0,
		expiresAt?: Date
	) {
		const client = this.getClient();
		const response = await client.createToken(
			create(CreateTokenRequestSchema, {
				name,
				maxUses,
				expiresAt: expiresAt
					? { seconds: BigInt(Math.floor(expiresAt.getTime() / 1000)), nanos: 0 }
					: undefined
			})
		);

		return response;
	}

	async listTokens(pageSize: number = 50, pageToken: string = '', includeDisabled: boolean = false) {
		const client = this.getClient();
		return client.listTokens(
			create(ListTokensRequestSchema, { pageSize, pageToken, includeDisabled })
		);
	}

	async renameToken(id: string, name: string) {
		const client = this.getClient();
		const response = await client.renameToken(
			create(RenameTokenRequestSchema, { id: { value: id }, name })
		);
		return response.token;
	}

	async setTokenDisabled(id: string, disabled: boolean) {
		const client = this.getClient();
		const response = await client.setTokenDisabled(
			create(SetTokenDisabledRequestSchema, { id: { value: id }, disabled })
		);
		return response.token;
	}

	async deleteToken(id: string) {
		const client = this.getClient();
		await client.deleteToken(create(DeleteTokenRequestSchema, { id: { value: id } }));
	}

	async createAction(data: Omit<CreateActionRequest, '$typeName'>) {
		const client = this.getClient();
		const response = await client.createAction(
			create(CreateActionRequestSchema, data)
		);
		return response.action;
	}

	async getAction(id: string) {
		const client = this.getClient();
		const response = await client.getAction(create(GetActionRequestSchema, { id: { value: id } }));
		return response.action;
	}

	async listActions(pageSize: number = 50, pageToken: string = '', typeFilter?: ActionType, unassignedOnly: boolean = false) {
		const client = this.getClient();
		return client.listActions(
			create(ListActionsRequestSchema, { pageSize, pageToken, typeFilter: typeFilter ?? 0, unassignedOnly })
		);
	}

	async renameAction(id: string, name: string) {
		const client = this.getClient();
		const response = await client.renameAction(
			create(RenameActionRequestSchema, { id: { value: id }, name })
		);
		return response.action;
	}

	async updateActionDescription(id: string, description: string) {
		const client = this.getClient();
		const response = await client.updateActionDescription(
			create(UpdateActionDescriptionRequestSchema, { id: { value: id }, description })
		);
		return response.action;
	}

	async updateActionParams(data: Omit<UpdateActionParamsRequest, '$typeName'>) {
		const client = this.getClient();
		const response = await client.updateActionParams(
			create(UpdateActionParamsRequestSchema, data)
		);
		return response.action;
	}

	async deleteAction(id: string) {
		const client = this.getClient();
		await client.deleteAction(create(DeleteActionRequestSchema, { id: { value: id } }));
	}

	async createActionSet(data: Omit<CreateActionSetRequest, '$typeName'>) {
		const client = this.getClient();
		const response = await client.createActionSet(
			create(CreateActionSetRequestSchema, data)
		);
		return response.set;
	}

	async getActionSet(id: string) {
		const client = this.getClient();
		return client.getActionSet(create(GetActionSetRequestSchema, { id: { value: id } }));
	}

	async listActionSets(pageSize: number = 50, pageToken: string = '', unassignedOnly: boolean = false) {
		const client = this.getClient();
		return client.listActionSets(
			create(ListActionSetsRequestSchema, { pageSize, pageToken, unassignedOnly })
		);
	}

	async renameActionSet(id: string, name: string) {
		const client = this.getClient();
		const response = await client.renameActionSet(
			create(RenameActionSetRequestSchema, { id: { value: id }, name })
		);
		return response.set;
	}

	async updateActionSetDescription(id: string, description: string) {
		const client = this.getClient();
		const response = await client.updateActionSetDescription(
			create(UpdateActionSetDescriptionRequestSchema, { id: { value: id }, description })
		);
		return response.set;
	}

	async updateActionSetSchedule(id: string, schedule: ActionSchedule) {
		const client = this.getClient();
		const response = await client.updateActionSetSchedule(
			create(UpdateActionSetScheduleRequestSchema, { id: { value: id }, schedule })
		);
		return response.set;
	}

	async deleteActionSet(id: string) {
		const client = this.getClient();
		await client.deleteActionSet(create(DeleteActionSetRequestSchema, { id: { value: id } }));
	}

	async addActionToSet(setId: string, actionId: string, sortOrder: number = 0) {
		const client = this.getClient();
		const response = await client.addActionToSet(
			create(AddActionToSetRequestSchema, { setId: { value: setId }, actionId: { value: actionId }, sortOrder })
		);
		return response.set;
	}

	async removeActionFromSet(setId: string, actionId: string) {
		const client = this.getClient();
		const response = await client.removeActionFromSet(
			create(RemoveActionFromSetRequestSchema, { setId: { value: setId }, actionId: { value: actionId } })
		);
		return response.set;
	}

	async reorderActionInSet(setId: string, actionId: string, newOrder: number) {
		const client = this.getClient();
		const response = await client.reorderActionInSet(
			create(ReorderActionInSetRequestSchema, { setId: { value: setId }, actionId: { value: actionId }, newOrder })
		);
		return response.set;
	}

	async createDefinition(data: Omit<CreateDefinitionRequest, '$typeName'>) {
		const client = this.getClient();
		const response = await client.createDefinition(
			create(CreateDefinitionRequestSchema, data)
		);
		return response.definition;
	}

	async getDefinition(id: string) {
		const client = this.getClient();
		return client.getDefinition(create(GetDefinitionRequestSchema, { id: { value: id } }));
	}

	async listDefinitions(pageSize: number = 50, pageToken: string = '') {
		const client = this.getClient();
		return client.listDefinitions(
			create(ListDefinitionsRequestSchema, { pageSize, pageToken })
		);
	}

	async renameDefinition(id: string, name: string) {
		const client = this.getClient();
		const response = await client.renameDefinition(
			create(RenameDefinitionRequestSchema, { id: { value: id }, name })
		);
		return response.definition;
	}

	async updateDefinitionDescription(id: string, description: string) {
		const client = this.getClient();
		const response = await client.updateDefinitionDescription(
			create(UpdateDefinitionDescriptionRequestSchema, { id: { value: id }, description })
		);
		return response.definition;
	}

	async updateDefinitionSchedule(id: string, schedule: ActionSchedule) {
		const client = this.getClient();
		const response = await client.updateDefinitionSchedule(
			create(UpdateDefinitionScheduleRequestSchema, { id: { value: id }, schedule })
		);
		return response.definition;
	}

	async deleteDefinition(id: string) {
		const client = this.getClient();
		await client.deleteDefinition(create(DeleteDefinitionRequestSchema, { id: { value: id } }));
	}

	async addActionSetToDefinition(definitionId: string, actionSetId: string, sortOrder: number = 0) {
		const client = this.getClient();
		const response = await client.addActionSetToDefinition(
			create(AddActionSetToDefinitionRequestSchema, { definitionId: { value: definitionId }, actionSetId: { value: actionSetId }, sortOrder })
		);
		return response.definition;
	}

	async removeActionSetFromDefinition(definitionId: string, actionSetId: string) {
		const client = this.getClient();
		const response = await client.removeActionSetFromDefinition(
			create(RemoveActionSetFromDefinitionRequestSchema, { definitionId: { value: definitionId }, actionSetId: { value: actionSetId } })
		);
		return response.definition;
	}

	async reorderActionSetInDefinition(definitionId: string, actionSetId: string, newOrder: number) {
		const client = this.getClient();
		const response = await client.reorderActionSetInDefinition(
			create(ReorderActionSetInDefinitionRequestSchema, { definitionId: { value: definitionId }, actionSetId: { value: actionSetId }, newOrder })
		);
		return response.definition;
	}

	async createDeviceGroup(name: string, description: string = '', isDynamic: boolean = false, dynamicQuery: string = '') {
		const client = this.getClient();
		const response = await client.createDeviceGroup(
			create(CreateDeviceGroupRequestSchema, { name, description, isDynamic, dynamicQuery })
		);
		return response.group;
	}

	async getDeviceGroup(id: string) {
		const client = this.getClient();
		return client.getDeviceGroup(create(GetDeviceGroupRequestSchema, { id: { value: id } }));
	}

	async listDeviceGroups(pageSize: number = 50, pageToken: string = '') {
		const client = this.getClient();
		return client.listDeviceGroups(
			create(ListDeviceGroupsRequestSchema, { pageSize, pageToken })
		);
	}

	async listDeviceGroupsForDevice(deviceId: string) {
		const client = this.getClient();
		return client.listDeviceGroupsForDevice(
			create(ListDeviceGroupsForDeviceRequestSchema, { deviceId: { value: deviceId } })
		);
	}

	async renameDeviceGroup(id: string, name: string) {
		const client = this.getClient();
		const response = await client.renameDeviceGroup(
			create(RenameDeviceGroupRequestSchema, { id: { value: id }, name })
		);
		return response.group;
	}

	async updateDeviceGroupDescription(id: string, description: string) {
		const client = this.getClient();
		const response = await client.updateDeviceGroupDescription(
			create(UpdateDeviceGroupDescriptionRequestSchema, { id: { value: id }, description })
		);
		return response.group;
	}

	async deleteDeviceGroup(id: string) {
		const client = this.getClient();
		await client.deleteDeviceGroup(create(DeleteDeviceGroupRequestSchema, { id: { value: id } }));
	}

	async addDeviceToGroup(groupId: string, deviceIds: string[]) {
		const client = this.getClient();
		const response = await client.addDeviceToGroup(
			create(AddDeviceToGroupRequestSchema, { groupId: { value: groupId }, deviceIds: deviceIds.map((value) => ({ value })) })
		);
		return response.group;
	}

	async removeDeviceFromGroup(groupId: string, deviceId: string) {
		const client = this.getClient();
		const response = await client.removeDeviceFromGroup(
			create(RemoveDeviceFromGroupRequestSchema, { groupId: { value: groupId }, deviceId: { value: deviceId } })
		);
		return response.group;
	}

	async updateDeviceGroupQuery(id: string, isDynamic: boolean, dynamicQuery: string = '') {
		const client = this.getClient();
		const response = await client.updateDeviceGroupQuery(
			create(UpdateDeviceGroupQueryRequestSchema, { id: { value: id }, isDynamic, dynamicQuery })
		);
		return response.group;
	}

	async validateDynamicQuery(query: string) {
		const client = this.getClient();
		return client.validateDynamicQuery(
			create(ValidateDynamicQueryRequestSchema, { query })
		);
	}

	async evaluateDynamicGroup(id: string) {
		const client = this.getClient();
		return client.evaluateDynamicGroup(
			create(EvaluateDynamicGroupRequestSchema, { id: { value: id } })
		);
	}

	async setDeviceGroupSyncInterval(id: string, syncIntervalMinutes: number) {
		const client = this.getClient();
		const response = await client.setDeviceGroupSyncInterval(
			create(SetDeviceGroupSyncIntervalRequestSchema, { id: { value: id }, syncIntervalMinutes })
		);
		return response.group;
	}

	async setDeviceGroupInventoryInterval(id: string, inventoryIntervalMinutes: number) {
		const client = this.getClient();
		const response = await client.setDeviceGroupInventoryInterval(
			create(SetDeviceGroupInventoryIntervalRequestSchema, { id: { value: id }, inventoryIntervalMinutes })
		);
		return response.group;
	}

	async setDeviceGroupMaintenanceWindow(
		id: string,
		maintenanceWindow: MaintenanceWindow | undefined
	) {
		const client = this.getClient();
		const response = await client.setDeviceGroupMaintenanceWindow(
			create(SetDeviceGroupMaintenanceWindowRequestSchema, { id: { value: id }, maintenanceWindow })
		);
		return response.group;
	}

	async createAssignment(
		sourceType: AssignmentSourceType,
		sourceId: string,
		targetType: AssignmentTargetType,
		targetId: string,
		mode: number = 0
	) {
		const client = this.getClient();
		const response = await client.createAssignment(
			create(CreateAssignmentRequestSchema, { sourceType, sourceId: { value: sourceId }, targetType, targetId: { value: targetId }, mode })
		);
		return response.assignment;
	}

	async batchCreateAssignments(
		sourceType: AssignmentSourceType,
		sourceId: string,
		targets: Array<{ targetType: AssignmentTargetType; targetId: string }>,
		mode: number = 0
	) {
		return Promise.all(
			targets.map((t) =>
				this.createAssignment(sourceType, sourceId, t.targetType, t.targetId, mode)
			)
		);
	}

	async deleteAssignment(id: string) {
		const client = this.getClient();
		await client.deleteAssignment(create(DeleteAssignmentRequestSchema, { id: { value: id } }));
	}

	async listAssignments(
		pageSize: number = 50,
		pageToken: string = '',
		sourceType: AssignmentSourceType = AssignmentSourceType.UNSPECIFIED,
		sourceId: string = '',
		targetType: AssignmentTargetType = AssignmentTargetType.UNSPECIFIED,
		targetId: string = ''
	) {
		const client = this.getClient();
		return client.listAssignments(
			create(ListAssignmentsRequestSchema, { pageSize, pageToken, sourceType, sourceId: { value: sourceId }, targetType, targetId: { value: targetId } })
		);
	}

	async getDeviceAssignments(deviceId: string) {
		const client = this.getClient();
		return client.getDeviceAssignments(
			create(GetDeviceAssignmentsRequestSchema, { deviceId: { value: deviceId } })
		);
	}

	async getUserAssignments(userId: string) {
		const client = this.getClient();
		return client.getUserAssignments(
			create(GetUserAssignmentsRequestSchema, { userId: { value: userId } })
		);
	}

	async listAvailableActions(deviceId: string) {
		const client = this.getClient();
		const response = await client.listAvailableActions(
			create(ListAvailableActionsRequestSchema, { deviceId: { value: deviceId } })
		);
		return response.items;
	}

	async setUserSelection(deviceId: string, sourceType: AssignmentSourceType, sourceId: string, selected: boolean) {
		const client = this.getClient();
		return client.setUserSelection(
			create(SetUserSelectionRequestSchema, { deviceId: { value: deviceId }, sourceType, sourceId: { value: sourceId }, selected })
		);
	}

	async rebootDevice(deviceId: string) {
		const client = this.getClient();
		await client.rebootDevice(create(RebootDeviceRequestSchema, { deviceId: { value: deviceId } }));
	}

	async syncDevice(deviceId: string) {
		const client = this.getClient();
		await client.syncDevice(create(SyncDeviceRequestSchema, { deviceId: { value: deviceId } }));
	}

	async listLpsPasswords(deviceId: string) {
		const client = this.getClient();
		return client.listLpsPasswords(
			create(ListLpsPasswordsRequestSchema, { deviceId: { value: deviceId } })
		);
	}

	async revealLpsPassword(id: string) {
		const client = this.getClient();
		return client.revealLpsPassword(
			create(RevealLpsPasswordRequestSchema, { id: { value: id } })
		);
	}

	async listLuksKeys(deviceId: string) {
		const client = this.getClient();
		return client.listLuksKeys(
			create(ListLuksKeysRequestSchema, { deviceId: { value: deviceId } })
		);
	}

	async revealLuksKey(id: string) {
		const client = this.getClient();
		return client.revealLuksKey(
			create(RevealLuksKeyRequestSchema, { id: { value: id } })
		);
	}

	async createLuksToken(deviceId: string, actionId: string) {
		const client = this.getClient();
		return client.createLuksToken(
			create(CreateLuksTokenRequestSchema, { deviceId: { value: deviceId }, actionId: { value: actionId } })
		);
	}

	async revokeLuksDeviceKey(deviceId: string, actionId: string) {
		const client = this.getClient();
		return client.revokeLuksDeviceKey(
			create(RevokeLuksDeviceKeyRequestSchema, { deviceId: { value: deviceId }, actionId: { value: actionId } })
		);
	}

	async getDeviceCompliance(deviceId: string): Promise<GetDeviceComplianceResponse> {
		const client = this.getClient();
		return client.getDeviceCompliance(
			create(GetDeviceComplianceRequestSchema, { deviceId: { value: deviceId } })
		);
	}

	async createCompliancePolicy(name: string, description: string = '') {
		const client = this.getClient();
		const response = await client.createCompliancePolicy(
			create(CreateCompliancePolicyRequestSchema, { name, description })
		);
		return response.policy;
	}

	async getCompliancePolicy(id: string) {
		const client = this.getClient();
		const response = await client.getCompliancePolicy(
			create(GetCompliancePolicyRequestSchema, { id: { value: id } })
		);
		return response.policy;
	}

	async listCompliancePolicies(pageSize: number = 50, pageToken: string = '') {
		const client = this.getClient();
		return client.listCompliancePolicies(
			create(ListCompliancePoliciesRequestSchema, { pageSize, pageToken })
		);
	}

	async renameCompliancePolicy(id: string, name: string) {
		const client = this.getClient();
		const response = await client.renameCompliancePolicy(
			create(RenameCompliancePolicyRequestSchema, { id: { value: id }, name })
		);
		return response.policy;
	}

	async updateCompliancePolicyDescription(id: string, description: string) {
		const client = this.getClient();
		const response = await client.updateCompliancePolicyDescription(
			create(UpdateCompliancePolicyDescriptionRequestSchema, { id: { value: id }, description })
		);
		return response.policy;
	}

	async deleteCompliancePolicy(id: string) {
		const client = this.getClient();
		await client.deleteCompliancePolicy(
			create(DeleteCompliancePolicyRequestSchema, { id: { value: id } })
		);
	}

	async addCompliancePolicyRule(policyId: string, actionId: string, gracePeriodHours: number = 0) {
		const client = this.getClient();
		const response = await client.addCompliancePolicyRule(
			create(AddCompliancePolicyRuleRequestSchema, { policyId: { value: policyId }, actionId: { value: actionId }, gracePeriodHours })
		);
		return response.policy;
	}

	async removeCompliancePolicyRule(policyId: string, actionId: string) {
		const client = this.getClient();
		const response = await client.removeCompliancePolicyRule(
			create(RemoveCompliancePolicyRuleRequestSchema, { policyId: { value: policyId }, actionId: { value: actionId } })
		);
		return response.policy;
	}

	async updateCompliancePolicyRule(policyId: string, actionId: string, gracePeriodHours: number) {
		const client = this.getClient();
		const response = await client.updateCompliancePolicyRule(
			create(UpdateCompliancePolicyRuleRequestSchema, { policyId: { value: policyId }, actionId: { value: actionId }, gracePeriodHours })
		);
		return response.policy;
	}

	async getDeviceCompliancePolicyStatus(deviceId: string): Promise<GetDeviceCompliancePolicyStatusResponse> {
		const client = this.getClient();
		return client.getDeviceCompliancePolicyStatus(
			create(GetDeviceCompliancePolicyStatusRequestSchema, { deviceId: { value: deviceId } })
		);
	}

	async getDeviceInventory(deviceId: string, tableNames?: string[]) {
		const client = this.getClient();
		return client.getDeviceInventory(
			create(GetDeviceInventoryRequestSchema, { deviceId: { value: deviceId }, tableNames: tableNames ?? [] })
		);
	}

	async refreshDeviceInventory(deviceId: string) {
		const client = this.getClient();
		return client.refreshDeviceInventory(
			create(RefreshDeviceInventoryRequestSchema, { deviceId: { value: deviceId } })
		);
	}

	async dispatchOSQuery(deviceId: string, table: string, columns?: string[], limit?: number, rawSql?: string) {
		const client = this.getClient();
		const response = await client.dispatchOSQuery(
			create(DispatchOSQueryRequestSchema, {
				deviceId: { value: deviceId }, table, columns: columns ?? [], limit: limit ?? 0, rawSql: rawSql ?? ''
			})
		);
		return response.queryId?.value ?? '';
	}

	async getOSQueryResult(queryId: string) {
		const client = this.getClient();
		return client.getOSQueryResult(
			create(GetOSQueryResultRequestSchema, { queryId: { value: queryId } })
		);
	}

	async queryDeviceLogs(deviceId: string, options?: {
		lines?: number, unit?: string, since?: string, until?: string,
		priority?: string, grep?: string, kernel?: boolean
	}) {
		const client = this.getClient();
		const response = await client.queryDeviceLogs(
			create(QueryDeviceLogsRequestSchema, {
				deviceId: { value: deviceId },
				lines: options?.lines ?? 0,
				unit: options?.unit ?? '',
				since: options?.since ?? '',
				until: options?.until ?? '',
				priority: options?.priority ?? '',
				grep: options?.grep ?? '',
				kernel: options?.kernel ?? false
			})
		);
		return response.queryId?.value ?? '';
	}

	async getDeviceLogResult(queryId: string) {
		const client = this.getClient();
		return client.getDeviceLogResult(
			create(GetDeviceLogResultRequestSchema, { queryId: { value: queryId } })
		);
	}

	async listAuditEvents(
		pageSize: number = 50,
		pageToken: string = '',
		actorId: string = '',
		streamType: string = '',
		eventType: string = ''
	) {
		const client = this.getClient();
		return client.listAuditEvents(
			create(ListAuditEventsRequestSchema, { pageSize, pageToken, actorId: { value: actorId }, streamType, eventType })
		);
	}

	async exportAuditEvents(options: {
		format: 'csv' | 'json';
		actorId?: string;
		streamTypes?: string[];
		eventType?: string;
		occurredFrom?: Timestamp;
		occurredTo?: Timestamp;
		pageToken?: string;
	}) {
		const client = this.getClient();
		return client.exportAuditEvents(create(ExportAuditEventsRequestSchema, {
			...options,
			actorId: options.actorId ? { value: options.actorId } : undefined,
		}));
	}

	async createRole(name: string, description: string, permissions: string[]) {
		const client = this.getClient();
		const response = await client.createRole(
			create(CreateRoleRequestSchema, { name, description, permissions })
		);
		return response.role;
	}

	async getRole(id: string) {
		const client = this.getClient();
		return client.getRole(create(GetRoleRequestSchema, { id: { value: id } }));
	}

	async listRoles(pageSize: number = 50, pageToken: string = '') {
		const client = this.getClient();
		return client.listRoles(
			create(ListRolesRequestSchema, { pageSize, pageToken })
		);
	}

	async updateRole(roleId: string, name: string, description: string, permissions: string[]) {
		const client = this.getClient();
		const response = await client.updateRole(
			create(UpdateRoleRequestSchema, { roleId: { value: roleId }, name, description, permissions })
		);
		return response.role;
	}

	async deleteRole(id: string) {
		const client = this.getClient();
		await client.deleteRole(create(DeleteRoleRequestSchema, { id: { value: id } }));
	}

	async assignRoleToUser(
		userId: string,
		roleIds: string[],
		scopeKind: RoleGrantScopeKind = RoleGrantScopeKind.UNSPECIFIED,
		scopeId: string = ''
	) {
		const client = this.getClient();
		await client.assignRoleToUser(
			create(AssignRoleToUserRequestSchema, { userId: { value: userId }, roleIds: roleIds.map((value) => ({ value })), scopeKind, scopeId: scopeId ? { value: scopeId } : undefined })
		);
	}

	async revokeRoleFromUser(
		userId: string,
		roleId: string,
		scopeKind: RoleGrantScopeKind = RoleGrantScopeKind.UNSPECIFIED,
		scopeId: string = ''
	) {
		const client = this.getClient();
		await client.revokeRoleFromUser(
			create(RevokeRoleFromUserRequestSchema, { userId: { value: userId }, roleId: { value: roleId }, scopeKind, scopeId: scopeId ? { value: scopeId } : undefined })
		);
	}

	async listPermissions() {
		const client = this.getClient();
		return client.listPermissions(
			create(ListPermissionsRequestSchema, {})
		);
	}

	async createUserGroup(name: string, description: string = '', isDynamic: boolean = false, dynamicQuery: string = '') {
		const client = this.getClient();
		const response = await client.createUserGroup(
			create(CreateUserGroupRequestSchema, { name, description, isDynamic, dynamicQuery })
		);
		return response.group;
	}

	async getUserGroup(id: string) {
		const client = this.getClient();
		return client.getUserGroup(create(GetUserGroupRequestSchema, { id: { value: id } }));
	}

	async listUserGroups(pageSize: number = 50, pageToken: string = '') {
		const client = this.getClient();
		return client.listUserGroups(
			create(ListUserGroupsRequestSchema, { pageSize, pageToken })
		);
	}

	async updateUserGroup(id: string, name: string, description: string) {
		const client = this.getClient();
		const response = await client.updateUserGroup(
			create(UpdateUserGroupRequestSchema, { groupId: { value: id }, name, description })
		);
		return response.group;
	}

	async deleteUserGroup(id: string) {
		const client = this.getClient();
		await client.deleteUserGroup(create(DeleteUserGroupRequestSchema, { id: { value: id } }));
	}

	async addUserToGroup(groupId: string, userIds: string[]) {
		const client = this.getClient();
		await client.addUserToGroup(
			create(AddUserToGroupRequestSchema, { groupId: { value: groupId }, userIds: userIds.map((value) => ({ value })) })
		);
	}

	async removeUserFromGroup(groupId: string, userId: string) {
		const client = this.getClient();
		await client.removeUserFromGroup(
			create(RemoveUserFromGroupRequestSchema, { groupId: { value: groupId }, userId: { value: userId } })
		);
	}

	async assignRoleToUserGroup(
		groupId: string,
		roleIds: string[],
		scopeKind: RoleGrantScopeKind = RoleGrantScopeKind.UNSPECIFIED,
		scopeId: string = ''
	) {
		const client = this.getClient();
		await client.assignRoleToUserGroup(
			create(AssignRoleToUserGroupRequestSchema, { groupId: { value: groupId }, roleIds: roleIds.map((value) => ({ value })), scopeKind, scopeId: scopeId ? { value: scopeId } : undefined })
		);
	}

	async revokeRoleFromUserGroup(
		groupId: string,
		roleId: string,
		scopeKind: RoleGrantScopeKind = RoleGrantScopeKind.UNSPECIFIED,
		scopeId: string = ''
	) {
		const client = this.getClient();
		await client.revokeRoleFromUserGroup(
			create(RevokeRoleFromUserGroupRequestSchema, { groupId: { value: groupId }, roleId: { value: roleId }, scopeKind, scopeId: scopeId ? { value: scopeId } : undefined })
		);
	}

	async listUserGroupsForUser(userId: string) {
		const client = this.getClient();
		return client.listUserGroupsForUser(
			create(ListUserGroupsForUserRequestSchema, { userId: { value: userId } })
		);
	}

	async updateUserGroupQuery(id: string, isDynamic: boolean, dynamicQuery: string) {
		const client = this.getClient();
		const response = await client.updateUserGroupQuery(
			create(UpdateUserGroupQueryRequestSchema, { id: { value: id }, isDynamic, dynamicQuery })
		);
		return response.group;
	}

	async validateUserGroupQuery(query: string) {
		const client = this.getClient();
		return client.validateUserGroupQuery(
			create(ValidateUserGroupQueryRequestSchema, { query })
		);
	}

	async evaluateDynamicUserGroup(id: string) {
		const client = this.getClient();
		return client.evaluateDynamicUserGroup(
			create(EvaluateDynamicUserGroupRequestSchema, { id: { value: id } })
		);
	}

	async setUserGroupMaintenanceWindow(
		id: string,
		maintenanceWindow: MaintenanceWindow | undefined
	) {
		const client = this.getClient();
		const response = await client.setUserGroupMaintenanceWindow(
			create(SetUserGroupMaintenanceWindowRequestSchema, { id: { value: id }, maintenanceWindow })
		);
		return response.group;
	}

	async listAuthMethods(email: string = '') {
		const client = this.getAuthClient();
		return client.listAuthMethods(
			create(ListAuthMethodsRequestSchema, { email })
		);
	}

	async getSSOLoginURL(slug: string, redirectUrl: string) {
		const client = this.getAuthClient();
		return client.getSSOLoginURL(
			create(GetSSOLoginURLRequestSchema, { slug, redirectUrl })
		);
	}

	async ssoCallback(slug: string, code: string, state: string) {
		const client = this.getAuthClient();
		const response = await client.sSOCallback(
			create(SSOCallbackRequestSchema, { slug, code, state })
		);

		if (response.accessToken && response.refreshToken && response.expiresAt && response.user) {
			this.opts.onAuthResponse(
				response.accessToken,
				response.refreshToken,
				timestampDate(response.expiresAt),
				response.user
			);
		}

		return response;
	}

	async createIdentityProvider(data: {
		name: string;
		slug: string;
		providerType: IdentityProviderType;
		clientId: string;
		clientSecret: string;
		issuerUrl: string;
		authorizationUrl?: string;
		tokenUrl?: string;
		userinfoUrl?: string;
		scopes?: string[];
		autoCreateUsers?: boolean;
		autoLinkByEmail?: boolean;
		trustEmailAssertions?: boolean;
		defaultRoleId?: string;
		groupClaim?: string;
		groupMapping?: Record<string, string>;
	}) {
		const client = this.getClient();
		const response = await client.createIdentityProvider(
			create(CreateIdentityProviderRequestSchema, {
				...data,
				clientId: { value: data.clientId },
				defaultRoleId: data.defaultRoleId ? { value: data.defaultRoleId } : undefined,
			})
		);
		return response.provider;
	}

	async createIdentityProviderWithBootstrapToken(
		bootstrapToken: string,
		data: {
			name: string;
			slug: string;
			providerType: IdentityProviderType;
			clientId: string;
			clientSecret: string;
			issuerUrl: string;
			authorizationUrl?: string;
			tokenUrl?: string;
			userinfoUrl?: string;
			scopes?: string[];
			autoCreateUsers?: boolean;
			autoLinkByEmail?: boolean;
			trustEmailAssertions?: boolean;
			defaultRoleId?: string;
			groupClaim?: string;
			groupMapping?: Record<string, string>;
		}
	) {
		const client = this.getAuthClient();
		const response = await client.createIdentityProvider(
			create(CreateIdentityProviderRequestSchema, {
				...data,
				clientId: { value: data.clientId },
				defaultRoleId: data.defaultRoleId ? { value: data.defaultRoleId } : undefined,
			}),
			{ headers: { Authorization: `Cadestro-Bootstrap ${bootstrapToken}` } }
		);
		return response.provider;
	}

	async getIdentityProvider(id: string) {
		const client = this.getClient();
		const response = await client.getIdentityProvider(
			create(GetIdentityProviderRequestSchema, { id: { value: id } })
		);
		return response.provider;
	}

	async listIdentityProviders(pageSize: number = 50, pageToken: string = '') {
		const client = this.getClient();
		return client.listIdentityProviders(
			create(ListIdentityProvidersRequestSchema, { pageSize, pageToken })
		);
	}

	async updateIdentityProvider(data: {
		id: string;
		name?: string;
		enabled?: boolean;
		clientId?: string;
		clientSecret?: string;
		issuerUrl?: string;
		authorizationUrl?: string;
		tokenUrl?: string;
		userinfoUrl?: string;
		scopes?: string[];
		autoCreateUsers?: boolean;
		autoLinkByEmail?: boolean;
		trustEmailAssertions?: boolean;
		defaultRoleId?: string;
		groupClaim?: string;
		groupMapping?: Record<string, string>;
	}) {
		const client = this.getClient();
		const response = await client.updateIdentityProvider(
			create(UpdateIdentityProviderRequestSchema, {
				...data,
				id: { value: data.id },
				clientId: data.clientId ? { value: data.clientId } : undefined,
				defaultRoleId: data.defaultRoleId ? { value: data.defaultRoleId } : undefined,
			})
		);
		return response.provider;
	}

	async deleteIdentityProvider(id: string) {
		const client = this.getClient();
		await client.deleteIdentityProvider(
			create(DeleteIdentityProviderRequestSchema, { id: { value: id } })
		);
	}

	async listIdentityLinks() {
		const client = this.getClient();
		return client.listIdentityLinks(
			create(ListIdentityLinksRequestSchema, {})
		);
	}

	async unlinkIdentity(linkId: string) {
		const client = this.getClient();
		await client.unlinkIdentity(
			create(UnlinkIdentityRequestSchema, { linkId: { value: linkId } })
		);
	}

	async enableSCIM(id: string) {
		const client = this.getClient();
		return client.enableSCIM(create(EnableSCIMRequestSchema, { id: { value: id } }));
	}

	async disableSCIM(id: string) {
		const client = this.getClient();
		await client.disableSCIM(create(DisableSCIMRequestSchema, { id: { value: id } }));
	}

	async rotateSCIMToken(id: string) {
		const client = this.getClient();
		return client.rotateSCIMToken(create(RotateSCIMTokenRequestSchema, { id: { value: id } }));
	}

	async search(
		query: string,
		scope: SearchScope = SearchScope.UNSPECIFIED,
		pageSize: number = 50,
		pageToken: string = '',
		dateFilters?: Array<{ field: string; start: bigint; end: bigint }>,
		tagFilters?: Record<string, string>,
		sortField: SortField = SortField.UNSPECIFIED,
		sortDirection: SortDirection = SortDirection.UNSPECIFIED
	) {
		const client = this.getClient();
		const req: Record<string, unknown> = { query, scope, pageSize, pageToken, sortField, sortDirection };
		if (dateFilters && dateFilters.length > 0) {
			req.dateFilters = dateFilters.map((df) =>
				create(SearchDateFilterSchema, { field: df.field, start: df.start, end: df.end })
			);
		}
		if (tagFilters) {
			req.tagFilters = tagFilters;
		}
		return client.search(create(SearchRequestSchema, req));
	}

	async rebuildSearchIndex() {
		const client = this.getClient();
		await client.rebuildSearchIndex(create(RebuildSearchIndexRequestSchema, {}));
	}

	async getServerSettings() {
		const client = this.getClient();
		return client.getServerSettings(create(GetServerSettingsRequestSchema, {}));
	}

	async updateServerSettings(userProvisioningEnabled: boolean, sshAccessForAll: boolean) {
		const client = this.getClient();
		return client.updateServerSettings(create(UpdateServerSettingsRequestSchema, {
			userProvisioningEnabled,
			sshAccessForAll,
		}));
	}

	async setUserProvisioningEnabled(userId: string, enabled: boolean) {
		const client = this.getClient();
		return client.setUserProvisioningEnabled(create(SetUserProvisioningEnabledRequestSchema, {
			userId: { value: userId },
			enabled,
		}));
	}

	async startTerminal(deviceId: string, cols: number = 80, rows: number = 24) {
		const client = this.getClient();
		return client.startTerminal(create(StartTerminalRequestSchema, {
			deviceId: { value: deviceId },
			cols,
			rows,
		}));
	}

	async stopTerminal(sessionId: string) {
		const client = this.getClient();
		return client.stopTerminal(create(StopTerminalRequestSchema, {
			sessionId: { value: sessionId },
		}));
	}

	async listActiveTerminalSessions(
		pageSize: number = 50,
		pageToken: string = '',
		deviceId: string = '',
		userId: string = ''
	) {
		const client = this.getClient();
		return client.listActiveTerminalSessions(create(ListActiveTerminalSessionsRequestSchema, {
			pageSize,
			pageToken,
			deviceId: deviceId ? { value: deviceId } : undefined,
			userId: { value: userId },
		}));
	}

	async terminateTerminalSession(sessionId: string, reason: string = '') {
		const client = this.getClient();
		return client.terminateTerminalSession(create(TerminateTerminalSessionRequestSchema, {
			sessionId: { value: sessionId },
			reason,
		}));
	}

}

export function getErrorCode(error: unknown): ErrorCode | undefined {
	if (error instanceof ConnectError) {
		const details = error.findDetails(ErrorDetailSchema);
		const first = details[0];
		if (first && first.code !== ErrorCode.UNSPECIFIED) {
			return first.code;
		}
	}
	return undefined;
}

export function getRequestId(error: unknown): string | undefined {
	if (error instanceof ConnectError) {
		const details = error.findDetails(ErrorDetailSchema);
		const first = details[0];
		if (first && first.requestId) {
			return first.requestId.value;
		}
	}
	return undefined;
}

export type {
	User, Device, RegistrationToken, ManagedAction, ActionSet, Definition,
	DeviceGroup, Assignment, AuditEvent, InventoryTableResult,
	Role, PermissionInfo, UserGroup, UserGroupMember, IdentityProvider, IdentityLink,
	LpsPassword, LuksKey, CreateActionRequest, UpdateActionParamsRequest,
	AvailableItem, DeviceAssignee, CompliancePolicy, CompliancePolicyRule, DevicePolicyEvaluation,
	SearchResult, SshPublicKey, DeviceGroupMember, InheritedRole, ActionSetMember, DefinitionMember,
	StartTerminalResponse, TerminalSessionInfo
};
