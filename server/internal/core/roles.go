package core

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/types/known/timestamppb"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

const (
	administratorsRoleID = "01J00000000000000000000001"
	usersRoleID          = "01J00000000000000000000002"
)

func allPermissions() []cadestrov1.Permission {
	permissions := make([]cadestrov1.Permission, 0, 47)
	for permission := cadestrov1.Permission_PERMISSION_GET_CURRENT_USER; permission <= cadestrov1.Permission_PERMISSION_REVOKE_USER_SESSIONS; permission++ {
		permissions = append(permissions, permission)
	}
	return permissions
}

func (service *Service) ReconcileSystemRoles(ctx context.Context) error {
	now := service.now().UTC()
	return service.store.Transaction(ctx, func(queries *db.Queries) error {
		for _, spec := range []struct {
			id          string
			name        string
			permissions []cadestrov1.Permission
		}{
			{administratorsRoleID, "Administrators", allPermissions()},
			{usersRoleID, "Users", []cadestrov1.Permission{cadestrov1.Permission_PERMISSION_GET_CURRENT_USER}},
		} {
			role, err := queries.GetRole(ctx, spec.id)
			if store.IsNotFound(err) {
				role, err = queries.CreateRole(ctx, db.CreateRoleParams{ID: spec.id, Name: spec.name, IsSystem: true, CreatedAt: now, UpdatedAt: now})
			}
			if err != nil {
				return err
			}
			if role.Name != spec.name || !role.IsSystem {
				return fmt.Errorf("system role %q is invalid", spec.name)
			}
			if err := queries.ReplaceRolePermissions(ctx, spec.id); err != nil {
				return err
			}
			for _, permission := range spec.permissions {
				if err := queries.AddRolePermission(ctx, db.AddRolePermissionParams{RoleID: spec.id, Permission: permission}); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func roleProto(ctx context.Context, queries *db.Queries, role *db.Role) (*cadestrov1.Role, error) {
	permissions, err := queries.ListRolePermissions(ctx, role.ID)
	if err != nil {
		return nil, err
	}
	return &cadestrov1.Role{Id: &cadestrov1.RoleId{Value: role.ID}, Name: role.Name, Description: role.Description, Permissions: permissions, IsSystem: role.IsSystem, CreatedAt: timestamp(role.CreatedAt), UpdatedAt: timestamp(role.UpdatedAt)}, nil
}

func timestamp(value time.Time) *timestamppb.Timestamp { return timestamppb.New(value) }

func (service *Service) CreateRole(ctx context.Context, request *connect.Request[cadestrov1.CreateRoleRequest]) (*connect.Response[cadestrov1.CreateRoleResponse], error) {
	now := service.now().UTC()
	id := ulid.Make().String()
	if err := service.store.Transaction(ctx, func(queries *db.Queries) error {
		if _, err := queries.CreateRole(ctx, db.CreateRoleParams{ID: id, Name: request.Msg.GetName(), Description: request.Msg.GetDescription(), CreatedAt: now, UpdatedAt: now}); err != nil {
			return err
		}
		for _, permission := range request.Msg.GetPermissions() {
			if err := queries.AddRolePermission(ctx, db.AddRolePermissionParams{RoleID: id, Permission: permission}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		if store.IsConflict(err) {
			return nil, rpcConflict("role name")
		}
		return nil, service.internal("set role permissions", err)
	}
	role, err := service.store.Queries().GetRole(ctx, id)
	if err != nil {
		return nil, service.internal("read role", err)
	}
	value, err := roleProto(ctx, service.store.Queries(), role)
	if err != nil {
		return nil, service.internal("read role permissions", err)
	}
	return connect.NewResponse(&cadestrov1.CreateRoleResponse{Role: value}), nil
}

func (service *Service) GetRole(ctx context.Context, request *connect.Request[cadestrov1.GetRoleRequest]) (*connect.Response[cadestrov1.GetRoleResponse], error) {
	role, err := service.store.Queries().GetRole(ctx, request.Msg.GetId().GetValue())
	if err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("role")
		}
		return nil, service.internal("get role", err)
	}
	value, err := roleProto(ctx, service.store.Queries(), role)
	if err != nil {
		return nil, service.internal("read role permissions", err)
	}
	return connect.NewResponse(&cadestrov1.GetRoleResponse{Role: value}), nil
}

func (service *Service) ListRoles(ctx context.Context, request *connect.Request[cadestrov1.ListRolesRequest]) (*connect.Response[cadestrov1.ListRolesResponse], error) {
	limit := pageSize(request.Msg.GetPageSize())
	roles, err := service.store.Queries().ListRoles(ctx, db.ListRolesParams{ID: request.Msg.GetPageToken(), Limit: limit})
	if err != nil {
		return nil, service.internal("list roles", err)
	}
	response := &cadestrov1.ListRolesResponse{NextPageToken: nextPageToken(roles, limit, func(role *db.Role) string { return role.ID })}
	for _, role := range roles {
		value, err := roleProto(ctx, service.store.Queries(), role)
		if err != nil {
			return nil, service.internal("read role permissions", err)
		}
		response.Roles = append(response.Roles, value)
	}
	return connect.NewResponse(response), nil
}

func (service *Service) UpdateRole(ctx context.Context, request *connect.Request[cadestrov1.UpdateRoleRequest]) (*connect.Response[cadestrov1.UpdateRoleResponse], error) {
	id := request.Msg.GetId().GetValue()
	if err := service.store.Transaction(ctx, func(queries *db.Queries) error {
		role, err := queries.GetRole(ctx, id)
		if err != nil {
			return err
		}
		if role.IsSystem {
			return errSystemRole
		}
		if _, err := queries.UpdateRole(ctx, db.UpdateRoleParams{Name: request.Msg.GetName(), Description: request.Msg.GetDescription(), UpdatedAt: service.now().UTC(), ID: id}); err != nil {
			return err
		}
		if err := queries.ReplaceRolePermissions(ctx, id); err != nil {
			return err
		}
		for _, permission := range request.Msg.GetPermissions() {
			if err := queries.AddRolePermission(ctx, db.AddRolePermissionParams{RoleID: id, Permission: permission}); err != nil {
				return err
			}
		}
		return queries.BumpSessionsForRole(ctx, id)
	}); err != nil {
		if errors.Is(err, errSystemRole) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("system role cannot be changed"))
		}
		if store.IsNotFound(err) {
			return nil, rpcNotFound("role")
		}
		if store.IsConflict(err) {
			return nil, rpcConflict("role name")
		}
		return nil, service.internal("update role", err)
	}
	role, err := service.store.Queries().GetRole(ctx, id)
	if err != nil {
		return nil, service.internal("read updated role", err)
	}
	value, err := roleProto(ctx, service.store.Queries(), role)
	if err != nil {
		return nil, service.internal("read role permissions", err)
	}
	return connect.NewResponse(&cadestrov1.UpdateRoleResponse{Role: value}), nil
}

var errSystemRole = errors.New("system role")

func (service *Service) DeleteRole(ctx context.Context, request *connect.Request[cadestrov1.DeleteRoleRequest]) (*connect.Response[cadestrov1.DeleteRoleResponse], error) {
	id := request.Msg.GetId().GetValue()
	err := service.store.Transaction(ctx, func(queries *db.Queries) error {
		role, err := queries.GetRole(ctx, id)
		if err != nil {
			return err
		}
		if role.IsSystem {
			return errSystemRole
		}
		count, err := queries.CountUsersWithRole(ctx, id)
		if err != nil || count > 0 {
			if err != nil {
				return err
			}
			return errAssignedRole
		}
		rows, err := queries.DeleteRole(ctx, id)
		if err != nil {
			return err
		}
		if rows != 1 {
			return sql.ErrNoRows
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errSystemRole) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("system role cannot be changed"))
		}
		if errors.Is(err, errAssignedRole) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("assigned role cannot be deleted"))
		}
		if store.IsNotFound(err) {
			return nil, rpcNotFound("role")
		}
		return nil, service.internal("delete role", err)
	}
	return connect.NewResponse(&cadestrov1.DeleteRoleResponse{}), nil
}

var errAssignedRole = errors.New("assigned role")

func (service *Service) AssignRoleToUser(ctx context.Context, request *connect.Request[cadestrov1.AssignRoleToUserRequest]) (*connect.Response[cadestrov1.AssignRoleToUserResponse], error) {
	err := service.store.Transaction(ctx, func(queries *db.Queries) error {
		if _, err := queries.GetUser(ctx, request.Msg.GetUserId().GetValue()); err != nil {
			return err
		}
		if _, err := queries.GetRole(ctx, request.Msg.GetRoleId().GetValue()); err != nil {
			return err
		}
		if err := queries.AssignRoleToUser(ctx, db.AssignRoleToUserParams{UserID: request.Msg.GetUserId().GetValue(), RoleID: request.Msg.GetRoleId().GetValue()}); err != nil {
			return err
		}
		_, err := queries.RotateUserSessionByID(ctx, request.Msg.GetUserId().GetValue())
		return err
	})
	if err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("user or role")
		}
		if store.IsConflict(err) {
			return nil, rpcConflict("role assignment")
		}
		return nil, service.internal("assign role", err)
	}
	return connect.NewResponse(&cadestrov1.AssignRoleToUserResponse{}), nil
}

