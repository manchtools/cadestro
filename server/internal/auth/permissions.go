package auth

type PermissionTargetKind int

const (
	TargetUnspecified PermissionTargetKind = iota

	TargetDevice

	TargetUser
)

type PermissionInfo struct {
	Key         string
	Group       string
	Description string

	TargetKind PermissionTargetKind

	PrivilegeGranting bool
}

var privilegeGrantingKeys = map[string]bool{

	"CreateRole":              true,
	"UpdateRole":              true,
	"DeleteRole":              true,
	"AssignRoleToUser":        true,
	"RevokeRoleFromUser":      true,
	"AssignRoleToUserGroup":   true,
	"RevokeRoleFromUserGroup": true,
	"AssignRoleScope":         true,

	"AddUserToGroup":      true,
	"RemoveUserFromGroup": true,

	"SetUserDisabled": true,

	"CreateIdentityProvider": true,
	"UpdateIdentityProvider": true,
	"DeleteIdentityProvider": true,
	"EnableSCIM":             true,
	"DisableSCIM":            true,
	"RotateSCIMToken":        true,

	"UpdateServerSettings": true,
}

func AllPermissions() []PermissionInfo {
	raw := registryPermissions()
	perms := make([]PermissionInfo, len(raw))
	for i, e := range raw {
		perms[i] = PermissionInfo{
			Key:               e.key,
			Group:             e.group,
			Description:       e.description,
			TargetKind:        e.targetKind,
			PrivilegeGranting: privilegeGrantingKeys[e.key],
		}
	}
	return perms
}

type permEntry struct {
	key         string
	group       string
	description string
	targetKind  PermissionTargetKind
}

