package identity

import (
	"net/http"

	"connectrpc.com/connect"

	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
)

// Mount registers the identity procedures on mux.
//
// Each procedure is mounted at its own canonical Connect path with the
// shared interceptor chain, rather than through the whole-service
// constructor: the identity handlers are one part of the control
// service, and mounting only what this package implements keeps the
// wiring honest about which procedures are actually served.
//
// Procedures returns the exact set that was mounted, so a test can
// assert the surface rather than trusting this list.
func (h *Handlers) Mount(mux *http.ServeMux, opts ...connect.HandlerOption) []string {
	var mounted []string
	register := func(procedure string, handler http.Handler) {
		mux.Handle(procedure, handler)
		mounted = append(mounted, procedure)
	}
	// Sessions.
	register(cadestrov1connect.ControlServiceRefreshTokenProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceRefreshTokenProcedure, h.RefreshToken, opts...))
	register(cadestrov1connect.ControlServiceLogoutProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceLogoutProcedure, h.Logout, opts...))
	register(cadestrov1connect.ControlServiceGetCurrentUserProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceGetCurrentUserProcedure, h.GetCurrentUser, opts...))

	// Single sign-on.
	register(cadestrov1connect.ControlServiceListAuthMethodsProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceListAuthMethodsProcedure, h.ListAuthMethods, opts...))
	register(cadestrov1connect.ControlServiceGetSSOLoginURLProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceGetSSOLoginURLProcedure, h.GetSSOLoginURL, opts...))
	register(cadestrov1connect.ControlServiceSSOCallbackProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceSSOCallbackProcedure, h.SSOCallback, opts...))

	// Identity providers.
	register(cadestrov1connect.ControlServiceCreateIdentityProviderProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceCreateIdentityProviderProcedure, h.CreateIdentityProvider, opts...))
	register(cadestrov1connect.ControlServiceGetIdentityProviderProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceGetIdentityProviderProcedure, h.GetIdentityProvider, opts...))
	register(cadestrov1connect.ControlServiceListIdentityProvidersProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceListIdentityProvidersProcedure, h.ListIdentityProviders, opts...))
	register(cadestrov1connect.ControlServiceUpdateIdentityProviderProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceUpdateIdentityProviderProcedure, h.UpdateIdentityProvider, opts...))
	register(cadestrov1connect.ControlServiceDeleteIdentityProviderProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceDeleteIdentityProviderProcedure, h.DeleteIdentityProvider, opts...))
	register(cadestrov1connect.ControlServiceEnableSCIMProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceEnableSCIMProcedure, h.EnableSCIM, opts...))
	register(cadestrov1connect.ControlServiceDisableSCIMProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceDisableSCIMProcedure, h.DisableSCIM, opts...))
	register(cadestrov1connect.ControlServiceRotateSCIMTokenProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceRotateSCIMTokenProcedure, h.RotateSCIMToken, opts...))

	// Identity links.
	register(cadestrov1connect.ControlServiceListIdentityLinksProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceListIdentityLinksProcedure, h.ListIdentityLinks, opts...))
	register(cadestrov1connect.ControlServiceUnlinkIdentityProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceUnlinkIdentityProcedure, h.UnlinkIdentity, opts...))

	// Users.
	register(cadestrov1connect.ControlServiceEraseJITUserProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceEraseJITUserProcedure, h.EraseJITUser, opts...))
	register(cadestrov1connect.ControlServiceGetUserProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceGetUserProcedure, h.GetUser, opts...))
	register(cadestrov1connect.ControlServiceListUsersProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceListUsersProcedure, h.ListUsers, opts...))
	register(cadestrov1connect.ControlServiceUpdateUserEmailProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceUpdateUserEmailProcedure, h.UpdateUserEmail, opts...))
	register(cadestrov1connect.ControlServiceSetUserDisabledProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceSetUserDisabledProcedure, h.SetUserDisabled, opts...))
	register(cadestrov1connect.ControlServiceUpdateUserProfileProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceUpdateUserProfileProcedure, h.UpdateUserProfile, opts...))
	register(cadestrov1connect.ControlServiceUpdateUserLinuxUsernameProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceUpdateUserLinuxUsernameProcedure, h.UpdateUserLinuxUsername, opts...))
	register(cadestrov1connect.ControlServiceUpdateUserSshSettingsProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceUpdateUserSshSettingsProcedure, h.UpdateUserSshSettings, opts...))
	register(cadestrov1connect.ControlServiceAddUserSshKeyProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceAddUserSshKeyProcedure, h.AddUserSshKey, opts...))
	register(cadestrov1connect.ControlServiceRemoveUserSshKeyProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceRemoveUserSshKeyProcedure, h.RemoveUserSshKey, opts...))
	register(cadestrov1connect.ControlServiceSetUserProvisioningEnabledProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceSetUserProvisioningEnabledProcedure, h.SetUserProvisioningEnabled, opts...))
	// User groups and membership.
	register(cadestrov1connect.ControlServiceCreateUserGroupProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceCreateUserGroupProcedure, h.CreateUserGroup, opts...))
	register(cadestrov1connect.ControlServiceGetUserGroupProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceGetUserGroupProcedure, h.GetUserGroup, opts...))
	register(cadestrov1connect.ControlServiceListUserGroupsProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceListUserGroupsProcedure, h.ListUserGroups, opts...))
	register(cadestrov1connect.ControlServiceUpdateUserGroupProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceUpdateUserGroupProcedure, h.UpdateUserGroup, opts...))
	register(cadestrov1connect.ControlServiceDeleteUserGroupProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceDeleteUserGroupProcedure, h.DeleteUserGroup, opts...))
	register(cadestrov1connect.ControlServiceAddUserToGroupProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceAddUserToGroupProcedure, h.AddUserToGroup, opts...))
	register(cadestrov1connect.ControlServiceRemoveUserFromGroupProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceRemoveUserFromGroupProcedure, h.RemoveUserFromGroup, opts...))
	register(cadestrov1connect.ControlServiceListUserGroupsForUserProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceListUserGroupsForUserProcedure, h.ListUserGroupsForUser, opts...))
	register(cadestrov1connect.ControlServiceSetUserGroupMaintenanceWindowProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceSetUserGroupMaintenanceWindowProcedure, h.SetUserGroupMaintenanceWindow, opts...))
	register(cadestrov1connect.ControlServiceUpdateUserGroupQueryProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceUpdateUserGroupQueryProcedure, h.UpdateUserGroupQuery, opts...))
	register(cadestrov1connect.ControlServiceValidateUserGroupQueryProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceValidateUserGroupQueryProcedure, h.ValidateUserGroupQuery, opts...))
	register(cadestrov1connect.ControlServiceEvaluateDynamicUserGroupProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceEvaluateDynamicUserGroupProcedure, h.EvaluateDynamicUserGroup, opts...))

	// Roles and grants.
	register(cadestrov1connect.ControlServiceCreateRoleProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceCreateRoleProcedure, h.CreateRole, opts...))
	register(cadestrov1connect.ControlServiceGetRoleProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceGetRoleProcedure, h.GetRole, opts...))
	register(cadestrov1connect.ControlServiceListRolesProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceListRolesProcedure, h.ListRoles, opts...))
	register(cadestrov1connect.ControlServiceUpdateRoleProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceUpdateRoleProcedure, h.UpdateRole, opts...))
	register(cadestrov1connect.ControlServiceDeleteRoleProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceDeleteRoleProcedure, h.DeleteRole, opts...))
	register(cadestrov1connect.ControlServiceAssignRoleToUserProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceAssignRoleToUserProcedure, h.AssignRoleToUser, opts...))
	register(cadestrov1connect.ControlServiceRevokeRoleFromUserProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceRevokeRoleFromUserProcedure, h.RevokeRoleFromUser, opts...))
	register(cadestrov1connect.ControlServiceAssignRoleToUserGroupProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceAssignRoleToUserGroupProcedure, h.AssignRoleToUserGroup, opts...))
	register(cadestrov1connect.ControlServiceRevokeRoleFromUserGroupProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceRevokeRoleFromUserGroupProcedure, h.RevokeRoleFromUserGroup, opts...))
	register(cadestrov1connect.ControlServiceListPermissionsProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceListPermissionsProcedure, h.ListPermissions, opts...))

	// Fleet settings.
	register(cadestrov1connect.ControlServiceGetServerSettingsProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceGetServerSettingsProcedure, h.GetServerSettings, opts...))
	register(cadestrov1connect.ControlServiceUpdateServerSettingsProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceUpdateServerSettingsProcedure, h.UpdateServerSettings, opts...))

	// Append-only audit evidence.
	register(cadestrov1connect.ControlServiceListAuditEventsProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceListAuditEventsProcedure, h.ListAuditEvents, opts...))
	register(cadestrov1connect.ControlServiceExportAuditEventsProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceExportAuditEventsProcedure, h.ExportAuditEvents, opts...))

	return mounted
}

