package identity

const (
	PermGetCurrentUser = "GetCurrentUser"
	PermCreateApiToken = "CreateApiToken"
	PermListApiTokens  = "ListApiTokens"
	PermRevokeApiToken = "RevokeApiToken"

	PermGetUser                    = "GetUser"
	PermListUsers                  = "ListUsers"
	PermEraseJITUser               = "EraseJITUser"
	PermUpdateUserEmail            = "UpdateUserEmail"
	PermSetUserDisabled            = "SetUserDisabled"
	PermUpdateUserProfile          = "UpdateUserProfile"
	PermUpdateUserLinuxUsername    = "UpdateUserLinuxUsername"
	PermUpdateUserSshSettings      = "UpdateUserSshSettings"
	PermAddUserSshKey              = "AddUserSshKey"
	PermRemoveUserSshKey           = "RemoveUserSshKey"
	PermSetUserProvisioningEnabled = "SetUserProvisioningEnabled"

	PermCreateRole         = "CreateRole"
	PermGetRole            = "GetRole"
	PermListRoles          = "ListRoles"
	PermUpdateRole         = "UpdateRole"
	PermDeleteRole         = "DeleteRole"
	PermAssignRoleToUser   = "AssignRoleToUser"
	PermRevokeRoleFromUser = "RevokeRoleFromUser"
	PermListPermissions    = "ListPermissions"

	PermAssignRoleToUserGroup   = "AssignRoleToUserGroup"
	PermRevokeRoleFromUserGroup = "RevokeRoleFromUserGroup"
	PermCreateStaticUserGroup   = "CreateStaticUserGroup"
	PermCreateDynamicUserGroup  = "CreateDynamicUserGroup"
	PermGetUserGroup            = "GetUserGroup"
	PermListUserGroups          = "ListUserGroups"
	PermUpdateUserGroup         = "UpdateUserGroup"
	PermDeleteUserGroup         = "DeleteUserGroup"
	PermAddUserToGroup          = "AddUserToGroup"
	PermRemoveUserFromGroup     = "RemoveUserFromGroup"
	PermListUserGroupsForUser   = "ListUserGroupsForUser"
	PermSetUserGroupMaintenance = "SetUserGroupMaintenanceWindow"
	PermUpdateDynamicUserGroup  = "UpdateDynamicUserGroupQuery"
	PermValidateUserGroupQuery  = "ValidateUserGroupQuery"
	PermEvaluateDynamicGroup    = "EvaluateDynamicUserGroup"

	PermCreateIdentityProvider = "CreateIdentityProvider"
	PermGetIdentityProvider    = "GetIdentityProvider"
	PermListIdentityProviders  = "ListIdentityProviders"
	PermUpdateIdentityProvider = "UpdateIdentityProvider"
	PermDeleteIdentityProvider = "DeleteIdentityProvider"
	PermEnableSCIM             = "EnableSCIM"
	PermDisableSCIM            = "DisableSCIM"
	PermRotateSCIMToken        = "RotateSCIMToken"

	PermListIdentityLinks = "ListIdentityLinks"
	PermUnlinkIdentity    = "UnlinkIdentity"

	PermGetServerSettings    = "GetServerSettings"
	PermUpdateServerSettings = "UpdateServerSettings"
	PermListAuditEvents      = "ListAuditEvents"
)

var gatedPermissions = []string{
	PermGetCurrentUser, PermCreateApiToken, PermListApiTokens, PermRevokeApiToken,
	PermGetUser, PermListUsers, PermEraseJITUser, PermUpdateUserEmail,
	PermSetUserDisabled, PermUpdateUserProfile, PermUpdateUserLinuxUsername,
	PermUpdateUserSshSettings, PermAddUserSshKey, PermRemoveUserSshKey,
	PermSetUserProvisioningEnabled,
	PermCreateRole, PermGetRole, PermListRoles, PermUpdateRole, PermDeleteRole,
	PermAssignRoleToUser, PermRevokeRoleFromUser, PermListPermissions,
	PermAssignRoleToUserGroup, PermRevokeRoleFromUserGroup,
	PermCreateStaticUserGroup, PermCreateDynamicUserGroup,
	PermGetUserGroup, PermListUserGroups, PermUpdateUserGroup, PermDeleteUserGroup,
	PermAddUserToGroup, PermRemoveUserFromGroup, PermListUserGroupsForUser,
	PermSetUserGroupMaintenance,
	PermUpdateDynamicUserGroup, PermValidateUserGroupQuery, PermEvaluateDynamicGroup,
	PermCreateIdentityProvider, PermGetIdentityProvider, PermListIdentityProviders,
	PermUpdateIdentityProvider, PermDeleteIdentityProvider,
	PermEnableSCIM, PermDisableSCIM, PermRotateSCIMToken,
	PermListIdentityLinks, PermUnlinkIdentity,
	PermGetServerSettings, PermUpdateServerSettings, PermListAuditEvents,
}

func GatedPermissions() []string {
	return append([]string(nil), gatedPermissions...)
}