func registryPermissions() []permEntry {
	return []permEntry{

		{"GetCurrentUser", "Users", "View own profile", TargetUnspecified},
		{"CreateApiToken", "Tokens", "Create API tokens for yourself", TargetUnspecified},
		{"ListApiTokens", "Tokens", "List your API tokens", TargetUnspecified},
		{"RevokeApiToken", "Tokens", "Revoke your API tokens", TargetUnspecified},
		{"GetUser", "Users", "View any user", TargetUser},
		{"GetUser:self", "Users", "View own profile only", TargetUnspecified},
		{"ListUsers", "Users", "List all users", TargetUser},
		{"EraseJITUser", "Users", "Erase OIDC JIT users", TargetUser},
		{"UpdateUserEmail", "Users", "Change any user's email", TargetUser},
		{"UpdateUserEmail:self", "Users", "Change own email", TargetUnspecified},

		{"SetUserDisabled", "Users", "Disable/enable users", TargetUnspecified},
		{"UpdateUserProfile", "Users", "Update any user's profile", TargetUser},
		{"UpdateUserProfile:self", "Users", "Update own profile", TargetUnspecified},
		{"UpdateUserSshSettings", "Users", "Update any user's SSH settings", TargetUser},
		{"UpdateUserSshSettings:self", "Users", "Update own SSH settings", TargetUnspecified},

		{"UpdateUserLinuxUsername", "Users", "Change any user's linux username", TargetUser},
		{"AddUserSshKey", "Users", "Add SSH key to any user", TargetUser},
		{"AddUserSshKey:self", "Users", "Add own SSH key", TargetUnspecified},
		{"RemoveUserSshKey", "Users", "Remove SSH key from any user", TargetUser},
		{"RemoveUserSshKey:self", "Users", "Remove own SSH key", TargetUnspecified},

		{"ListDevices", "Devices", "List all devices", TargetDevice},
		{"ListDevices:assigned", "Devices", "List own assigned devices", TargetUnspecified},
		{"GetDevice", "Devices", "View any device", TargetDevice},
		{"GetDevice:assigned", "Devices", "View own assigned devices", TargetUnspecified},

		{"SetDeviceLabel", "Devices", "Set device labels", TargetUnspecified},
		{"RemoveDeviceLabel", "Devices", "Remove device labels", TargetUnspecified},

		{"AssignDevice", "Devices", "Assign devices to users or groups", TargetUnspecified},
		{"UnassignDevice", "Devices", "Unassign devices from users or groups", TargetUnspecified},
		{"ListDeviceAssignees", "Devices", "List device assignees", TargetUnspecified},
		{"SetDeviceSyncInterval", "Devices", "Set device sync interval", TargetDevice},
		{"SetDeviceInventoryInterval", "Devices", "Set device inventory collection interval", TargetDevice},
		{"DeleteDevice", "Devices", "Delete devices", TargetDevice},

		{"CreateToken", "Tokens", "Create registration tokens", TargetUnspecified},
		{"ListTokens", "Tokens", "List tokens", TargetUnspecified},
		{"RenameToken", "Tokens", "Rename tokens", TargetUnspecified},
		{"SetTokenDisabled", "Tokens", "Disable/enable tokens", TargetUnspecified},
		{"DeleteToken", "Tokens", "Delete tokens", TargetUnspecified},

		{"CreateAction", "Actions", "Create actions", TargetUnspecified},
		{"GetAction", "Actions", "View actions", TargetUnspecified},
		{"ListActions", "Actions", "List actions", TargetUnspecified},
		{"RenameAction", "Actions", "Rename actions", TargetUnspecified},
		{"UpdateActionDescription", "Actions", "Update action descriptions", TargetUnspecified},
		{"UpdateActionParams", "Actions", "Update action parameters", TargetUnspecified},
		{"DeleteAction", "Actions", "Delete actions", TargetUnspecified},

		{"CreateActionSet", "Action Sets", "Create action sets", TargetUnspecified},
		{"GetActionSet", "Action Sets", "View action sets", TargetUnspecified},
		{"ListActionSets", "Action Sets", "List action sets", TargetUnspecified},
		{"RenameActionSet", "Action Sets", "Rename action sets", TargetUnspecified},
		{"UpdateActionSetDescription", "Action Sets", "Update action set descriptions", TargetUnspecified},
		{"UpdateActionSetSchedule", "Action Sets", "Update action set schedule", TargetUnspecified},
		{"DeleteActionSet", "Action Sets", "Delete action sets", TargetUnspecified},
		{"AddActionToSet", "Action Sets", "Add actions to sets", TargetUnspecified},
		{"RemoveActionFromSet", "Action Sets", "Remove actions from sets", TargetUnspecified},
		{"ReorderActionInSet", "Action Sets", "Reorder actions in sets", TargetUnspecified},

		{"CreateDefinition", "Definitions", "Create definitions", TargetUnspecified},
		{"GetDefinition", "Definitions", "View definitions", TargetUnspecified},
		{"ListDefinitions", "Definitions", "List definitions", TargetUnspecified},
		{"RenameDefinition", "Definitions", "Rename definitions", TargetUnspecified},
		{"UpdateDefinitionDescription", "Definitions", "Update definition descriptions", TargetUnspecified},
		{"UpdateDefinitionSchedule", "Definitions", "Update definition schedule", TargetUnspecified},
		{"DeleteDefinition", "Definitions", "Delete definitions", TargetUnspecified},
		{"AddActionSetToDefinition", "Definitions", "Add action sets to definitions", TargetUnspecified},
		{"RemoveActionSetFromDefinition", "Definitions", "Remove action sets from definitions", TargetUnspecified},
		{"ReorderActionSetInDefinition", "Definitions", "Reorder action sets in definitions", TargetUnspecified},

		{"CreateStaticDeviceGroup", "Device Groups", "Create static device groups", TargetUnspecified},
		{"CreateDynamicDeviceGroup", "Device Groups", "Create dynamic device groups", TargetUnspecified},
		{"GetDeviceGroup", "Device Groups", "View device groups", TargetDevice},
		{"ListDeviceGroups", "Device Groups", "List device groups", TargetDevice},
		{"ListDeviceGroupsForDevice", "Device Groups", "List device groups for a device", TargetDevice},
		{"RenameDeviceGroup", "Device Groups", "Rename device groups", TargetDevice},
		{"UpdateDeviceGroupDescription", "Device Groups", "Update device group descriptions", TargetDevice},
		{"UpdateDynamicDeviceGroupQuery", "Device Groups", "Update dynamic device group queries", TargetUnspecified},
		{"DeleteDeviceGroup", "Device Groups", "Delete device groups", TargetDevice},
		{"AddDeviceToGroup", "Device Groups", "Add devices to groups", TargetDevice},
		{"RemoveDeviceFromGroup", "Device Groups", "Remove devices from groups", TargetDevice},
		{"ValidateDynamicQuery", "Device Groups", "Validate dynamic device group queries", TargetUnspecified},
		{"EvaluateDynamicGroup", "Device Groups", "Evaluate dynamic groups", TargetUnspecified},
		{"SetDeviceGroupSyncInterval", "Device Groups", "Set device group sync interval", TargetDevice},
		{"SetDeviceGroupInventoryInterval", "Device Groups", "Set device group inventory collection interval", TargetDevice},
		{"SetDeviceGroupMaintenanceWindow", "Device Groups", "Set device group maintenance window", TargetDevice},

		{"CreateAssignment", "Assignments", "Create assignments", TargetUnspecified},
		{"DeleteAssignment", "Assignments", "Delete assignments", TargetUnspecified},
		{"ListAssignments", "Assignments", "List assignments", TargetUnspecified},
		{"GetDeviceAssignments", "Assignments", "View device assignments", TargetUnspecified},
		{"GetUserAssignments", "Assignments", "View user assignments", TargetUnspecified},

		{"SetUserSelection", "User Selections", "Manage user selections", TargetUnspecified},
		{"ListAvailableActions", "User Selections", "List available actions", TargetUnspecified},

		{"SyncDevice", "Live Control", "Sync device", TargetDevice},
		{"RebootDevice", "Live Control", "Reboot device", TargetDevice},

		{"DispatchOSQuery", "OSQuery", "Run OSQuery on device", TargetDevice},
		{"GetOSQueryResult", "OSQuery", "View OSQuery results", TargetDevice},
		{"GetDeviceInventory", "OSQuery", "View device inventory", TargetDevice},
		{"RefreshDeviceInventory", "OSQuery", "Refresh device inventory", TargetDevice},

		{"QueryDeviceLogs", "Device Logs", "Query device logs", TargetDevice},
		{"GetDeviceLogResult", "Device Logs", "View device log results", TargetDevice},

		{"GetDeviceCompliance", "Compliance", "View device compliance", TargetDevice},
		{"GetDeviceCompliance:assigned", "Compliance", "View compliance for assigned devices", TargetUnspecified},

		{"CreateCompliancePolicy", "Compliance Policies", "Create compliance policies", TargetUnspecified},
		{"GetCompliancePolicy", "Compliance Policies", "View compliance policies", TargetUnspecified},
		{"ListCompliancePolicies", "Compliance Policies", "List compliance policies", TargetUnspecified},
		{"RenameCompliancePolicy", "Compliance Policies", "Rename compliance policies", TargetUnspecified},
		{"UpdateCompliancePolicyDescription", "Compliance Policies", "Update compliance policy descriptions", TargetUnspecified},
		{"DeleteCompliancePolicy", "Compliance Policies", "Delete compliance policies", TargetUnspecified},
		{"AddCompliancePolicyRule", "Compliance Policies", "Add rules to compliance policies", TargetUnspecified},
		{"RemoveCompliancePolicyRule", "Compliance Policies", "Remove rules from compliance policies", TargetUnspecified},
		{"UpdateCompliancePolicyRule", "Compliance Policies", "Update compliance policy rules", TargetUnspecified},
		{"GetDeviceCompliancePolicyStatus", "Compliance Policies", "View device compliance policy status", TargetDevice},
		{"GetDeviceCompliancePolicyStatus:assigned", "Compliance Policies", "View compliance policy status for assigned devices", TargetUnspecified},

		{"ListAuditEvents", "Audit", "View audit log", TargetUnspecified},

		{"ListLpsPasswords", "LPS", "List LPS password metadata", TargetUnspecified},
		{"RevealLpsPassword", "LPS", "Reveal one LPS password", TargetUnspecified},

		{"ListLuksKeys", "LUKS", "List LUKS key metadata", TargetUnspecified},
		{"RevealLuksKey", "LUKS", "Reveal one LUKS key", TargetUnspecified},
		{"CreateLuksToken", "LUKS", "Create LUKS recovery token", TargetUnspecified},
		{"RevokeLuksDeviceKey", "LUKS", "Revoke LUKS device key", TargetUnspecified},

		{"CreateRole", "Roles", "Create roles", TargetUnspecified},
		{"GetRole", "Roles", "View roles", TargetUnspecified},
		{"ListRoles", "Roles", "List roles", TargetUnspecified},
		{"UpdateRole", "Roles", "Update roles", TargetUnspecified},
		{"DeleteRole", "Roles", "Delete roles", TargetUnspecified},
		{"AssignRoleToUser", "Roles", "Assign roles to users", TargetUnspecified},
		{"RevokeRoleFromUser", "Roles", "Revoke roles from users", TargetUnspecified},
		{"AssignRoleScope", "Roles", "Attach a scope (device group / user group) to a role grant", TargetUnspecified},
		{"ListPermissions", "Roles", "List available permissions", TargetUnspecified},

		{"CreateStaticUserGroup", "User Groups", "Create static user groups", TargetUnspecified},
		{"CreateDynamicUserGroup", "User Groups", "Create dynamic user groups", TargetUnspecified},
		{"GetUserGroup", "User Groups", "View user groups", TargetUser},
		{"ListUserGroups", "User Groups", "List user groups", TargetUser},
		{"UpdateUserGroup", "User Groups", "Update user groups", TargetUser},
		{"DeleteUserGroup", "User Groups", "Delete user groups", TargetUser},

		{"AddUserToGroup", "User Groups", "Add users to groups", TargetUnspecified},
		{"RemoveUserFromGroup", "User Groups", "Remove users from groups", TargetUnspecified},
		{"AssignRoleToUserGroup", "User Groups", "Assign roles to user groups", TargetUnspecified},
		{"RevokeRoleFromUserGroup", "User Groups", "Revoke roles from user groups", TargetUnspecified},
		{"ListUserGroupsForUser", "User Groups", "List user groups for a user", TargetUser},
		{"UpdateDynamicUserGroupQuery", "User Groups", "Update dynamic user group queries", TargetUnspecified},
		{"ValidateUserGroupQuery", "User Groups", "Validate user group queries", TargetUnspecified},
		{"EvaluateDynamicUserGroup", "User Groups", "Evaluate dynamic user groups", TargetUnspecified},
		{"SetUserGroupMaintenanceWindow", "User Groups", "Set user group maintenance window", TargetUser},

		{"CreateIdentityProvider", "Identity Providers", "Create identity providers", TargetUnspecified},
		{"GetIdentityProvider", "Identity Providers", "View identity providers", TargetUnspecified},
		{"ListIdentityProviders", "Identity Providers", "List identity providers", TargetUnspecified},
		{"UpdateIdentityProvider", "Identity Providers", "Update identity providers", TargetUnspecified},
		{"DeleteIdentityProvider", "Identity Providers", "Delete identity providers", TargetUnspecified},
		{"EnableSCIM", "Identity Providers", "Enable SCIM provisioning", TargetUnspecified},
		{"DisableSCIM", "Identity Providers", "Disable SCIM provisioning", TargetUnspecified},
		{"RotateSCIMToken", "Identity Providers", "Rotate SCIM token", TargetUnspecified},

		{"ListIdentityLinks", "Authentication", "View own linked identities", TargetUnspecified},
		{"UnlinkIdentity", "Authentication", "Unlink own identity", TargetUnspecified},

		{"Search", "Search", "Search across entities", TargetUnspecified},
		{"RebuildSearchIndex", "Search", "Force rebuild search index", TargetUnspecified},

		{"GetServerSettings", "Server Settings", "View server settings", TargetUnspecified},
		{"UpdateServerSettings", "Server Settings", "Update server settings", TargetUnspecified},

		{"SetUserProvisioningEnabled", "Users", "Toggle user provisioning per user", TargetUser},

		{"StartTerminal", "Remote Terminal", "Open a remote terminal session on a device", TargetDevice},
		{"StopTerminal", "Remote Terminal", "Stop a remote terminal session you opened", TargetDevice},
		{"ListActiveTerminalSessions", "Remote Terminal", "View active terminal sessions across all devices (admin)", TargetDevice},
		{"TerminateTerminalSession", "Remote Terminal", "Forcibly terminate any terminal session (admin)", TargetDevice},
		{"TerminalAdminLimited", "Remote Terminal", "Grant a passwordless LIMITED sudoers policy in remote terminal sessions", TargetDevice},
		{"TerminalAdminFull", "Remote Terminal", "Grant a passwordless FULL sudoers policy in remote terminal sessions", TargetDevice},
	}
}

