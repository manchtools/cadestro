package identity

import (
	"context"
	"errors"
	"slices"

	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

var errConferredAuthority = errors.New("conferred authority exceeds actor authority")

func (h *Handlers) ListPermissions(ctx context.Context, req *connect.Request[cadestrov1.ListPermissionsRequest]) (*connect.Response[cadestrov1.ListPermissionsResponse], error) {
	if _, err := h.requireActor(ctx); err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, PermListPermissions, ""); err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.ListPermissionsResponse{Permissions: permissionsToProto()}), nil
}

func (h *Handlers) CreateRole(ctx context.Context, req *connect.Request[cadestrov1.CreateRoleRequest]) (*connect.Response[cadestrov1.CreateRoleResponse], error) {
	actor, err := h.requireActor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, PermCreateRole, ""); err != nil {
		return nil, err
	}
	perms, err := h.checkPermissionKeys(ctx, req.Msg.Permissions)
	if err != nil {
		return nil, err
	}

	roleID := ulid.Make().String()
	at := h.now().UTC()
	var created store.RoleRow
	_, err = h.store.WithAudit(ctx, h.mutationOp(req, actor, PermCreateRole),
		func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
			var err error
			created, err = tx.InsertRole(ctx, db.InsertRoleParams{
				ID:          roleID,
				Name:        req.Msg.Name,
				Description: req.Msg.Description,
				Permissions: perms,
				CreatedAt:   at,
				CreatedBy:   actor.ID,
			})
			if err != nil {
				return err
			}
			count := int64(len(perms))
			rec.Effect(store.AuditEffect{
				ResourceType:  "role",
				ResourceID:    roleID,
				Action:        "CREATE",
				Outcome:       store.EffectApplied,
				ChangedFields: []string{"name", "description", "permissions"},
				AfterCount:    &count,
			})
			return nil
		})
	if err != nil {
		if store.IsConflict(err) {
			return nil, rpcError(ctx, ErrRoleNameExists, connect.CodeAlreadyExists, "a role with that name already exists")
		}
		return nil, internalError(ctx, "failed to create role")
	}
	return connect.NewResponse(&cadestrov1.CreateRoleResponse{Role: roleToProto(created)}), nil
}

func (h *Handlers) GetRole(ctx context.Context, req *connect.Request[cadestrov1.GetRoleRequest]) (*connect.Response[cadestrov1.GetRoleResponse], error) {
	if _, err := h.requireActor(ctx); err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, PermGetRole, req.Msg.GetId().GetValue()); err != nil {
		return nil, err
	}
	role, err := h.store.GetRole(ctx, req.Msg.GetId().GetValue())
	if err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, ErrRoleNotFound, "role not found")
		}
		return nil, internalError(ctx, "failed to load role")
	}
	holders, err := h.store.CountRoleHolders(ctx, role.ID)
	if err != nil {
		return nil, internalError(ctx, "failed to count role holders")
	}
	return connect.NewResponse(&cadestrov1.GetRoleResponse{
		Role:      roleToProto(role),
		UserCount: int32(holders),
	}), nil
}

func (h *Handlers) ListRoles(ctx context.Context, req *connect.Request[cadestrov1.ListRolesRequest]) (*connect.Response[cadestrov1.ListRolesResponse], error) {
	if _, err := h.requireActor(ctx); err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, PermListRoles, ""); err != nil {
		return nil, err
	}
	limit := pageLimit(req.Msg.PageSize)
	rows, err := h.store.ListRoles(ctx, req.Msg.PageToken, limit)
	if err != nil {
		return nil, internalError(ctx, "failed to list roles")
	}
	total, err := h.store.CountRoles(ctx)
	if err != nil {
		return nil, internalError(ctx, "failed to count roles")
	}
	resp := &cadestrov1.ListRolesResponse{TotalCount: int32(total)}
	for _, r := range rows {
		resp.Roles = append(resp.Roles, roleToProto(r))
	}
	if len(rows) == int(limit) {
		resp.NextPageToken = rows[len(rows)-1].ID
	}
	return connect.NewResponse(resp), nil
}

