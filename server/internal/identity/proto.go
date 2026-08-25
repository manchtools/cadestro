package identity

import (
	"encoding/json"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/store"
)

// Wire conversion. Nothing here reads the database; a caller assembles
// the rows it wants on the response and passes them in, so the shape of
// a response is visible at its handler rather than hidden behind a
// loader.

func timestamp(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}

func timestampValue(t time.Time) *timestamppb.Timestamp {
	return timestamppb.New(t)
}

// userView is everything a User message can carry. A handler fills only
// the parts it read, so a list response is not silently N+1 queries.
type userView struct {
	Row            store.UserRow
	SSHKeys        []store.UserSSHKeyRow
	IdentityLinks  []store.IdentityLinkWithProviderRow
	RoleGrants     []store.RoleGrantRow
	InheritedRoles []store.InheritedRoleRow
	// ScopeNames resolves a grant's scope id to a display name. A scope
	// whose group was deleted is simply absent, and its name renders
	// empty rather than the response failing.
	ScopeNames map[string]string
}

func userToProto(v userView) *cadestrov1.User {
	u := &cadestrov1.User{
		Id:                      &cadestrov1.UserId{Value: v.Row.ID},
		Email:                   v.Row.Email,
		CreatedAt:               timestamp(v.Row.CreatedAt),
		LastLoginAt:             timestamp(v.Row.LastLoginAt),
		Disabled:                v.Row.Disabled,
		DisplayName:             v.Row.DisplayName,
		GivenName:               v.Row.GivenName,
		FamilyName:              v.Row.FamilyName,
		PreferredUsername:       v.Row.PreferredUsername,
		Picture:                 v.Row.Picture,
		Locale:                  v.Row.Locale,
		LinuxUsername:           v.Row.LinuxUsername,
		LinuxUid:                v.Row.LinuxUid,
		SshAccessEnabled:        v.Row.SshAccessEnabled,
		SshAllowPubkey:          v.Row.SshAllowPubkey,
		SshAllowPassword:        v.Row.SshAllowPassword,
		UserProvisioningEnabled: v.Row.UserProvisioningEnabled,
	}
	for _, k := range v.SSHKeys {
		u.SshPublicKeys = append(u.SshPublicKeys, sshKeyToProto(k))
	}
	for _, l := range v.IdentityLinks {
		u.IdentityLinks = append(u.IdentityLinks, linkToProto(l))
	}
	for _, g := range v.RoleGrants {
		u.RoleGrants = append(u.RoleGrants, grantToProto(g, v.ScopeNames))
	}
	for _, r := range v.InheritedRoles {
		u.InheritedRoles = append(u.InheritedRoles, &cadestrov1.InheritedRole{
			RoleId:    &cadestrov1.RoleId{Value: r.RoleID},
			RoleName:  r.RoleName,
			GroupId:   &cadestrov1.UserGroupId{Value: r.GroupID},
			GroupName: r.GroupName,
		})
	}
	return u
}

func sshKeyToProto(k store.UserSSHKeyRow) *cadestrov1.SshPublicKey {
	out := &cadestrov1.SshPublicKey{Id: &cadestrov1.SshKeyId{Value: k.KeyID}, AddedAt: timestampValue(k.AddedAt)}
	if k.PublicKey != nil {
		out.PublicKey = *k.PublicKey
	}
	if k.Comment != nil {
		out.Comment = *k.Comment
	}
	return out
}

func roleToProto(r store.RoleRow) *cadestrov1.Role {
	return &cadestrov1.Role{
		Id:          &cadestrov1.RoleId{Value: r.ID},
		Name:        r.Name,
		Description: r.Description,
		Permissions: append([]string(nil), r.Permissions...),
		CreatedAt:   timestampValue(r.CreatedAt),
		IsSystem:    r.IsSystem,
	}
}