// MutationProcedures is the exact set of identity procedures that
// change state. It is what an audit-coverage test enumerates: every
// entry must be shown to write its operation and effects in the same
// transaction as its mutation, and an entry added to Mount without
// being classified here fails that test.
func MutationProcedures() []string {
	return []string{
		cadestrov1connect.ControlServiceRefreshTokenProcedure,
		cadestrov1connect.ControlServiceLogoutProcedure,
		cadestrov1connect.ControlServiceGetSSOLoginURLProcedure,
		cadestrov1connect.ControlServiceSSOCallbackProcedure,
		cadestrov1connect.ControlServiceCreateIdentityProviderProcedure,
		cadestrov1connect.ControlServiceUpdateIdentityProviderProcedure,
		cadestrov1connect.ControlServiceDeleteIdentityProviderProcedure,
		cadestrov1connect.ControlServiceEnableSCIMProcedure,
		cadestrov1connect.ControlServiceDisableSCIMProcedure,
		cadestrov1connect.ControlServiceRotateSCIMTokenProcedure,
		cadestrov1connect.ControlServiceUnlinkIdentityProcedure,
		cadestrov1connect.ControlServiceEraseJITUserProcedure,
		cadestrov1connect.ControlServiceUpdateUserEmailProcedure,
		cadestrov1connect.ControlServiceSetUserDisabledProcedure,
		cadestrov1connect.ControlServiceUpdateUserProfileProcedure,
		cadestrov1connect.ControlServiceUpdateUserLinuxUsernameProcedure,
		cadestrov1connect.ControlServiceUpdateUserSshSettingsProcedure,
		cadestrov1connect.ControlServiceAddUserSshKeyProcedure,
		cadestrov1connect.ControlServiceRemoveUserSshKeyProcedure,
		cadestrov1connect.ControlServiceSetUserProvisioningEnabledProcedure,
		cadestrov1connect.ControlServiceCreateUserGroupProcedure,
		cadestrov1connect.ControlServiceUpdateUserGroupProcedure,
		cadestrov1connect.ControlServiceDeleteUserGroupProcedure,
		cadestrov1connect.ControlServiceAddUserToGroupProcedure,
		cadestrov1connect.ControlServiceRemoveUserFromGroupProcedure,
		cadestrov1connect.ControlServiceSetUserGroupMaintenanceWindowProcedure,
		cadestrov1connect.ControlServiceUpdateUserGroupQueryProcedure,
		cadestrov1connect.ControlServiceEvaluateDynamicUserGroupProcedure,
		cadestrov1connect.ControlServiceCreateRoleProcedure,
		cadestrov1connect.ControlServiceUpdateRoleProcedure,
		cadestrov1connect.ControlServiceDeleteRoleProcedure,
		cadestrov1connect.ControlServiceAssignRoleToUserProcedure,
		cadestrov1connect.ControlServiceRevokeRoleFromUserProcedure,
		cadestrov1connect.ControlServiceAssignRoleToUserGroupProcedure,
		cadestrov1connect.ControlServiceRevokeRoleFromUserGroupProcedure,
		cadestrov1connect.ControlServiceUpdateServerSettingsProcedure,
	}
}