func (h *Handlers) UpdateRole(ctx context.Context, req *connect.Request[cadestrov1.UpdateRoleRequest]) (*connect.Response[cadestrov1.UpdateRoleResponse], error) {
	actor, err := h.requireActor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, PermUpdateRole, req.Msg.GetRoleId().GetValue()); err != nil {
		return nil, err
	}
	before, err := h.store.GetRole(ctx, req.Msg.GetRoleId().GetValue())
	if err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, ErrRoleNotFound, "role not found")
		}
		return nil, internalError(ctx, "failed to load role")
	}
	if before.IsSystem {
		return nil, rpcError(ctx, ErrCannotModifySystemRole, connect.CodeFailedPrecondition, "system roles are managed by the server")
	}
	perms, err := h.checkPermissionKeys(ctx, req.Msg.Permissions)
	if err != nil {
		return nil, err
	}

	at := h.now().UTC()
	var updated store.RoleRow
	_, err = h.store.WithAudit(ctx, h.mutationOp(req, actor, PermUpdateRole),
		func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
			if err := checkPermissionSubset(actor.Permissions, perms); err != nil {
				return err
			}
			var err error
			updated, err = tx.UpdateRole(ctx, db.UpdateRoleParams{
				ID:          before.ID,
				Name:        req.Msg.Name,
				Description: req.Msg.Description,
				Permissions: perms,
				UpdatedAt:   &at,
			})
			if err != nil {
				return err
			}

			if err := h.invalidateRoleHolderSessions(ctx, tx, rec, before.ID); err != nil {
				return err
			}
			beforeCount := int64(len(before.Permissions))
			afterCount := int64(len(perms))
			rec.Effect(store.AuditEffect{
				ResourceType:  "role",
				ResourceID:    before.ID,
				Action:        "UPDATE",
				Outcome:       store.EffectApplied,
				ChangedFields: []string{"name", "description", "permissions"},
				BeforeCount:   &beforeCount,
				AfterCount:    &afterCount,
			})
			return nil
		})
	if err != nil {
		if errors.Is(err, errConferredAuthority) {
			return nil, rpcError(ctx, ErrPermissionDenied, connect.CodePermissionDenied, "cannot grant authority you do not hold")
		}
		if store.IsConflict(err) {
			return nil, rpcError(ctx, ErrRoleNameExists, connect.CodeAlreadyExists, "a role with that name already exists")
		}
		if store.IsNotFound(err) {
			return nil, notFound(ctx, ErrRoleNotFound, "role not found")
		}
		return nil, internalError(ctx, "failed to update role")
	}
	return connect.NewResponse(&cadestrov1.UpdateRoleResponse{Role: roleToProto(updated)}), nil
}

func (h *Handlers) DeleteRole(ctx context.Context, req *connect.Request[cadestrov1.DeleteRoleRequest]) (*connect.Response[cadestrov1.DeleteRoleResponse], error) {
	actor, err := h.requireActor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, PermDeleteRole, req.Msg.GetId().GetValue()); err != nil {
		return nil, err
	}
	before, err := h.store.GetRole(ctx, req.Msg.GetId().GetValue())
	if err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, ErrRoleNotFound, "role not found")
		}
		return nil, internalError(ctx, "failed to load role")
	}
	if before.IsSystem {
		return nil, rpcError(ctx, ErrCannotModifySystemRole, connect.CodeFailedPrecondition, "system roles cannot be deleted")
	}
	holders, err := h.store.CountRoleHolders(ctx, before.ID)
	if err != nil {
		return nil, internalError(ctx, "failed to count role holders")
	}
	if holders > 0 {
		return nil, rpcError(ctx, ErrRoleInUse, connect.CodeFailedPrecondition, "the role is still granted to subjects")
	}

	at := h.now().UTC()
	_, err = h.store.WithAudit(ctx, h.mutationOp(req, actor, PermDeleteRole),
		func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
			n, err := tx.SoftDeleteRole(ctx, db.SoftDeleteRoleParams{ID: before.ID, UpdatedAt: &at})
			if err != nil {
				return err
			}
			if n == 0 {
				return store.ErrNotFound
			}
			yes := true
			rec.Effect(store.AuditEffect{
				ResourceType:  "role",
				ResourceID:    before.ID,
				Action:        "DELETE",
				Outcome:       store.EffectApplied,
				ChangedFields: []string{"is_deleted"},
				AfterFlag:     &yes,
			})
			return nil
		})
	if err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, ErrRoleNotFound, "role not found")
		}
		return nil, internalError(ctx, "failed to delete role")
	}
	return connect.NewResponse(&cadestrov1.DeleteRoleResponse{}), nil
}

