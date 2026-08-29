package auth

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
)

type Interceptor struct {
	jwt *JWTManager
}

func NewInterceptor(jwt *JWTManager) *Interceptor {
	return &Interceptor{jwt: jwt}
}

func (interceptor *Interceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, request connect.AnyRequest) (connect.AnyResponse, error) {
		if publicProcedure(request.Spec().Procedure) {
			return next(ctx, request)
		}
		authorization := request.Header().Get("Authorization")
		prefix, token, found := strings.Cut(authorization, " ")
		if !found || !strings.EqualFold(prefix, "Bearer") || token == "" {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
		}
		claims, err := interceptor.jwt.ValidateToken(token, TokenTypeAccess)
		if err != nil {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid access token"))
		}
		permission, ok := PermissionForProcedure(request.Spec().Procedure)
		if !ok || !userHasPermission(claims.Permissions, permission) {
			return nil, connect.NewError(connect.CodePermissionDenied, errors.New("permission denied"))
		}
		return next(WithUser(ctx, &UserContext{ID: claims.UserID, Email: claims.Email, SessionVersion: claims.SessionVersion, Permissions: claims.Permissions}), request)
	}
}

func userHasPermission(permissions []cadestrov1.Permission, required cadestrov1.Permission) bool {
	for _, permission := range permissions {
		if permission == required {
			return true
		}
	}
	return false
}

