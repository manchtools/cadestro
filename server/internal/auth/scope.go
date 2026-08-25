package auth

import (
	"context"
	"errors"

	"connectrpc.com/connect"
)

const (
	ScopeKindDeviceGroup = "device_group"
	ScopeKindUserGroup   = "user_group"
)

type ScopedGrant struct {
	Permission string `json:"p"`
	ScopeKind  string `json:"k,omitempty"`
	ScopeID    string `json:"i,omitempty"`
}

type ScopeFilter struct {
	Global   bool
	GroupIDs []string
}

type ScopeResolver interface {
	DeviceGroupsForDevice(ctx context.Context, deviceID string) ([]string, error)

	UserGroupsForUser(ctx context.Context, userID string) ([]string, error)
}

func scopeFilterFor(ctx context.Context, permission, wantKind string) ScopeFilter {
	user, ok := UserFromContext(ctx)
	if !ok {
		return ScopeFilter{}
	}
	var groupIDs []string
	for _, g := range user.ScopedGrants {
		if g.Permission != permission {
			continue
		}
		if g.ScopeKind == "" {

			return ScopeFilter{Global: true}
		}
		if g.ScopeKind == wantKind {
			groupIDs = append(groupIDs, g.ScopeID)
		}
	}
	return ScopeFilter{Global: false, GroupIDs: groupIDs}
}

func DeviceScopeFilterFor(ctx context.Context, permission string) ScopeFilter {
	return scopeFilterFor(ctx, permission, ScopeKindDeviceGroup)
}

func UserScopeFilterFor(ctx context.Context, permission string) ScopeFilter {
	return scopeFilterFor(ctx, permission, ScopeKindUserGroup)
}

func EnforceDeviceScope(ctx context.Context, resolver ScopeResolver, permission, deviceID string) error {
	if _, ok := UserFromContext(ctx); !ok {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("not authenticated"))
	}
	f := DeviceScopeFilterFor(ctx, permission)
	if f.Global {
		return nil
	}
	if len(f.GroupIDs) == 0 {
		return connect.NewError(connect.CodePermissionDenied, errors.New("permission denied"))
	}
	deviceGroups, err := resolver.DeviceGroupsForDevice(ctx, deviceID)
	if err != nil {
		return connect.NewError(connect.CodeInternal, errors.New("scope resolution failed"))
	}
	if intersects(f.GroupIDs, deviceGroups) {
		return nil
	}
	return connect.NewError(connect.CodePermissionDenied, errors.New("permission denied"))
}

func EnforceUserScope(ctx context.Context, resolver ScopeResolver, permission, targetUserID string) error {
	if _, ok := UserFromContext(ctx); !ok {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("not authenticated"))
	}
	f := UserScopeFilterFor(ctx, permission)
	if f.Global {
		return nil
	}
	if len(f.GroupIDs) == 0 {
		return connect.NewError(connect.CodePermissionDenied, errors.New("permission denied"))
	}
	userGroups, err := resolver.UserGroupsForUser(ctx, targetUserID)
	if err != nil {
		return connect.NewError(connect.CodeInternal, errors.New("scope resolution failed"))
	}
	if intersects(f.GroupIDs, userGroups) {
		return nil
	}
	return connect.NewError(connect.CodePermissionDenied, errors.New("permission denied"))
}

const AssignRoleScopePermission = "AssignRoleScope"

func targetKindForScopeKind(scopeKind string) (PermissionTargetKind, bool) {
	switch scopeKind {
	case ScopeKindDeviceGroup:
		return TargetDevice, true
	case ScopeKindUserGroup:
		return TargetUser, true
	default:
		return TargetUnspecified, false
	}
}

func RolePermissionsScopableWith(perms []string, scopeKind string) (badPerm string, ok bool) {
	want, valid := targetKindForScopeKind(scopeKind)
	if !valid {
		return "", false
	}
	for _, p := range perms {
		if TargetKindFor(p) != want {
			return p, false
		}
	}
	return "", true
}

func EnforceGrantScopeAuthority(ctx context.Context, scopeKind, scopeID string) error {
	if _, ok := UserFromContext(ctx); !ok {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("not authenticated"))
	}
	f := scopeFilterFor(ctx, AssignRoleScopePermission, scopeKind)
	if f.Global {
		return nil
	}
	for _, id := range f.GroupIDs {
		if id == scopeID {
			return nil
		}
	}
	return connect.NewError(connect.CodePermissionDenied,
		errors.New("cannot grant a scope outside your own scope authority"))
}

func EnforceUnscopedGrantAuthority(ctx context.Context) error {
	if _, ok := UserFromContext(ctx); !ok {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("not authenticated"))
	}
	dg := DeviceScopeFilterFor(ctx, AssignRoleScopePermission)
	ug := UserScopeFilterFor(ctx, AssignRoleScopePermission)
	if dg.Global || ug.Global {
		return nil
	}
	if len(dg.GroupIDs) > 0 || len(ug.GroupIDs) > 0 {
		return connect.NewError(connect.CodePermissionDenied,
			errors.New("a scope-limited admin cannot create an unscoped grant"))
	}
	return nil
}