func (h *Handlers) AssignRoleToUser(ctx context.Context, req *connect.Request[cadestrov1.AssignRoleToUserRequest]) (*connect.Response[cadestrov1.AssignRoleToUserResponse], error) {
	actor, err := h.requireActor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, PermAssignRoleToUser, ""); err != nil {
		return nil, err
	}

	roleValues := make([]string, 0, len(req.Msg.GetRoleIds()))
	for _, id := range req.Msg.GetRoleIds() {
		roleValues = append(roleValues, id.GetValue())
	}
	roleIDs := requestedRoleIDs(req.Msg.GetRoleId().GetValue(), roleValues)
	if len(roleIDs) == 0 {
		return nil, rpcError(ctx, ErrValidationFailed, connect.CodeInvalidArgument, "role_id or role_ids is required")
	}
	target, err := h.store.GetUser(ctx, req.Msg.GetUserId().GetValue())
	if err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, ErrUserNotFound, "user not found")
		}
		return nil, internalError(ctx, "failed to load user")
	}

	scopeKind, scopeID, err := h.checkGrantScope(ctx, req.Msg.ScopeKind, req.Msg.GetScopeId().GetValue(), roleIDs)
	if err != nil {
		return nil, err
	}

	at := h.now().UTC()
	_, err = h.store.WithAudit(ctx, h.mutationOp(req, actor, PermAssignRoleToUser),
		func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
			if scopeID == nil {
				if err := h.enforceConferredAuthorityTx(ctx, tx, actor, roleIDs); err != nil {
					return err
				}
			}
			for _, roleID := range roleIDs {
				grantID := ulid.Make().String()
				if _, err := tx.InsertUserRoleGrant(ctx, db.InsertUserRoleGrantParams{
					GrantID:    grantID,
					UserID:     target.ID,
					RoleID:     roleID,
					AssignedAt: at,
					AssignedBy: actor.ID,
					ScopeKind:  scopeKind,
					ScopeID:    scopeID,
				}); err != nil {
					return err
				}
				rec.Effect(store.AuditEffect{
					ResourceType: "user_role",
					ResourceID:   grantID,
					Action:       "GRANT",
					Outcome:      store.EffectApplied,
					BeforeRef:    &target.ID,
					AfterRef:     &roleID,
				})
			}
			if _, err := h.invalidateSubjectSessions(ctx, tx, rec, target.ID); err != nil {
				return err
			}
			return nil
		})
	if err != nil {
		if store.IsConflict(err) {
			return nil, rpcError(ctx, ErrUserAlreadyHasRole, connect.CodeAlreadyExists, "the subject already holds that role at that scope")
		}
		if errors.Is(err, errConferredAuthority) {
			return nil, rpcError(ctx, ErrPermissionDenied, connect.CodePermissionDenied, "cannot grant authority you do not hold")
		}
		return nil, internalError(ctx, "failed to assign role")
	}
	return connect.NewResponse(&cadestrov1.AssignRoleToUserResponse{}), nil
}