func AdminPermissions() []string {
	perms := make([]string, len(AllPermissions()))
	for i, p := range AllPermissions() {
		perms[i] = p.Key
	}
	return perms
}

func DefaultUserPermissions() []string {
	return []string{
		"GetCurrentUser", "CreateApiToken", "ListApiTokens", "RevokeApiToken",
		"GetUser:self",
		"UpdateUserEmail:self",
		"UpdateUserProfile:self",
		"ListDevices:assigned",
		"GetDevice:assigned",
		"SetUserSelection",
		"ListAvailableActions",
		"ListIdentityLinks",
		"UnlinkIdentity",
		"GetDeviceCompliance:assigned",
		"UpdateUserSshSettings:self",

		"AddUserSshKey:self",
		"RemoveUserSshKey:self",
		"StopTerminal",
	}
}

func ValidPermissionKeys() map[string]bool {
	m := make(map[string]bool)
	for _, p := range AllPermissions() {
		m[p.Key] = true
	}
	return m
}

var permTargetKinds = func() map[string]PermissionTargetKind {
	m := make(map[string]PermissionTargetKind)
	for _, p := range AllPermissions() {
		m[p.Key] = p.TargetKind
	}
	return m
}()

func TargetKindFor(key string) PermissionTargetKind {
	return permTargetKinds[key]
}

func IsPrivilegeGranting(key string) bool {
	if privilegeGrantingKeys[key] {
		return true
	}
	return !ValidPermissionKeys()[key]
}

func FirstPrivilegeGranting(perms []string) (string, bool) {
	for _, p := range perms {
		if IsPrivilegeGranting(p) {
			return p, true
		}
	}
	return "", false
}