func PermissionForProcedure(procedure string) (cadestrov1.Permission, bool) {
	switch procedure {
	case cadestrov1connect.ControlServiceGetCurrentUserProcedure:
		return cadestrov1.Permission_PERMISSION_GET_CURRENT_USER, true
	case cadestrov1connect.ControlServiceCreateIdentityProviderProcedure:
		return cadestrov1.Permission_PERMISSION_CREATE_IDENTITY_PROVIDER, true
	case cadestrov1connect.ControlServiceGetIdentityProviderProcedure:
		return cadestrov1.Permission_PERMISSION_GET_IDENTITY_PROVIDER, true
	case cadestrov1connect.ControlServiceListIdentityProvidersProcedure:
		return cadestrov1.Permission_PERMISSION_LIST_IDENTITY_PROVIDERS, true
	case cadestrov1connect.ControlServiceUpdateIdentityProviderProcedure:
		return cadestrov1.Permission_PERMISSION_UPDATE_IDENTITY_PROVIDER, true
	case cadestrov1connect.ControlServiceDeleteIdentityProviderProcedure:
		return cadestrov1.Permission_PERMISSION_DELETE_IDENTITY_PROVIDER, true
	case cadestrov1connect.ControlServiceListDevicesProcedure:
		return cadestrov1.Permission_PERMISSION_LIST_DEVICES, true
	case cadestrov1connect.ControlServiceGetDeviceProcedure:
		return cadestrov1.Permission_PERMISSION_GET_DEVICE, true
	case cadestrov1connect.ControlServiceDeleteDeviceProcedure:
		return cadestrov1.Permission_PERMISSION_DELETE_DEVICE, true
	case cadestrov1connect.ControlServiceCreateTokenProcedure:
		return cadestrov1.Permission_PERMISSION_CREATE_TOKEN, true
	case cadestrov1connect.ControlServiceListTokensProcedure:
		return cadestrov1.Permission_PERMISSION_LIST_TOKENS, true
	case cadestrov1connect.ControlServiceRenameTokenProcedure:
		return cadestrov1.Permission_PERMISSION_RENAME_TOKEN, true
	case cadestrov1connect.ControlServiceSetTokenDisabledProcedure:
		return cadestrov1.Permission_PERMISSION_SET_TOKEN_DISABLED, true
	case cadestrov1connect.ControlServiceDeleteTokenProcedure:
		return cadestrov1.Permission_PERMISSION_DELETE_TOKEN, true
	case cadestrov1connect.ControlServiceCreateActionProcedure:
		return cadestrov1.Permission_PERMISSION_CREATE_ACTION, true
	case cadestrov1connect.ControlServiceGetActionProcedure:
		return cadestrov1.Permission_PERMISSION_GET_ACTION, true
	case cadestrov1connect.ControlServiceListActionsProcedure:
		return cadestrov1.Permission_PERMISSION_LIST_ACTIONS, true
	case cadestrov1connect.ControlServiceRenameActionProcedure:
		return cadestrov1.Permission_PERMISSION_RENAME_ACTION, true
	case cadestrov1connect.ControlServiceUpdateActionDescriptionProcedure:
		return cadestrov1.Permission_PERMISSION_UPDATE_ACTION_DESCRIPTION, true
	case cadestrov1connect.ControlServiceUpdateActionParamsProcedure:
		return cadestrov1.Permission_PERMISSION_UPDATE_ACTION_PARAMS, true
	case cadestrov1connect.ControlServiceDeleteActionProcedure:
		return cadestrov1.Permission_PERMISSION_DELETE_ACTION, true
	case cadestrov1connect.ControlServiceCreateDeviceGroupProcedure:
		return cadestrov1.Permission_PERMISSION_CREATE_DEVICE_GROUP, true
	case cadestrov1connect.ControlServiceGetDeviceGroupProcedure:
		return cadestrov1.Permission_PERMISSION_GET_DEVICE_GROUP, true
	case cadestrov1connect.ControlServiceListDeviceGroupsProcedure:
		return cadestrov1.Permission_PERMISSION_LIST_DEVICE_GROUPS, true
	case cadestrov1connect.ControlServiceListDeviceGroupsForDeviceProcedure:
		return cadestrov1.Permission_PERMISSION_LIST_DEVICE_GROUPS_FOR_DEVICE, true
	case cadestrov1connect.ControlServiceRenameDeviceGroupProcedure:
		return cadestrov1.Permission_PERMISSION_RENAME_DEVICE_GROUP, true
	case cadestrov1connect.ControlServiceUpdateDeviceGroupDescriptionProcedure:
		return cadestrov1.Permission_PERMISSION_UPDATE_DEVICE_GROUP_DESCRIPTION, true
	case cadestrov1connect.ControlServiceDeleteDeviceGroupProcedure:
		return cadestrov1.Permission_PERMISSION_DELETE_DEVICE_GROUP, true
	case cadestrov1connect.ControlServiceAddDeviceToGroupProcedure:
		return cadestrov1.Permission_PERMISSION_ADD_DEVICE_TO_GROUP, true
	case cadestrov1connect.ControlServiceRemoveDeviceFromGroupProcedure:
		return cadestrov1.Permission_PERMISSION_REMOVE_DEVICE_FROM_GROUP, true
	case cadestrov1connect.ControlServiceCreateAssignmentProcedure:
		return cadestrov1.Permission_PERMISSION_CREATE_ASSIGNMENT, true
	case cadestrov1connect.ControlServiceDeleteAssignmentProcedure:
		return cadestrov1.Permission_PERMISSION_DELETE_ASSIGNMENT, true
	case cadestrov1connect.ControlServiceListAssignmentsProcedure:
		return cadestrov1.Permission_PERMISSION_LIST_ASSIGNMENTS, true
	case cadestrov1connect.ControlServiceGetDeviceAssignmentsProcedure:
		return cadestrov1.Permission_PERMISSION_GET_DEVICE_ASSIGNMENTS, true
	case cadestrov1connect.ControlServiceGetDeviceComplianceProcedure:
		return cadestrov1.Permission_PERMISSION_GET_DEVICE_COMPLIANCE, true
	case cadestrov1connect.ControlServiceListExecutionResultsProcedure:
		return cadestrov1.Permission_PERMISSION_LIST_EXECUTION_RESULTS, true
	case cadestrov1connect.ControlServiceListAuditEventsProcedure:
		return cadestrov1.Permission_PERMISSION_LIST_AUDIT_EVENTS, true
	case cadestrov1connect.ControlServiceCreateRoleProcedure:
		return cadestrov1.Permission_PERMISSION_CREATE_ROLE, true
	case cadestrov1connect.ControlServiceGetRoleProcedure:
		return cadestrov1.Permission_PERMISSION_GET_ROLE, true
	case cadestrov1connect.ControlServiceListRolesProcedure:
		return cadestrov1.Permission_PERMISSION_LIST_ROLES, true
	case cadestrov1connect.ControlServiceUpdateRoleProcedure:
		return cadestrov1.Permission_PERMISSION_UPDATE_ROLE, true
	case cadestrov1connect.ControlServiceDeleteRoleProcedure:
		return cadestrov1.Permission_PERMISSION_DELETE_ROLE, true
	case cadestrov1connect.ControlServiceAssignRoleToUserProcedure:
		return cadestrov1.Permission_PERMISSION_ASSIGN_ROLE_TO_USER, true
	case cadestrov1connect.ControlServiceRevokeRoleFromUserProcedure:
		return cadestrov1.Permission_PERMISSION_REVOKE_ROLE_FROM_USER, true
	case cadestrov1connect.ControlServiceListPermissionsProcedure:
		return cadestrov1.Permission_PERMISSION_LIST_PERMISSIONS, true
	case cadestrov1connect.ControlServiceListUsersProcedure:
		return cadestrov1.Permission_PERMISSION_LIST_USERS, true
	case cadestrov1connect.ControlServiceRevokeUserSessionsProcedure:
		return cadestrov1.Permission_PERMISSION_REVOKE_USER_SESSIONS, true
	default:
		return cadestrov1.Permission_PERMISSION_UNSPECIFIED, false
	}
}

func publicProcedure(procedure string) bool {
	switch procedure {
	case cadestrov1connect.ControlServiceRefreshTokenProcedure,
		cadestrov1connect.ControlServiceLogoutProcedure,
		cadestrov1connect.ControlServiceListAuthMethodsProcedure,
		cadestrov1connect.ControlServiceGetSSOLoginURLProcedure,
		cadestrov1connect.ControlServiceSSOCallbackProcedure,
		cadestrov1connect.ControlServiceRegisterProcedure,
		cadestrov1connect.ControlServiceRenewCertificateProcedure:
		return true
	default:
		return false
	}
}