func (h *Handlers) RevokeRoleFromUser(ctx context.Context, req *connect.Request[cadestrov1.RevokeRoleFromUserRequest]) (*connect.Response[cadestrov1.RevokeRoleFromUserResponse], error) {
	actor, err := h.requireActor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, PermRevokeRoleFromUser, ""); err != nil {
		return nil, err
	}
	target, err := h.store.GetUser(ctx, req.Msg.GetUserId().GetValue())
	if err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, ErrUserNotFound, "user not found")
		}
		return nil, internalError(ctx, "failed to load user")
	}
	scopeKind, scopeID, err := h.describedScope(ctx, req.Msg.ScopeKind, req.Msg.GetScopeId().GetValue())
	if err != nil {
		return nil, err
	}

	_, err = h.store.WithAudit(ctx, h.mutationOp(req, actor, PermRevokeRoleFromUser),
		func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
			var (
				grant db.UserRole
				err   error
			)
			if scopeID == nil {
				grant, err = tx.DeleteUnscopedUserRoleGrant(ctx, db.DeleteUnscopedUserRoleGrantParams{
					UserID: target.ID, RoleID: req.Msg.GetRoleId().GetValue(),
				})
			} else {
				grant, err = tx.DeleteScopedUserRoleGrant(ctx, db.DeleteScopedUserRoleGrantParams{
					UserID: target.ID, RoleID: req.Msg.GetRoleId().GetValue(), ScopeKind: scopeKind, ScopeID: scopeID,
				})
			}
			if err != nil {
				return err
			}
			rec.Effect(store.AuditEffect{
				ResourceType: "user_role",
				ResourceID:   grant.GrantID,
				Action:       "REVOKE",
				Outcome:      store.EffectApplied,
				BeforeRef:    &target.ID,
				AfterRef:     stringPtr(req.Msg.GetRoleId().GetValue()),
			})
			if _, err := h.invalidateSubjectSessions(ctx, tx, rec, target.ID); err != nil {
				return err
			}
			return nil
		})
	if err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, ErrGrantNotFound, "no such grant")
		}
		return nil, internalError(ctx, "failed to revoke role")
	}
	return connect.NewResponse(&cadestrov1.RevokeRoleFromUserResponse{}), nil
}

func (h *Handlers) AssignRoleToUserGroup(ctx context.Context, req *connect.Request[cadestrov1.AssignRoleToUserGroupRequest]) (*connect.Response[cadestrov1.AssignRoleToUserGroupResponse], error) {
	actor, err := h.requireActor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, PermAssignRoleToUserGroup, ""); err != nil {
		return nil, err
	}
	roleValues := make([]string, 0, len(req.Msg.GetRoleIds()))
	for _, id := range req.Msg.GetRoleIds() {
		roleValues = append(roleValues, id.GetValue())
	}
	roleIDs := requestedRoleIDs(req.Msg.GetRoleId().GetValue(), roleValues)
	if len(roleIDs) == 0 {
		return nil, rpcError(ctx, ErrValidationFailed, connect.CodeInvalidArgument, "role_id or role_ids is required")
	}
	scopeKind, scopeID, err := h.checkGrantScope(ctx, req.Msg.ScopeKind, req.Msg.GetScopeId().GetValue(), roleIDs)
	if err != nil {
		return nil, err
	}

	at := h.now().UTC()
	_, err = h.store.WithAudit(ctx, h.mutationOp(req, actor, PermAssignRoleToUserGroup),
		func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
			if scopeID == nil {
				if err := h.enforceConferredAuthorityTx(ctx, tx, actor, roleIDs); err != nil {
					return err
				}
			}
			for _, roleID := range roleIDs {
				grantID := ulid.Make().String()
				if _, err := tx.InsertUserGroupRoleGrant(ctx, db.InsertUserGroupRoleGrantParams{
					GrantID:    grantID,
					GroupID:    req.Msg.GetGroupId().GetValue(),
					RoleID:     roleID,
					AssignedAt: at,
					AssignedBy: actor.ID,
					ScopeKind:  scopeKind,
					ScopeID:    scopeID,
				}); err != nil {
					return err
				}
				rec.Effect(store.AuditEffect{
					ResourceType: "user_group_role",
					ResourceID:   grantID,
					Action:       "GRANT",
					Outcome:      store.EffectApplied,
					BeforeRef:    stringPtr(req.Msg.GetGroupId().GetValue()),
					AfterRef:     &roleID,
				})
			}
			if err := h.invalidateGroupMemberSessions(ctx, tx, rec, req.Msg.GetGroupId().GetValue()); err != nil {
				return err
			}
			return nil
		})
	if err != nil {
		if store.IsConflict(err) {
			return nil, rpcError(ctx, ErrUserAlreadyHasRole, connect.CodeAlreadyExists, "the group already holds that role at that scope")
		}
		if errors.Is(err, errConferredAuthority) {
			return nil, rpcError(ctx, ErrPermissionDenied, connect.CodePermissionDenied, "cannot grant authority you do not hold")
		}

		return nil, notFound(ctx, ErrRoleNotFound, "group or role not found")
	}
	return connect.NewResponse(&cadestrov1.AssignRoleToUserGroupResponse{}), nil
}

