package identity_test

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/manchtools/cadestro/server/internal/identity"
)

type call func(f *fixture, token string) error

var authenticatedMutations = map[string]call{
	cadestrov1connect.ControlServiceCreateIdentityProviderProcedure: func(f *fixture, token string) error {
		_, err := f.client.CreateIdentityProvider(f.ctx(), authed(&cadestrov1.CreateIdentityProviderRequest{
			Name:         "Corp",
			Slug:         "corp",
			ProviderType: cadestrov1.IdentityProviderType_IDENTITY_PROVIDER_TYPE_OIDC,
			ClientId:     &cadestrov1.OidcClientId{Value: "client"},
			ClientSecret: "secret",
			IssuerUrl:    "https://idp.example/",
		}, token))
		return err
	},
	cadestrov1connect.ControlServiceUpdateIdentityProviderProcedure: func(f *fixture, token string) error {
		_, err := f.client.UpdateIdentityProvider(f.ctx(), authed(&cadestrov1.UpdateIdentityProviderRequest{
			Id:   &cadestrov1.IdentityProviderId{Value: newULID()},
			Name: "Corp",
		}, token))
		return err
	},
	cadestrov1connect.ControlServiceDeleteIdentityProviderProcedure: func(f *fixture, token string) error {
		_, err := f.client.DeleteIdentityProvider(f.ctx(), authed(&cadestrov1.DeleteIdentityProviderRequest{Id: &cadestrov1.IdentityProviderId{Value: newULID()}}, token))
		return err
	},
	cadestrov1connect.ControlServiceEnableSCIMProcedure: func(f *fixture, token string) error {
		_, err := f.client.EnableSCIM(f.ctx(), authed(&cadestrov1.EnableSCIMRequest{Id: &cadestrov1.IdentityProviderId{Value: newULID()}}, token))
		return err
	},
	cadestrov1connect.ControlServiceDisableSCIMProcedure: func(f *fixture, token string) error {
		_, err := f.client.DisableSCIM(f.ctx(), authed(&cadestrov1.DisableSCIMRequest{Id: &cadestrov1.IdentityProviderId{Value: newULID()}}, token))
		return err
	},
	cadestrov1connect.ControlServiceRotateSCIMTokenProcedure: func(f *fixture, token string) error {
		_, err := f.client.RotateSCIMToken(f.ctx(), authed(&cadestrov1.RotateSCIMTokenRequest{Id: &cadestrov1.IdentityProviderId{Value: newULID()}}, token))
		return err
	},
	cadestrov1connect.ControlServiceUnlinkIdentityProcedure: func(f *fixture, token string) error {
		_, err := f.client.UnlinkIdentity(f.ctx(), authed(&cadestrov1.UnlinkIdentityRequest{LinkId: &cadestrov1.IdentityLinkId{Value: newULID()}}, token))
		return err
	},
	cadestrov1connect.ControlServiceEraseJITUserProcedure: func(f *fixture, token string) error {
		_, err := f.client.EraseJITUser(f.ctx(), authed(&cadestrov1.EraseJITUserRequest{Id: &cadestrov1.UserId{Value: newULID()}}, token))
		return err
	},
	cadestrov1connect.ControlServiceUpdateUserEmailProcedure: func(f *fixture, token string) error {
		_, err := f.client.UpdateUserEmail(f.ctx(), authed(&cadestrov1.UpdateUserEmailRequest{
			Id: &cadestrov1.UserId{Value: newULID()}, Email: "moved@test.example",
		}, token))
		return err
	},
	cadestrov1connect.ControlServiceSetUserDisabledProcedure: func(f *fixture, token string) error {
		_, err := f.client.SetUserDisabled(f.ctx(), authed(&cadestrov1.SetUserDisabledRequest{Id: &cadestrov1.UserId{Value: newULID()}, Disabled: true}, token))
		return err
	},
	cadestrov1connect.ControlServiceUpdateUserProfileProcedure: func(f *fixture, token string) error {
		_, err := f.client.UpdateUserProfile(f.ctx(), authed(&cadestrov1.UpdateUserProfileRequest{
			Id: &cadestrov1.UserId{Value: newULID()}, DisplayName: "Name",
		}, token))
		return err
	},
	cadestrov1connect.ControlServiceUpdateUserLinuxUsernameProcedure: func(f *fixture, token string) error {
		_, err := f.client.UpdateUserLinuxUsername(f.ctx(), authed(&cadestrov1.UpdateUserLinuxUsernameRequest{
			UserId: &cadestrov1.UserId{Value: newULID()}, LinuxUsername: "alice",
		}, token))
		return err
	},
	cadestrov1connect.ControlServiceUpdateUserSshSettingsProcedure: func(f *fixture, token string) error {
		_, err := f.client.UpdateUserSshSettings(f.ctx(), authed(&cadestrov1.UpdateUserSshSettingsRequest{
			UserId: &cadestrov1.UserId{Value: newULID()}, SshAccessEnabled: true,
		}, token))
		return err
	},
	cadestrov1connect.ControlServiceAddUserSshKeyProcedure: func(f *fixture, token string) error {
		_, err := f.client.AddUserSshKey(f.ctx(), authed(&cadestrov1.AddUserSshKeyRequest{
			UserId: &cadestrov1.UserId{Value: newULID()}, PublicKey: testSSHKey,
		}, token))
		return err
	},
	cadestrov1connect.ControlServiceRemoveUserSshKeyProcedure: func(f *fixture, token string) error {
		_, err := f.client.RemoveUserSshKey(f.ctx(), authed(&cadestrov1.RemoveUserSshKeyRequest{
			UserId: &cadestrov1.UserId{Value: newULID()}, KeyId: &cadestrov1.SshKeyId{Value: newULID()},
		}, token))
		return err
	},
	cadestrov1connect.ControlServiceSetUserProvisioningEnabledProcedure: func(f *fixture, token string) error {
		_, err := f.client.SetUserProvisioningEnabled(f.ctx(), authed(&cadestrov1.SetUserProvisioningEnabledRequest{
			UserId: &cadestrov1.UserId{Value: newULID()}, Enabled: true,
		}, token))
		return err
	},
	cadestrov1connect.ControlServiceCreateUserGroupProcedure: func(f *fixture, token string) error {
		_, err := f.client.CreateUserGroup(f.ctx(), authed(&cadestrov1.CreateUserGroupRequest{Name: "Operators"}, token))
		return err
	},
	cadestrov1connect.ControlServiceUpdateUserGroupProcedure: func(f *fixture, token string) error {
		_, err := f.client.UpdateUserGroup(f.ctx(), authed(&cadestrov1.UpdateUserGroupRequest{
			GroupId: &cadestrov1.UserGroupId{Value: newULID()}, Name: "Operators",
		}, token))
		return err
	},
	cadestrov1connect.ControlServiceDeleteUserGroupProcedure: func(f *fixture, token string) error {
		_, err := f.client.DeleteUserGroup(f.ctx(), authed(&cadestrov1.DeleteUserGroupRequest{Id: &cadestrov1.UserGroupId{Value: newULID()}}, token))
		return err
	},
	cadestrov1connect.ControlServiceAddUserToGroupProcedure: func(f *fixture, token string) error {
		_, err := f.client.AddUserToGroup(f.ctx(), authed(&cadestrov1.AddUserToGroupRequest{
			GroupId: &cadestrov1.UserGroupId{Value: newULID()}, UserId: &cadestrov1.UserId{Value: newULID()},
		}, token))
		return err
	},
	cadestrov1connect.ControlServiceRemoveUserFromGroupProcedure: func(f *fixture, token string) error {
		_, err := f.client.RemoveUserFromGroup(f.ctx(), authed(&cadestrov1.RemoveUserFromGroupRequest{
			GroupId: &cadestrov1.UserGroupId{Value: newULID()}, UserId: &cadestrov1.UserId{Value: newULID()},
		}, token))
		return err
	},
	cadestrov1connect.ControlServiceSetUserGroupMaintenanceWindowProcedure: func(f *fixture, token string) error {
		_, err := f.client.SetUserGroupMaintenanceWindow(f.ctx(), authed(&cadestrov1.SetUserGroupMaintenanceWindowRequest{
			Id: &cadestrov1.UserGroupId{Value: newULID()},
		}, token))
		return err
	},
	cadestrov1connect.ControlServiceUpdateUserGroupQueryProcedure: func(f *fixture, token string) error {
		_, err := f.client.UpdateUserGroupQuery(f.ctx(), authed(&cadestrov1.UpdateUserGroupQueryRequest{
			Id: &cadestrov1.UserGroupId{Value: newULID()}, DynamicQuery: stringPtr(`user.disabled == true`),
		}, token))
		return err
	},
	cadestrov1connect.ControlServiceEvaluateDynamicUserGroupProcedure: func(f *fixture, token string) error {
		_, err := f.client.EvaluateDynamicUserGroup(f.ctx(), authed(&cadestrov1.EvaluateDynamicUserGroupRequest{
			Id: &cadestrov1.UserGroupId{Value: newULID()},
		}, token))
		return err
	},
	cadestrov1connect.ControlServiceUpdateServerSettingsProcedure: func(f *fixture, token string) error {
		_, err := f.client.UpdateServerSettings(f.ctx(), authed(&cadestrov1.UpdateServerSettingsRequest{
			UserProvisioningEnabled: true,
		}, token))
		return err
	},
	cadestrov1connect.ControlServiceCreateRoleProcedure: func(f *fixture, token string) error {
		_, err := f.client.CreateRole(f.ctx(), authed(&cadestrov1.CreateRoleRequest{
			Name: "Auditors", Permissions: []string{"ListUsers"},
		}, token))
		return err
	},
	cadestrov1connect.ControlServiceUpdateRoleProcedure: func(f *fixture, token string) error {
		_, err := f.client.UpdateRole(f.ctx(), authed(&cadestrov1.UpdateRoleRequest{
			RoleId: &cadestrov1.RoleId{Value: newULID()}, Name: "Auditors", Permissions: []string{"ListUsers"},
		}, token))
		return err
	},
	cadestrov1connect.ControlServiceDeleteRoleProcedure: func(f *fixture, token string) error {
		_, err := f.client.DeleteRole(f.ctx(), authed(&cadestrov1.DeleteRoleRequest{Id: &cadestrov1.RoleId{Value: newULID()}}, token))
		return err
	},
	cadestrov1connect.ControlServiceAssignRoleToUserProcedure: func(f *fixture, token string) error {
		_, err := f.client.AssignRoleToUser(f.ctx(), authed(&cadestrov1.AssignRoleToUserRequest{
			UserId: &cadestrov1.UserId{Value: newULID()}, RoleId: &cadestrov1.RoleId{Value: newULID()},
		}, token))
		return err
	},
	cadestrov1connect.ControlServiceRevokeRoleFromUserProcedure: func(f *fixture, token string) error {
		_, err := f.client.RevokeRoleFromUser(f.ctx(), authed(&cadestrov1.RevokeRoleFromUserRequest{
			UserId: &cadestrov1.UserId{Value: newULID()}, RoleId: &cadestrov1.RoleId{Value: newULID()},
		}, token))
		return err
	},
	cadestrov1connect.ControlServiceAssignRoleToUserGroupProcedure: func(f *fixture, token string) error {
		_, err := f.client.AssignRoleToUserGroup(f.ctx(), authed(&cadestrov1.AssignRoleToUserGroupRequest{
			GroupId: &cadestrov1.UserGroupId{Value: newULID()}, RoleId: &cadestrov1.RoleId{Value: newULID()},
		}, token))
		return err
	},
	cadestrov1connect.ControlServiceRevokeRoleFromUserGroupProcedure: func(f *fixture, token string) error {
		_, err := f.client.RevokeRoleFromUserGroup(f.ctx(), authed(&cadestrov1.RevokeRoleFromUserGroupRequest{
			GroupId: &cadestrov1.UserGroupId{Value: newULID()}, RoleId: &cadestrov1.RoleId{Value: newULID()},
		}, token))
		return err
	},
}