// ReadProcedures is the exact set of identity procedures that change
// nothing.
func ReadProcedures() []string {
	return []string{
		cadestrov1connect.ControlServiceGetCurrentUserProcedure,
		cadestrov1connect.ControlServiceListAuthMethodsProcedure,
		cadestrov1connect.ControlServiceGetIdentityProviderProcedure,
		cadestrov1connect.ControlServiceListIdentityProvidersProcedure,
		cadestrov1connect.ControlServiceListIdentityLinksProcedure,
		cadestrov1connect.ControlServiceGetUserProcedure,
		cadestrov1connect.ControlServiceListUsersProcedure,
		cadestrov1connect.ControlServiceGetUserGroupProcedure,
		cadestrov1connect.ControlServiceListUserGroupsProcedure,
		cadestrov1connect.ControlServiceListUserGroupsForUserProcedure,
		cadestrov1connect.ControlServiceValidateUserGroupQueryProcedure,
		cadestrov1connect.ControlServiceGetRoleProcedure,
		cadestrov1connect.ControlServiceListRolesProcedure,
		cadestrov1connect.ControlServiceListPermissionsProcedure,
		cadestrov1connect.ControlServiceGetServerSettingsProcedure,
		cadestrov1connect.ControlServiceListAuditEventsProcedure,
	}
}

// SensitiveReadProcedures is the exact read surface that must append audit
// evidence before returning protected material.
func SensitiveReadProcedures() []string {
	return []string{
		cadestrov1connect.ControlServiceExportAuditEventsProcedure,
	}
}