func (h *Handlers) RevokeRoleFromUserGroup(ctx context.Context, req *connect.Request[cadestrov1.RevokeRoleFromUserGroupRequest]) (*connect.Response[cadestrov1.RevokeRoleFromUserGroupResponse], error) {
	actor, err := h.requireActor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, PermRevokeRoleFromUserGroup, ""); err != nil {
		return nil, err
	}
	scopeKind, scopeID, err := h.describedScope(ctx, req.Msg.ScopeKind, req.Msg.GetScopeId().GetValue())
	if err != nil {
		return nil, err
	}

	_, err = h.store.WithAudit(ctx, h.mutationOp(req, actor, PermRevokeRoleFromUserGroup),
		func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
			var (
				grant db.UserGroupRole
				err   error
			)
			if scopeID == nil {
				grant, err = tx.DeleteUnscopedUserGroupRoleGrant(ctx, db.DeleteUnscopedUserGroupRoleGrantParams{
					GroupID: req.Msg.GetGroupId().GetValue(), RoleID: req.Msg.GetRoleId().GetValue(),
				})
			} else {
				grant, err = tx.DeleteScopedUserGroupRoleGrant(ctx, db.DeleteScopedUserGroupRoleGrantParams{
					GroupID: req.Msg.GetGroupId().GetValue(), RoleID: req.Msg.GetRoleId().GetValue(), ScopeKind: scopeKind, ScopeID: scopeID,
				})
			}
			if err != nil {
				return err
			}
			rec.Effect(store.AuditEffect{
				ResourceType: "user_group_role",
				ResourceID:   grant.GrantID,
				Action:       "REVOKE",
				Outcome:      store.EffectApplied,
				BeforeRef:    stringPtr(req.Msg.GetGroupId().GetValue()),
				AfterRef:     stringPtr(req.Msg.GetRoleId().GetValue()),
			})
			if err := h.invalidateGroupMemberSessions(ctx, tx, rec, req.Msg.GetGroupId().GetValue()); err != nil {
				return err
			}
			return nil
		})
	if err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, ErrGrantNotFound, "no such grant")
		}
		return nil, internalError(ctx, "failed to revoke role")
	}
	return connect.NewResponse(&cadestrov1.RevokeRoleFromUserGroupResponse{}), nil
}