func userGroupToProto(row store.UserGroupView, grants []store.GroupRoleGrantRow) (*cadestrov1.UserGroup, error) {
	group := &cadestrov1.UserGroup{
		Id: &cadestrov1.UserGroupId{Value: row.ID}, Name: row.Name, Description: row.Description,
		MemberCount: boundedIdentityCount(row.LiveMemberCount),
		CreatedAt:   timestampValue(row.CreatedAt), IsDynamic: row.IsDynamic,
		IsScimManaged: row.IsScimManaged,
	}
	if row.DynamicQuery != nil {
		group.DynamicQuery = *row.DynamicQuery
	}
	if len(row.MaintenanceWindow) > 0 && string(row.MaintenanceWindow) != "{}" {
		window := &cadestrov1.MaintenanceWindow{}
		if err := protojson.Unmarshal(row.MaintenanceWindow, window); err != nil {
			return nil, err
		}
		if len(window.Schedule) > 0 {
			group.MaintenanceWindow = window
		}
	}
	for _, grant := range grants {
		wire := &cadestrov1.RoleGrant{
			Role: roleToProto(grant.Role), ScopeKind: scopeKindToProto(grant.ScopeKind),
		}
		if grant.ScopeID != nil {
			wire.ScopeId = &cadestrov1.ScopeId{Value: *grant.ScopeID}
		}
		group.RoleGrants = append(group.RoleGrants, wire)
	}
	return group, nil
}

func grantToProto(g store.RoleGrantRow, scopeNames map[string]string) *cadestrov1.RoleGrant {
	out := &cadestrov1.RoleGrant{
		Role:      roleToProto(g.Role),
		ScopeKind: scopeKindToProto(g.ScopeKind),
	}
	if g.ScopeID != nil {
		out.ScopeId = &cadestrov1.ScopeId{Value: *g.ScopeID}
		out.ScopeName = scopeNames[*g.ScopeID]
	}
	return out
}

// scopeKindToProto maps the stored scope discriminator onto the wire
// enum. A nil discriminator is an unscoped grant.
func scopeKindToProto(kind *string) cadestrov1.RoleGrantScopeKind {
	if kind == nil {
		return cadestrov1.RoleGrantScopeKind_ROLE_GRANT_SCOPE_KIND_UNSPECIFIED
	}
	switch *kind {
	case auth.ScopeKindDeviceGroup:
		return cadestrov1.RoleGrantScopeKind_ROLE_GRANT_SCOPE_KIND_DEVICE_GROUP
	case auth.ScopeKindUserGroup:
		return cadestrov1.RoleGrantScopeKind_ROLE_GRANT_SCOPE_KIND_USER_GROUP
	default:
		return cadestrov1.RoleGrantScopeKind_ROLE_GRANT_SCOPE_KIND_UNSPECIFIED
	}
}

// scopeKindFromProto maps a wire scope kind onto the stored
// discriminator. An unrecognised kind is reported as invalid rather
// than silently becoming an unscoped (fleet-wide) grant.
func scopeKindFromProto(kind cadestrov1.RoleGrantScopeKind) (string, bool) {
	switch kind {
	case cadestrov1.RoleGrantScopeKind_ROLE_GRANT_SCOPE_KIND_DEVICE_GROUP:
		return auth.ScopeKindDeviceGroup, true
	case cadestrov1.RoleGrantScopeKind_ROLE_GRANT_SCOPE_KIND_USER_GROUP:
		return auth.ScopeKindUserGroup, true
	default:
		return "", false
	}
}

func targetKindToProto(k auth.PermissionTargetKind) cadestrov1.PermissionTargetKind {
	switch k {
	case auth.TargetDevice:
		return cadestrov1.PermissionTargetKind_PERMISSION_TARGET_KIND_DEVICE
	case auth.TargetUser:
		return cadestrov1.PermissionTargetKind_PERMISSION_TARGET_KIND_USER
	default:
		return cadestrov1.PermissionTargetKind_PERMISSION_TARGET_KIND_UNSPECIFIED
	}
}