func (service *Service) RevokeRoleFromUser(ctx context.Context, request *connect.Request[cadestrov1.RevokeRoleFromUserRequest]) (*connect.Response[cadestrov1.RevokeRoleFromUserResponse], error) {
	err := service.store.Transaction(ctx, func(queries *db.Queries) error {
		if _, err := queries.GetUser(ctx, request.Msg.GetUserId().GetValue()); err != nil {
			return err
		}
		role, err := queries.GetRole(ctx, request.Msg.GetRoleId().GetValue())
		if err != nil {
			return err
		}
		rows, err := queries.RevokeRoleFromUser(ctx, db.RevokeRoleFromUserParams{UserID: request.Msg.GetUserId().GetValue(), RoleID: request.Msg.GetRoleId().GetValue()})
		if err != nil {
			return err
		}
		if rows != 1 {
			return sql.ErrNoRows
		}
		if role.ID == administratorsRoleID {
			count, err := queries.CountUsersWithRole(ctx, role.ID)
			if err != nil {
				return err
			}
			if count == 0 {
				return errLastAdministrator
			}
		}
		_, err = queries.RotateUserSessionByID(ctx, request.Msg.GetUserId().GetValue())
		return err
	})
	if err != nil {
		if errors.Is(err, errLastAdministrator) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("last administrator cannot be revoked"))
		}
		if store.IsNotFound(err) {
			return nil, rpcNotFound("user or role assignment")
		}
		return nil, service.internal("revoke role", err)
	}
	return connect.NewResponse(&cadestrov1.RevokeRoleFromUserResponse{}), nil
}

