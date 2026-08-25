package identity_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/identity"
)

func TestGatedPermissions_AreAllRegistered(t *testing.T) {
	t.Parallel()
	gated := identity.GatedPermissions()
	require.NotEmpty(t, gated, "no gated permissions were enumerated; this test would pass vacuously")

	registered := auth.ValidPermissionKeys()
	require.NotEmpty(t, registered)
	for _, key := range gated {
		assert.True(t, registered[key], "handler gates on %q, which is not a registered permission", key)
	}
}

func TestPrivilegeGrantingPermissions_AreNeverScopable(t *testing.T) {
	t.Parallel()
	all := auth.AllPermissions()
	require.NotEmpty(t, all)

	var privileged int
	for _, p := range all {
		if !p.PrivilegeGranting {
			continue
		}
		privileged++
		assert.Equal(t, auth.TargetUnspecified, p.TargetKind,
			"%s can grant or widen privilege, so it must declare no scopable target kind", p.Key)
		assert.True(t, auth.IsPrivilegeGranting(p.Key))
	}
	require.Positive(t, privileged,
		"the registry marks nothing privilege-granting; the global-only rule would be unenforced")
}

func TestUnknownPermission_IsTreatedAsPrivilegeGranting(t *testing.T) {
	t.Parallel()
	assert.True(t, auth.IsPrivilegeGranting("SomePermissionAddedTomorrow"),
		"refusing to scope what the registry cannot classify is the fail-closed answer")

	offender, found := auth.FirstPrivilegeGranting([]string{"ListUsers", "CreateRole"})
	assert.True(t, found)
	assert.Equal(t, "CreateRole", offender)

	_, found = auth.FirstPrivilegeGranting([]string{"ListUsers", "GetUser"})
	assert.False(t, found, "an ordinary read-only role is scopable")
}

func TestRegistry_HasNoLocalCredentialPermissions(t *testing.T) {
	t.Parallel()
	all := auth.AllPermissions()
	require.NotEmpty(t, all)

	forbidden := map[string]bool{
		"UpdateUserPassword":      true,
		"UpdateUserPassword:self": true,
		"SetupTOTP":               true,
		"VerifyTOTP":              true,
		"DisableTOTP":             true,
		"GetTOTPStatus":           true,
		"RegenerateBackupCodes":   true,
		"AdminDisableUserTOTP":    true,
		"Login":                   true,
		"VerifyLoginTOTP":         true,
	}
	for _, p := range all {
		assert.False(t, forbidden[p.Key], "%s is a removed local-credential permission", p.Key)
	}
}

func TestPublicProcedures_AreExactlyTheUnauthenticatedSurface(t *testing.T) {
	t.Parallel()
	expected := map[string]bool{
		"/cadestro.v1.ControlService/RefreshToken":     true,
		"/cadestro.v1.ControlService/Logout":           true,
		"/cadestro.v1.ControlService/Register":         true,
		"/cadestro.v1.ControlService/RenewCertificate": true,
		"/cadestro.v1.ControlService/ListAuthMethods":  true,
		"/cadestro.v1.ControlService/GetSSOLoginURL":   true,
		"/cadestro.v1.ControlService/SSOCallback":      true,
	}
	require.Len(t, auth.PublicProcedures, len(expected),
		"the unauthenticated surface changed size; that is a deliberate act that must be reviewed")
	for procedure := range auth.PublicProcedures {
		assert.True(t, expected[procedure], "%s was made public", procedure)
	}

	for procedure := range auth.PublicProcedures {
		assert.NotContains(t, procedure, "Login/")
		assert.NotContains(t, procedure, "TOTP")
	}

	for _, removed := range []string{
		"/cadestro.v1.ControlService/BeginCLILogin",
		"/cadestro.v1.ControlService/ExchangeCLISession",
	} {
		assert.NotContains(t, auth.PublicProcedures, removed,
			"%s no longer exists on the contract; a public entry for it would gate nothing", removed)
	}
}

func TestMountedProcedures_UseTheCanonicalContractPaths(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	require.NotEmpty(t, f.mounted)
	for _, p := range f.mounted {
		assert.Contains(t, p, auth.ControlProcedurePrefix,
			"%s is not a cadestro.v1 control procedure", p)
	}
}