func linkToProto(l store.IdentityLinkWithProviderRow) *cadestrov1.IdentityLink {
	return &cadestrov1.IdentityLink{
		Id:            &cadestrov1.IdentityLinkId{Value: l.ID},
		UserId:        &cadestrov1.UserId{Value: l.UserID},
		ProviderId:    &cadestrov1.IdentityProviderId{Value: l.ProviderID},
		ProviderName:  l.ProviderName,
		ProviderSlug:  l.ProviderSlug,
		ExternalId:    &cadestrov1.ExternalIdentityId{Value: l.ExternalID},
		ExternalEmail: l.ExternalEmail,
		ExternalName:  l.ExternalName,
		LinkedAt:      timestampValue(l.LinkedAt),
		LastLoginAt:   timestamp(l.LastLoginAt),
	}
}

// providerToProto renders a provider for the wire.
//
// client_secret_encrypted has no wire field and scim_token_hash has no
// wire field: the secret is write-only by contract and the token is
// shown exactly once, at the moment it is minted.
func (h *Handlers) providerToProto(p store.IdentityProviderRow) *cadestrov1.IdentityProvider {
	out := &cadestrov1.IdentityProvider{
		Id:                   &cadestrov1.IdentityProviderId{Value: p.ID},
		Name:                 p.Name,
		Slug:                 p.Slug,
		ProviderType:         providerTypeToProto(p.ProviderType),
		Enabled:              p.Enabled,
		ClientId:             &cadestrov1.OidcClientId{Value: p.ClientID},
		IssuerUrl:            p.IssuerUrl,
		AuthorizationUrl:     p.AuthorizationUrl,
		TokenUrl:             p.TokenUrl,
		UserinfoUrl:          p.UserinfoUrl,
		Scopes:               append([]string(nil), p.Scopes...),
		AutoCreateUsers:      p.AutoCreateUsers,
		AutoLinkByEmail:      p.AutoLinkByEmail,
		TrustEmailAssertions: p.TrustEmailAssertions,
		DefaultRoleId:        &cadestrov1.RoleId{Value: p.DefaultRoleID},
		GroupClaim:           p.GroupClaim,
		GroupMapping:         idpGroupMapping(p.GroupMapping),
		CreatedAt:            timestampValue(p.CreatedAt),
		UpdatedAt:            timestampValue(p.UpdatedAt),
		ScimEnabled:          p.ScimEnabled,
	}
	if p.ScimEnabled {
		out.ScimEndpointUrl = h.scimEndpointURL(p.ID)
	}
	return out
}

const providerTypeOIDC = "oidc"

func providerTypeToProto(t string) cadestrov1.IdentityProviderType {
	if t == providerTypeOIDC {
		return cadestrov1.IdentityProviderType_IDENTITY_PROVIDER_TYPE_OIDC
	}
	return cadestrov1.IdentityProviderType_IDENTITY_PROVIDER_TYPE_UNSPECIFIED
}

func providerTypeFromProto(t cadestrov1.IdentityProviderType) (string, bool) {
	if t == cadestrov1.IdentityProviderType_IDENTITY_PROVIDER_TYPE_OIDC {
		return providerTypeOIDC, true
	}
	return "", false
}

func (h *Handlers) scimEndpointURL(providerID string) string {
	return strings.TrimSuffix(h.baseURL, "/") + "/scim/v2/" + providerID
}

func permissionsToProto() []*cadestrov1.PermissionInfo {
	all := auth.AllPermissions()
	out := make([]*cadestrov1.PermissionInfo, 0, len(all))
	for _, p := range all {
		out = append(out, &cadestrov1.PermissionInfo{
			Key:         p.Key,
			Group:       p.Group,
			Description: p.Description,
			TargetKind:  targetKindToProto(p.TargetKind),
		})
	}
	return out
}

// idpGroupMapping decodes the provider's stored external-group map. A
// mapping that will not decode renders as absent rather than failing
// the read: it is display metadata, and the authorization it feeds is
// re-derived from the same bytes at login.
func idpGroupMapping(raw []byte) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}