var errLastAdministrator = errors.New("last administrator")

func (service *Service) ListPermissions(context.Context, *connect.Request[cadestrov1.ListPermissionsRequest]) (*connect.Response[cadestrov1.ListPermissionsResponse], error) {
	return connect.NewResponse(&cadestrov1.ListPermissionsResponse{Permissions: allPermissions()}), nil
}

func (service *Service) ListUsers(ctx context.Context, request *connect.Request[cadestrov1.ListUsersRequest]) (*connect.Response[cadestrov1.ListUsersResponse], error) {
	users, err := service.store.Queries().ListUsers(ctx, db.ListUsersParams{ID: request.Msg.GetPageToken(), Limit: pageSize(request.Msg.GetPageSize())})
	if err != nil {
		return nil, service.internal("list users", err)
	}
	response := &cadestrov1.ListUsersResponse{NextPageToken: nextPageToken(users, pageSize(request.Msg.GetPageSize()), func(user *db.User) string { return user.ID })}
	for _, user := range users {
		value, err := service.userProto(ctx, user)
		if err != nil {
			return nil, service.internal("read user roles", err)
		}
		response.Users = append(response.Users, value)
	}
	return connect.NewResponse(response), nil
}

func (service *Service) RevokeUserSessions(ctx context.Context, request *connect.Request[cadestrov1.RevokeUserSessionsRequest]) (*connect.Response[cadestrov1.RevokeUserSessionsResponse], error) {
	if _, err := service.store.Queries().RotateUserSessionByID(ctx, request.Msg.GetUserId().GetValue()); err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("user")
		}
		return nil, service.internal("revoke user sessions", err)
	}
	if err := service.audit(ctx, "user.sessions_revoked", "user", request.Msg.GetUserId().GetValue(), "user", ""); err != nil {
		return nil, service.internal("audit session revocation", err)
	}
	return connect.NewResponse(&cadestrov1.RevokeUserSessionsResponse{}), nil
}

func (service *Service) userProto(ctx context.Context, user *db.User) (*cadestrov1.User, error) {
	roles, err := service.store.Queries().ListUserRoles(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	permissions, err := service.store.Queries().ListUserPermissions(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	result := &cadestrov1.User{Id: &cadestrov1.UserId{Value: user.ID}, Email: user.Email, DisplayName: user.DisplayName, Picture: user.Picture, CreatedAt: timestamppb.New(user.CreatedAt), LastLoginAt: timestamppb.New(user.LastLoginAt), Permissions: permissions}
	for _, role := range roles {
		value, err := roleProto(ctx, service.store.Queries(), role)
		if err != nil {
			return nil, err
		}
		result.Roles = append(result.Roles, value)
	}
	return result, nil
}
