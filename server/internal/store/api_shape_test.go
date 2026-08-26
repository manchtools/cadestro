package store_test

import (
	"reflect"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manchtools/cadestro/server/internal/store"
)

var mutationCapableExports = map[string]string{
	"WithAudit":                  "the audited mutation door",
	"RecordOperation":            "audited operation with no state change",
	"WithAuditEffects":           "audited continuation of an existing operation",
	"RebuildSearchIndexes":       "audited SQLite FTS5 index maintenance",
	"RecordHeartbeatTelemetry":   "bounded high-rate telemetry exception",
	"CleanupExpiredAuthStates":   "audited one-time OIDC state cleanup",
	"RecordPolicyActionResult":   "audited policy result ingestion",
	"RecordPolicyManifestResult": "audited policy result ingestion",
}

var nonMutatingExports = map[string]string{
	"Close":                            "releases the pool",
	"Ping":                             "read-only connectivity check",
	"SetLogger":                        "in-process wiring",
	"GetAuditOperation":                "read",
	"ListAuditEffects":                 "read",
	"ListAuditEventRows":               "read",
	"CountAuditEventRows":              "read",
	"CountAuditOperations":             "read",
	"GetDevice":                        "read",
	"GetDeviceSecret":                  "read",
	"GetCurrentLuksKeyForAgent":        "read",
	"ListDeviceMaintenanceWindows":     "read",
	"GetJob":                           "read",
	"GetLiveJobByDedupe":               "read",
	"ListClaimableJobs":                "read",
	"GetManifestAction":                "read",
	"GetManifestActionSet":             "read",
	"ListManifestActionSetActions":     "read",
	"GetManifestDefinition":            "read",
	"ListManifestDefinitionActionSets": "read",
	"ListManifestDefinitionActions":    "read",
	"ListAuthoringActions":             "read",
	"CountAuthoringActions":            "read",
	"ListAuthoringAssignmentTargets":   "read",
	"GetAssignment":                    "read",
	"FindAssignment":                   "read",
	"ListAssignments":                  "read",
	"CountAssignments":                 "read",
	"ListAssignmentsForUser":           "read",
	"ListAvailableSources":             "read",
	"ListResolvedSources":              "read",
	"GetDeviceGroupID":                 "read",
	"GetDeviceGroup":                   "read",
	"ListDeviceGroupMembers":           "read",
	"ListDevicesForDynamicEvaluation":  "read",
	"ListDeviceGroups":                 "read",
	"CountDeviceGroups":                "read",
	"ListDeviceGroupsForDevice":        "read",
	"ListContainingActionSetIDs":       "read",
	"ListContainingDefinitionIDs":      "read",
	"ListAuthoringActionSets":          "read",
	"CountAuthoringActionSets":         "read",
	"ListRegistrationTokens":           "read",
	"CountRegistrationTokens":          "read",
	"CountDevices":                     "read",
	"CountActions":                     "read",
	"CountActionSets":                  "read",
	"ListActionSetMembers":             "read",
	"ListAuthoringDefinitions":         "read",
	"CountAuthoringDefinitions":        "read",
	"CountDefinitions":                 "read",
	"ListDefinitionMembers":            "read",
	"GetAuthoringCompliancePolicy":     "read",
	"ListAuthoringCompliancePolicies":  "read",
	"CountAuthoringCompliancePolicies": "read",
	"ListCompliancePolicyRules":        "read",
	"ListCompliancePolicyIDsForAction": "read",
	"GetDeviceView":                    "read",
	"ListDeviceViews":                  "read",
	"CountDeviceViews":                 "read",
	"ListDeviceGroupIDs":               "read",
	"IsDeviceAssignedToUser":           "read",
	"ListDeviceAssignees":              "read",
	"ListDeviceInventory":              "read",
	"GetOSQueryResult":                 "read",
	"GetDeviceLogResult":               "read",
	"ListDeviceComplianceResults":      "read",
	"ListDeviceComplianceEvaluations":  "read",
	"ListDeviceLpsPasswords":           "read",
	"ListDeviceLuksKeys":               "read",
	"GetLpsPasswordForReveal":          "read",
	"GetLuksKeyForReveal":              "read",
	"GetLuksRevocationTarget":          "read",
	"GetOpenTerminalSession":           "read",
	"IsDeviceDirectlyAssignedToUser":   "read",
	"GetUser":                          "read",
	"CountUsers":                       "read",
	"GetUserEncryptionKey":             "read",
	"ListApiTokensForUser":             "read",
	"CountApiTokensForUser":            "read",
	"GetApiTokenForAuth":               "read",

	"GetUserByEmail":                         "read",
	"GetUserSessionState":                    "read",
	"ListUsers":                              "read",
	"ListUserPermissions":                    "read",
	"ListUserScopedGrants":                   "read",
	"ListUserRoleGrants":                     "read",
	"ListUserGroupRoleGrants":                "read",
	"ListInheritedRolesForUser":              "read",
	"ListUserGroupIDsForUser":                "read",
	"GetUserGroupView":                       "read",
	"ListUserGroups":                         "read",
	"CountUserGroups":                        "read",
	"ListUserGroupsForUser":                  "read",
	"ListUserGroupMembers":                   "read",
	"ListUsersForDynamicUserGroupEvaluation": "read",
	"ListUserSSHKeys":                        "read",
	"GetRole":                                "read",
	"GetRoleByName":                          "read",
	"ListRoles":                              "read",
	"CountRoles":                             "read",
	"CountRoleHolders":                       "read",
	"GetIdentityProvider":                    "read",
	"GetIdentityProviderBySlug":              "read",
	"ListIdentityProviders":                  "read",
	"ListEnabledIdentityProviders":           "read",
	"CountIdentityProviders":                 "read",
	"GetIdentityLink":                        "read",
	"ListIdentityLinksForUser":               "read",
	"IsTokenRevoked":                         "read",
	"GetServerSettings":                      "read",
	"CountLiveBootstrapAdminTokens":          "read",
	"Search":                                 "read",

	"ListSCIMUsers":                    "read",
	"CountSCIMUsers":                   "read",
	"FindSCIMUserByEmail":              "read",
	"FindSCIMUserByExternalID":         "read",
	"GetIdentityLinkByProviderAndUser": "read",
	"CountIdentityLinksForUser":        "read",
	"GetUserGroup":                     "read",
	"ListUserGroupMemberIDs":           "read",
	"GetSCIMGroupMapping":              "read",
	"GetSCIMGroupMappingByUserGroup":   "read",
	"ListSCIMGroupMappings":            "read",
}