func (h *Handlers) checkPermissionKeys(ctx context.Context, keys []string) ([]string, error) {
	valid := auth.ValidPermissionKeys()
	out := make([]string, 0, len(keys))
	seen := make(map[string]bool, len(keys))
	for _, k := range keys {
		if !valid[k] {
			return nil, rpcError(ctx, ErrValidationFailed, connect.CodeInvalidArgument, "permissions contains an unknown permission key")
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	slices.Sort(out)
	return out, nil
}

func (h *Handlers) checkGrantScope(
	ctx context.Context,
	kind cadestrov1.RoleGrantScopeKind,
	scopeID string,
	roleIDs []string,
) (*string, *string, error) {
	unscoped := kind == cadestrov1.RoleGrantScopeKind_ROLE_GRANT_SCOPE_KIND_UNSPECIFIED && scopeID == ""
	if unscoped {
		if err := h.enforceConferredAuthority(ctx, roleIDs); err != nil {
			return nil, nil, err
		}
		if err := auth.EnforceUnscopedGrantAuthority(ctx); err != nil {
			return nil, nil, rpcError(ctx, ErrPermissionDenied, connect.CodePermissionDenied,
				"a scope-limited administrator cannot create an unscoped grant")
		}
		return nil, nil, nil
	}

	if kind == cadestrov1.RoleGrantScopeKind_ROLE_GRANT_SCOPE_KIND_UNSPECIFIED || scopeID == "" {
		return nil, nil, rpcError(ctx, ErrValidationFailed, connect.CodeInvalidArgument,
			"scope_kind and scope_id must be set together or both omitted")
	}
	storedKind, ok := scopeKindFromProto(kind)
	if !ok {
		return nil, nil, rpcError(ctx, ErrValidationFailed, connect.CodeInvalidArgument, "unknown scope_kind")
	}

	if !auth.HasPermission(ctx, auth.AssignRoleScopePermission) {
		return nil, nil, rpcError(ctx, ErrPermissionDenied, connect.CodePermissionDenied,
			"attaching a scope to a role grant requires the scope-assignment authority")
	}

	for _, roleID := range roleIDs {
		role, err := h.store.GetRole(ctx, roleID)
		if err != nil {
			if store.IsNotFound(err) {
				return nil, nil, notFound(ctx, ErrRoleNotFound, "role not found")
			}
			return nil, nil, internalError(ctx, "failed to load role")
		}
		if _, found := auth.FirstPrivilegeGranting(role.Permissions); found {
			return nil, nil, rpcError(ctx, ErrScopeNotPermitted, connect.CodeInvalidArgument,
				"the role contains a permission that can grant or widen privilege; such a role can only be granted globally")
		}
		if _, scopable := auth.RolePermissionsScopableWith(role.Permissions, storedKind); !scopable {
			return nil, nil, rpcError(ctx, ErrScopeNotPermitted, connect.CodeInvalidArgument,
				"the role contains a permission that does not accept this scope kind")
		}
	}

	if err := auth.EnforceGrantScopeAuthority(ctx, storedKind, scopeID); err != nil {
		return nil, nil, rpcError(ctx, ErrPermissionDenied, connect.CodePermissionDenied,
			"cannot grant a scope outside your own scope authority")
	}
	return &storedKind, &scopeID, nil
}

func (h *Handlers) enforceConferredAuthority(ctx context.Context, roleIDs []string) error {
	actor, _ := auth.UserFromContext(ctx)
	if actor != nil && actor.Kind == auth.PrincipalBootstrapAdmin {
		return nil
	}
	for _, roleID := range roleIDs {
		role, err := h.store.GetRole(ctx, roleID)
		if err != nil {
			if store.IsNotFound(err) {
				return notFound(ctx, ErrRoleNotFound, "role not found")
			}
			return internalError(ctx, "failed to load role")
		}
		if err := checkPermissionSubset(actor.Permissions, role.Permissions); err != nil {
			return rpcError(ctx, ErrPermissionDenied, connect.CodePermissionDenied,
				"cannot grant authority you do not hold")
		}
	}
	return nil
}

func (h *Handlers) enforceConferredAuthorityTx(ctx context.Context, tx *store.Tx, actor *auth.UserContext, roleIDs []string) error {
	if actor != nil && actor.Kind == auth.PrincipalBootstrapAdmin {
		return nil
	}
	for _, roleID := range roleIDs {
		role, err := tx.GetRole(ctx, roleID)
		if err != nil {
			return err
		}
		if err := checkPermissionSubset(actor.Permissions, role.Permissions); err != nil {
			return err
		}
	}
	return nil
}

func checkPermissionSubset(actor, conferred []string) error {
	allowed := make(map[string]struct{}, len(actor))
	for _, permission := range actor {
		allowed[permission] = struct{}{}
	}
	for _, permission := range conferred {
		if _, ok := allowed[permission]; !ok {
			return errConferredAuthority
		}
	}
	return nil
}

func (h *Handlers) describedScope(ctx context.Context, kind cadestrov1.RoleGrantScopeKind, scopeID string) (*string, *string, error) {
	if kind == cadestrov1.RoleGrantScopeKind_ROLE_GRANT_SCOPE_KIND_UNSPECIFIED && scopeID == "" {
		return nil, nil, nil
	}
	if kind == cadestrov1.RoleGrantScopeKind_ROLE_GRANT_SCOPE_KIND_UNSPECIFIED || scopeID == "" {
		return nil, nil, rpcError(ctx, ErrValidationFailed, connect.CodeInvalidArgument,
			"scope_kind and scope_id must be set together or both omitted")
	}
	storedKind, ok := scopeKindFromProto(kind)
	if !ok {
		return nil, nil, rpcError(ctx, ErrValidationFailed, connect.CodeInvalidArgument, "unknown scope_kind")
	}
	return &storedKind, &scopeID, nil
}

func (h *Handlers) invalidateSubjectSessions(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder, userID string) (int32, error) {
	at := h.now().UTC()
	version, err := tx.BumpUserSessionVersion(ctx, db.BumpUserSessionVersionParams{ID: userID, UpdatedAt: &at})
	if err != nil {
		if store.IsNotFound(err) {
			return 0, nil
		}
		return 0, err
	}
	after := int64(version)
	rec.Effect(store.AuditEffect{
		ResourceType:  "user",
		ResourceID:    userID,
		Action:        "INVALIDATE_SESSIONS",
		Outcome:       store.EffectApplied,
		ChangedFields: []string{"session_version"},
		AfterCount:    &after,
	})
	return version, nil
}

func (h *Handlers) invalidateRoleHolderSessions(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder, roleID string) error {
	holderIDs, err := tx.ListRoleHolderIDs(ctx, roleID)
	if err != nil {
		return err
	}
	at := h.now().UTC()
	affected, err := tx.BumpSessionVersionForRoleHolders(ctx, db.BumpSessionVersionForRoleHoldersParams{
		RoleID:    roleID,
		UpdatedAt: &at,
	})
	if err != nil {
		return err
	}
	rec.Effect(store.AuditEffect{
		ResourceType:  "role",
		ResourceID:    roleID,
		Action:        "INVALIDATE_HOLDER_SESSIONS",
		Outcome:       store.EffectApplied,
		ChangedFields: []string{"session_version"},
		AfterCount:    &affected,
	})
	for _, userID := range holderIDs {
		rec.RefreshSearch("user", userID)
	}
	return nil
}

func (h *Handlers) invalidateGroupMemberSessions(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder, groupID string) error {
	memberIDs, err := tx.ListUserGroupMemberIDs(ctx, groupID)
	if err != nil {
		return err
	}
	at := h.now().UTC()
	affected, err := tx.BumpSessionVersionForUserGroupMembers(ctx, db.BumpSessionVersionForUserGroupMembersParams{
		UpdatedAt: &at, GroupID: groupID,
	})
	if err != nil {
		return err
	}
	if affected > 0 {
		effect := userGroupEffect(groupID, "INVALIDATE_MEMBER_SESSIONS", "session_version")
		effect.AfterCount = &affected
		rec.Effect(effect)
	}
	for _, userID := range memberIDs {
		rec.RefreshSearch("user", userID)
	}
	return nil
}

func requestedRoleIDs(single string, many []string) []string {
	out := make([]string, 0, len(many)+1)
	seen := make(map[string]bool, len(many)+1)
	for _, id := range append([]string{single}, many...) {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