var publicMutations = map[string]bool{
	cadestrov1connect.ControlServiceRefreshTokenProcedure:   true,
	cadestrov1connect.ControlServiceLogoutProcedure:         true,
	cadestrov1connect.ControlServiceGetSSOLoginURLProcedure: true,
	cadestrov1connect.ControlServiceSSOCallbackProcedure:    true,
}

const testSSHKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIB7vTk4h/lPJHZ0k5R0VpZ5cV5hFB0j2m3wKuHbLQ9pR test@example"

func TestMutationMatrix_CoversEveryMutationProcedure(t *testing.T) {
	t.Parallel()
	procedures := identity.MutationProcedures()
	require.NotEmpty(t, procedures, "no mutation procedures were enumerated; the matrix would pass vacuously")

	var uncovered []string
	for _, p := range procedures {
		if publicMutations[p] {
			continue
		}
		if _, ok := authenticatedMutations[p]; !ok {
			uncovered = append(uncovered, p)
		}
	}
	assert.Empty(t, uncovered,
		"these mutation procedures have no credential/authorization case; add one to authenticatedMutations or classify them in publicMutations: %v",
		uncovered)

	var stale []string
	known := make(map[string]bool, len(procedures))
	for _, p := range procedures {
		known[p] = true
	}
	for p := range authenticatedMutations {
		if !known[p] {
			stale = append(stale, p)
		}
	}
	for p := range publicMutations {
		if !known[p] {
			stale = append(stale, p)
		}
	}
	assert.Empty(t, stale, "the matrix names procedures that are not mutations: %v", stale)

	for _, retired := range []string{
		"/cadestro.v1.ControlService/BeginCLILogin",
		"/cadestro.v1.ControlService/ExchangeCLISession",
	} {
		assert.NotContains(t, publicMutations, retired,
			"%s is exempted from the mutation matrix but no longer exists", retired)
		assert.NotContains(t, authenticatedMutations, retired,
			"%s has a matrix case but no longer exists", retired)
	}
}