var forbiddenExports = map[string]string{
	"Queries":     "would hand out the generated mutation surface",
	"Pool":        "would hand out the connection pool",
	"TestingPool": "would hand out the connection pool",
	"DB":          "would hand out the connection pool",
	"Conn":        "would hand out a raw connection",
	"Exec":        "would allow arbitrary statements",
	"Query":       "would allow arbitrary statements",
	"QueryRow":    "would allow arbitrary statements",
	"WithTx":      "would be an unaudited transaction",
	"Begin":       "would be an unaudited transaction",
	"Repos":       "would hand out repositories built on the raw handle",
	"SetRepos":    "would hand out repositories built on the raw handle",
}

func exportedStoreMethods(t *testing.T) []string {
	t.Helper()
	typ := reflect.TypeOf(&store.Store{})
	names := make([]string, 0, typ.NumMethod())
	for i := 0; i < typ.NumMethod(); i++ {
		names = append(names, typ.Method(i).Name)
	}
	sort.Strings(names)

	require.NotEmpty(t, names, "no exported Store methods were enumerated; the reflection is mis-scoped")
	return names
}

func TestStoreAPI_OnlyAuditedPrimitivesCanMutate(t *testing.T) {
	var unclassified []string
	for _, name := range exportedStoreMethods(t) {
		if _, ok := forbiddenExports[name]; ok {
			t.Errorf("Store.%s is an unaudited door into the database: %s", name, forbiddenExports[name])
			continue
		}
		_, mutating := mutationCapableExports[name]
		_, readOnly := nonMutatingExports[name]
		switch {
		case mutating && readOnly:
			t.Errorf("Store.%s is classified both ways", name)
		case !mutating && !readOnly:
			unclassified = append(unclassified, name)
		}
	}
	assert.Empty(t, unclassified,
		"every exported Store method must be classified as audited-mutating or read-only; "+
			"if one of these writes state it needs to go through WithAudit, and if it does not it belongs in nonMutatingExports: %v",
		unclassified)
}

func TestStoreAPI_ClassificationHasNoStaleEntries(t *testing.T) {
	present := map[string]bool{}
	for _, name := range exportedStoreMethods(t) {
		present[name] = true
	}

	var stale []string
	for name := range mutationCapableExports {
		if !present[name] {
			stale = append(stale, "mutationCapableExports: "+name)
		}
	}
	for name := range nonMutatingExports {
		if !present[name] {
			stale = append(stale, "nonMutatingExports: "+name)
		}
	}
	sort.Strings(stale)
	assert.Empty(t, stale, "the API classification names methods that no longer exist: %v", stale)
}

func TestStoreAPI_HasNoExportedFields(t *testing.T) {
	typ := reflect.TypeOf(store.Store{})
	require.Positive(t, typ.NumField(), "matches-zero guard: Store has no fields to inspect")

	var exported []string
	for i := 0; i < typ.NumField(); i++ {
		if f := typ.Field(i); f.IsExported() {
			exported = append(exported, f.Name)
		}
	}
	assert.Empty(t, exported, "Store must expose no field; these bypass the audited door: %v", exported)
}
