package core

import (
	"context"
	"database/sql"

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
	permissions := make([]cadestrov1.Permission, 0, 46)
	for permission := cadestrov1.Permission_PERMISSION_GET_CURRENT_USER; permission <= cadestrov1.Permission_PERMISSION_REVOKE_USER_SESSIONS; permission++ {
		permissions = append(permissions, permission)
	}
	return permissions
}

func roleProto(ctx context.Context, queries *db.Queries, role *db.Role) (*cadestrov1.Role, error) {
	permissions, err := queries.ListRolePermissions(ctx, role.ID)
	if err != nil {
		return nil, err
	}
	return &cadestrov1.Role{Id: &cadestrov1.RoleId{Value: role.ID}, Name: role.Name, Description: role.Description, Permissions: permissions, CreatedAt: timestamppb.New(role.CreatedAt), UpdatedAt: timestamppb.New(role.UpdatedAt)}, nil
}

func (service *Service) CreateRole(ctx context.Context, request *connect.Request[cadestrov1.CreateRoleRequest]) (*connect.Response[cadestrov1.CreateRoleResponse], error) {
	id := ulid.Make().String()
	var value *cadestrov1.Role
	if err := service.store.Transaction(ctx, func(queries *db.Queries) error {
		role, err := queries.CreateRole(ctx, db.CreateRoleParams{ID: id, Name: request.Msg.GetName(), Description: request.Msg.GetDescription()})
		if err != nil {
			return err
		}
		for _, permission := range request.Msg.GetPermissions() {
			if _, err := queries.GrantRolePermission(ctx, db.GrantRolePermissionParams{RoleID: id, Permission: permission}); err != nil {
				return err
			}
		}
		value, err = roleProto(ctx, queries, role)
		return err
	}); err != nil {
		if store.IsConflict(err) {
			return nil, rpcConflict("role name")
		}
		return nil, service.internal("set role permissions", err)
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
	roles, err := service.store.Queries().ListRoles(ctx, db.ListRolesParams{ID: request.Msg.GetPageToken(), Limit: limit + 1})
	if err != nil {
		return nil, service.internal("list roles", err)
	}
	roles, next := paginate(roles, limit, func(role *db.Role) string { return role.ID })
	response := &cadestrov1.ListRolesResponse{NextPageToken: next}
	for _, role := range roles {
		value, err := roleProto(ctx, service.store.Queries(), role)
		if err != nil {
			return nil, service.internal("read role permissions", err)
		}
		response.Roles = append(response.Roles, value)
	}
	return connect.NewResponse(response), nil
}

func (service *Service) RenameRole(ctx context.Context, request *connect.Request[cadestrov1.RenameRoleRequest]) (*connect.Response[cadestrov1.RenameRoleResponse], error) {
	id := request.Msg.GetId().GetValue()
	role, err := service.roleMutation(ctx, id, "rename role", func(queries *db.Queries) (*db.Role, error) {
		return queries.RenameRole(ctx, db.RenameRoleParams{Name: request.Msg.GetName(), ID: id})
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.RenameRoleResponse{Role: role}), nil
}

func (service *Service) SetRoleDescription(ctx context.Context, request *connect.Request[cadestrov1.SetRoleDescriptionRequest]) (*connect.Response[cadestrov1.SetRoleDescriptionResponse], error) {
	id := request.Msg.GetId().GetValue()
	role, err := service.roleMutation(ctx, id, "set role description", func(queries *db.Queries) (*db.Role, error) {
		return queries.SetRoleDescription(ctx, db.SetRoleDescriptionParams{Description: request.Msg.GetDescription(), ID: id})
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.SetRoleDescriptionResponse{Role: role}), nil
}

func (service *Service) GrantRolePermission(ctx context.Context, request *connect.Request[cadestrov1.GrantRolePermissionRequest]) (*connect.Response[cadestrov1.GrantRolePermissionResponse], error) {
	id := request.Msg.GetId().GetValue()
	role, err := service.roleMutation(ctx, id, "grant role permission", func(queries *db.Queries) (*db.Role, error) {
		if _, err := queries.GrantRolePermission(ctx, db.GrantRolePermissionParams{RoleID: id, Permission: request.Msg.GetPermission()}); err != nil {
			return nil, err
		}
		return queries.TouchRole(ctx, id)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.GrantRolePermissionResponse{Role: role}), nil
}

func (service *Service) RevokeRolePermission(ctx context.Context, request *connect.Request[cadestrov1.RevokeRolePermissionRequest]) (*connect.Response[cadestrov1.RevokeRolePermissionResponse], error) {
	id := request.Msg.GetId().GetValue()
	role, err := service.roleMutation(ctx, id, "revoke role permission", func(queries *db.Queries) (*db.Role, error) {
		rows, err := queries.RevokeRolePermission(ctx, db.RevokeRolePermissionParams{RoleID: id, Permission: request.Msg.GetPermission()})
		if err != nil {
			return nil, err
		}
		if rows != 1 {
			return nil, sql.ErrNoRows
		}
		return queries.TouchRole(ctx, id)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.RevokeRolePermissionResponse{Role: role}), nil
}

func (service *Service) roleMutation(ctx context.Context, id, operation string, mutate func(*db.Queries) (*db.Role, error)) (*cadestrov1.Role, error) {
	var value *cadestrov1.Role
	if err := service.store.Transaction(ctx, func(queries *db.Queries) error {
		role, err := mutate(queries)
		if err != nil {
			return err
		}
		if err := queries.BumpSessionsForRole(ctx, id); err != nil {
			return err
		}
		value, err = roleProto(ctx, queries, role)
		return err
	}); err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("role")
		}
		if store.IsConflict(err) {
			if operation == "grant role permission" {
				return nil, rpcConflict("role permission")
			}
			return nil, rpcConflict("role name")
		}
		return nil, service.internal(operation, err)
	}
	return value, nil
}

func (service *Service) DeleteRole(ctx context.Context, request *connect.Request[cadestrov1.DeleteRoleRequest]) (*connect.Response[cadestrov1.DeleteRoleResponse], error) {
	id := request.Msg.GetId().GetValue()
	err := service.store.Transaction(ctx, func(queries *db.Queries) error {
		if err := queries.BumpSessionsForRole(ctx, id); err != nil {
			return err
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
		if store.IsNotFound(err) {
			return nil, rpcNotFound("role")
		}
		return nil, service.internal("delete role", err)
	}
	return connect.NewResponse(&cadestrov1.DeleteRoleResponse{}), nil
}

func (service *Service) AssignRoleToUser(ctx context.Context, request *connect.Request[cadestrov1.AssignRoleToUserRequest]) (*connect.Response[cadestrov1.AssignRoleToUserResponse], error) {
	err := service.store.Transaction(ctx, func(queries *db.Queries) error {
		if _, err := queries.AssignRoleToUser(ctx, db.AssignRoleToUserParams{UserID: request.Msg.GetUserId().GetValue(), RoleID: request.Msg.GetRoleId().GetValue()}); err != nil {
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
		rows, err := queries.RevokeRoleFromUser(ctx, db.RevokeRoleFromUserParams{UserID: request.Msg.GetUserId().GetValue(), RoleID: request.Msg.GetRoleId().GetValue()})
		if err != nil {
			return err
		}
		if rows != 1 {
			return sql.ErrNoRows
		}
		_, err = queries.RotateUserSessionByID(ctx, request.Msg.GetUserId().GetValue())
		return err
	})
	if err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("user or role assignment")
		}
		return nil, service.internal("revoke role", err)
	}
	return connect.NewResponse(&cadestrov1.RevokeRoleFromUserResponse{}), nil
}

func (service *Service) ListPermissions(context.Context, *connect.Request[cadestrov1.ListPermissionsRequest]) (*connect.Response[cadestrov1.ListPermissionsResponse], error) {
	return connect.NewResponse(&cadestrov1.ListPermissionsResponse{Permissions: allPermissions()}), nil
}

func (service *Service) ListUsers(ctx context.Context, request *connect.Request[cadestrov1.ListUsersRequest]) (*connect.Response[cadestrov1.ListUsersResponse], error) {
	limit := pageSize(request.Msg.GetPageSize())
	users, err := service.store.Queries().ListUsers(ctx, db.ListUsersParams{ID: request.Msg.GetPageToken(), Limit: limit + 1})
	if err != nil {
		return nil, service.internal("list users", err)
	}
	users, next := paginate(users, limit, func(user *db.User) string { return user.ID })
	response := &cadestrov1.ListUsersResponse{NextPageToken: next}
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
	userID := request.Msg.GetUserId().GetValue()
	err := service.store.Transaction(ctx, func(queries *db.Queries) error {
		if _, err := queries.RotateUserSessionByID(ctx, userID); err != nil {
			return err
		}
		return service.audit(ctx, queries, cadestrov1.AuditEventType_AUDIT_EVENT_TYPE_USER_SESSIONS_REVOKED, cadestrov1.AuditStreamType_AUDIT_STREAM_TYPE_USER, userID, cadestrov1.AuditActorType_AUDIT_ACTOR_TYPE_USER, "")
	})
	if err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("user")
		}
		return nil, service.internal("revoke user sessions", err)
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
	result := &cadestrov1.User{Id: &cadestrov1.UserId{Value: user.ID}, Email: user.Email, DisplayName: user.DisplayName, CreatedAt: timestamppb.New(user.CreatedAt), LastLoginAt: timestamppb.New(user.LastLoginAt), Permissions: permissions}
	for _, role := range roles {
		value, err := roleProto(ctx, service.store.Queries(), role)
		if err != nil {
			return nil, err
		}
		result.Roles = append(result.Roles, value)
	}
	return result, nil
}