func TestProcedureClassification_MatchesTheMountedSurface(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	require.NotEmpty(t, f.mounted, "nothing was mounted; the surface assertions would pass vacuously")

	classified := make(map[string]string, len(f.mounted))
	for _, p := range identity.MutationProcedures() {
		classified[p] = "mutation"
	}
	for _, p := range identity.ReadProcedures() {
		if prior, dup := classified[p]; dup {
			t.Errorf("procedure %s is classified both as %s and as a read", p, prior)
		}
		classified[p] = "read"
	}
	for _, p := range identity.SensitiveReadProcedures() {
		if prior, dup := classified[p]; dup {
			t.Errorf("procedure %s is classified both as %s and as a sensitive read", p, prior)
		}
		classified[p] = "sensitive read"
	}

	mountedSet := make(map[string]bool, len(f.mounted))
	for _, p := range f.mounted {
		mountedSet[p] = true
		if _, ok := classified[p]; !ok {
			t.Errorf("mounted procedure %s is not classified", p)
		}
	}
	for p := range classified {
		assert.True(t, mountedSet[p], "procedure %s is classified but never mounted", p)
	}
}

func TestMutations_RejectMissingCredential(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	for procedure, do := range authenticatedMutations {
		t.Run(shortName(procedure), func(t *testing.T) {
			err := do(f, "")
			assert.Equal(t, connect.CodeUnauthenticated, connectCodeOf(t, err))
		})
	}
}