func EnforceUserScopeOrSelf(ctx context.Context, resolver ScopeResolver, permission, targetUserID string) error {
	user, ok := UserFromContext(ctx)
	if !ok {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("not authenticated"))
	}
	if HasPermission(ctx, permission) {

		f := UserScopeFilterFor(ctx, permission)
		if f.Global {
			return nil
		}
		if len(f.GroupIDs) > 0 {
			return EnforceUserScope(ctx, resolver, permission, targetUserID)
		}

		if hasScopedGrant(ctx, permission) {
			return connect.NewError(connect.CodePermissionDenied, errors.New("permission denied"))
		}
		return nil
	}
	if HasPermission(ctx, permission+":self") && targetUserID == user.ID {
		return nil
	}
	return connect.NewError(connect.CodePermissionDenied, errors.New("permission denied"))
}

func EnforceDeviceScopeOnBaseTier(ctx context.Context, resolver ScopeResolver, permission, deviceID string) error {
	if _, ok := UserFromContext(ctx); !ok {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("not authenticated"))
	}
	if !HasPermission(ctx, permission) {
		return nil
	}

	f := DeviceScopeFilterFor(ctx, permission)
	if f.Global {
		return nil
	}
	if len(f.GroupIDs) > 0 {
		return EnforceDeviceScope(ctx, resolver, permission, deviceID)
	}
	if hasScopedGrant(ctx, permission) {
		return connect.NewError(connect.CodePermissionDenied, errors.New("permission denied"))
	}
	return nil
}

func EnforceDeviceGroupScope(ctx context.Context, permission, groupID string) error {
	return enforceGroupScope(ctx, DeviceScopeFilterFor(ctx, permission), permission, groupID)
}

func EnforceUserGroupScope(ctx context.Context, permission, groupID string) error {
	return enforceGroupScope(ctx, UserScopeFilterFor(ctx, permission), permission, groupID)
}

func enforceGroupScope(ctx context.Context, f ScopeFilter, permission, groupID string) error {
	if _, ok := UserFromContext(ctx); !ok {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("not authenticated"))
	}
	if !HasPermission(ctx, permission) {
		return connect.NewError(connect.CodePermissionDenied, errors.New("permission denied"))
	}
	if f.Global {
		return nil
	}
	if len(f.GroupIDs) > 0 {
		for _, id := range f.GroupIDs {
			if id == groupID {
				return nil
			}
		}
		return connect.NewError(connect.CodePermissionDenied, errors.New("permission denied"))
	}

	if hasScopedGrant(ctx, permission) {
		return connect.NewError(connect.CodePermissionDenied, errors.New("permission denied"))
	}
	return nil
}

func DeviceScopeListFilter(ctx context.Context, permission string) (groupIDs []string, restricted bool) {
	return scopeListFilter(ctx, permission, DeviceScopeFilterFor)
}

func IsDeviceScopeRestricted(ctx context.Context, permission string) bool {
	_, restricted := scopeListFilter(ctx, permission, DeviceScopeFilterFor)
	return restricted
}

func UserScopeListFilter(ctx context.Context, permission string) (groupIDs []string, restricted bool) {
	return scopeListFilter(ctx, permission, UserScopeFilterFor)
}

func ObjectScopeListFilter(ctx context.Context) (groupIDs []string, restricted bool) {
	user, ok := UserFromContext(ctx)
	if !ok {

		return nil, true
	}
	seen := make(map[string]struct{}, len(user.ScopedGrants))
	sawScopedGrant := false
	for _, g := range user.ScopedGrants {
		if g.ScopeKind != ScopeKindDeviceGroup && g.ScopeKind != ScopeKindUserGroup {
			continue
		}

		sawScopedGrant = true
		if g.ScopeID == "" {
			continue
		}
		if _, dup := seen[g.ScopeID]; dup {
			continue
		}
		seen[g.ScopeID] = struct{}{}
		groupIDs = append(groupIDs, g.ScopeID)
	}
	if len(groupIDs) == 0 {

		return nil, sawScopedGrant
	}
	return groupIDs, true
}

func scopeListFilter(ctx context.Context, permission string, filterFor func(context.Context, string) ScopeFilter) (groupIDs []string, restricted bool) {
	if _, ok := UserFromContext(ctx); !ok {

		return nil, true
	}
	if !HasPermission(ctx, permission) {

		return nil, false
	}
	f := filterFor(ctx, permission)
	if f.Global {
		return nil, false
	}
	if len(f.GroupIDs) > 0 {
		return f.GroupIDs, true
	}
	if hasScopedGrant(ctx, permission) {
		return nil, true
	}
	return nil, false
}

func hasScopedGrant(ctx context.Context, permission string) bool {
	user, ok := UserFromContext(ctx)
	if !ok {
		return false
	}
	for _, g := range user.ScopedGrants {
		if g.Permission == permission {
			return true
		}
	}
	return false
}

func intersects(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(a))
	for _, x := range a {
		set[x] = struct{}{}
	}
	for _, y := range b {
		if _, ok := set[y]; ok {
			return true
		}
	}
	return false
}