func TestMutations_RejectExpiredCredential(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	subject := f.seedSubject()
	expired := f.expiredToken(subject.ID, subject.Email)

	for procedure, do := range authenticatedMutations {
		t.Run(shortName(procedure), func(t *testing.T) {
			err := do(f, expired)
			assert.Equal(t, connect.CodeUnauthenticated, connectCodeOf(t, err))
		})
	}
}

func TestMutations_RejectForgedSignature(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	subject := f.seedSubject()
	forged := f.forgedToken(subject.ID, subject.Email, allPermissionKeys())

	for procedure, do := range authenticatedMutations {
		t.Run(shortName(procedure), func(t *testing.T) {
			err := do(f, forged)
			assert.Equal(t, connect.CodeUnauthenticated, connectCodeOf(t, err),
				"a token signed by an unknown key must never authenticate")
		})
	}
}

func TestMutations_RejectRefreshTokenUsedAsAccessToken(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	actor := f.seedActor(grant{Permissions: allPermissionKeys()})
	pair := f.mintPair(actor.ID, actor.Email)

	for procedure, do := range authenticatedMutations {
		t.Run(shortName(procedure), func(t *testing.T) {
			err := do(f, pair.RefreshToken)
			assert.Equal(t, connect.CodeUnauthenticated, connectCodeOf(t, err))
		})
	}
}

func TestMutations_RejectUnauthorizedActor(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	powerless := f.seedActor(grant{Permissions: []string{"GetCurrentUser"}})

	for procedure, do := range authenticatedMutations {
		t.Run(shortName(procedure), func(t *testing.T) {
			err := do(f, powerless.Token)
			assert.Equal(t, connect.CodePermissionDenied, connectCodeOf(t, err))
		})
	}
}

func TestMutations_RejectedRequestsCommitNoMutation(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	subject := f.seedSubject()
	forged := f.forgedToken(subject.ID, subject.Email, allPermissionKeys())

	for _, do := range authenticatedMutations {
		require.Error(t, do(f, forged))
	}

	rows, err := f.raw.Query(f.ctx(), `SELECT DISTINCT operation_class FROM audit_operations`)
	require.NoError(t, err)
	defer rows.Close()
	var classes []string
	for rows.Next() {
		var c string
		require.NoError(t, rows.Scan(&c))
		classes = append(classes, c)
	}
	require.NoError(t, rows.Err())
	require.NotEmpty(t, classes, "no audit rows at all; the rejection path recorded nothing")
	assert.Equal(t, []string{"REJECTED_AUTHENTICATION"}, classes,
		"a rejected request must record only its rejection, never a mutation")
}

func shortName(procedure string) string {
	for i := len(procedure) - 1; i >= 0; i-- {
		if procedure[i] == '/' {
			return procedure[i+1:]
		}
	}
	return procedure
}
