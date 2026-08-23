// Package identity implements the control server's identity RPCs:
// sessions, OIDC single sign-on, identity providers and their links,
// users, roles and role grants.
//
// Every handler in this package follows the same order at its trust
// boundary — validate, authenticate, authorize, act — and every
// mutation reaches the database through store.WithAudit, so the state
// change and the evidence for it commit together or not at all.
package identity

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"

	pmv1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/middleware"
)

// Error codes carried in the structured error detail. They are a fixed
// vocabulary: a client branches on the code, never on the message.
const (
	ErrNotAuthenticated = pmv1.ErrorCode_ERROR_CODE_NOT_AUTHENTICATED
	ErrTokenExpired     = pmv1.ErrorCode_ERROR_CODE_TOKEN_EXPIRED
	ErrPermissionDenied = pmv1.ErrorCode_ERROR_CODE_PERMISSION_DENIED
	ErrValidationFailed = pmv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED
	ErrInvalidPageToken = pmv1.ErrorCode_ERROR_CODE_INVALID_PAGE_TOKEN
	ErrInternal         = pmv1.ErrorCode_ERROR_CODE_INTERNAL_ERROR

	ErrUserNotFound            = pmv1.ErrorCode_ERROR_CODE_USER_NOT_FOUND
	ErrRoleNotFound            = pmv1.ErrorCode_ERROR_CODE_ROLE_NOT_FOUND
	ErrProviderNotFound        = pmv1.ErrorCode_ERROR_CODE_PROVIDER_NOT_FOUND
	ErrIdentityLinkNotFound    = pmv1.ErrorCode_ERROR_CODE_IDENTITY_LINK_NOT_FOUND
	ErrGrantNotFound           = pmv1.ErrorCode_ERROR_CODE_GRANT_NOT_FOUND
	ErrUserGroupNotFound       = pmv1.ErrorCode_ERROR_CODE_USER_GROUP_NOT_FOUND
	ErrUserGroupMemberNotFound = pmv1.ErrorCode_ERROR_CODE_USER_GROUP_MEMBER_NOT_FOUND

	ErrEmailAlreadyExists  = pmv1.ErrorCode_ERROR_CODE_EMAIL_ALREADY_EXISTS
	ErrRoleNameExists      = pmv1.ErrorCode_ERROR_CODE_ROLE_NAME_EXISTS
	ErrProviderSlugExists  = pmv1.ErrorCode_ERROR_CODE_PROVIDER_SLUG_EXISTS
	ErrUserAlreadyHasRole  = pmv1.ErrorCode_ERROR_CODE_USER_ALREADY_HAS_ROLE
	ErrUserGroupNameExists = pmv1.ErrorCode_ERROR_CODE_USER_GROUP_NAME_EXISTS

	ErrCannotModifySystemRole = pmv1.ErrorCode_ERROR_CODE_CANNOT_MODIFY_SYSTEM_ROLE
	ErrRoleInUse              = pmv1.ErrorCode_ERROR_CODE_ROLE_IN_USE
	ErrScopeNotPermitted      = pmv1.ErrorCode_ERROR_CODE_SCOPE_NOT_PERMITTED
	ErrProviderDisabled       = pmv1.ErrorCode_ERROR_CODE_PROVIDER_DISABLED
	ErrSCIMNotEnabled         = pmv1.ErrorCode_ERROR_CODE_SCIM_NOT_ENABLED
	ErrSSOStateExpired        = pmv1.ErrorCode_ERROR_CODE_SSO_STATE_EXPIRED
	ErrSSONoMatchingAccount   = pmv1.ErrorCode_ERROR_CODE_SSO_NO_MATCHING_ACCOUNT
	ErrCannotUnlinkOtherUser  = pmv1.ErrorCode_ERROR_CODE_CANNOT_UNLINK_OTHER_USER
	ErrLastAuthMethod         = pmv1.ErrorCode_ERROR_CODE_LAST_AUTH_METHOD
	ErrDynamicGroupMembership = pmv1.ErrorCode_ERROR_CODE_DYNAMIC_GROUP_MEMBERSHIP_MANAGED
	ErrSCIMManagedResource    = pmv1.ErrorCode_ERROR_CODE_SCIM_MANAGED_RESOURCE
	ErrInvalidDynamicQuery    = pmv1.ErrorCode_ERROR_CODE_INVALID_DYNAMIC_QUERY
	ErrGroupNotDynamic        = pmv1.ErrorCode_ERROR_CODE_GROUP_NOT_DYNAMIC
)

// rpcError builds a connect error carrying the structured detail the
// client correlates on.
//
// The message is always a fixed string chosen by the handler. Request
// input is never interpolated into it: an error message is the easiest
// accidental oracle in an RPC surface, and echoing input into one is
// how a value that should never leave the server ends up in a client
// log.
func rpcError(ctx context.Context, code pmv1.ErrorCode, connectCode connect.Code, msg string) *connect.Error {
	e := connect.NewError(connectCode, errors.New(msg))
	detail := &pmv1.ErrorDetail{Code: code, RequestId: middleware.RequestIDFromContext(ctx)}
	if d, err := connect.NewErrorDetail(detail); err == nil {
		e.AddDetail(d)
	}
	return e
}

// notFound is the answer for every object the caller may not see, as
// well as for every object that does not exist.
//
// Scoped non-owner access reports not-found rather than
// permission-denied: telling a caller "you may not see THAT one" is
// telling them it exists, which is the existence oracle the design
// forbids.
func notFound(ctx context.Context, code pmv1.ErrorCode, msg string) *connect.Error {
	return rpcError(ctx, code, connect.CodeNotFound, msg)
}

// internalError is the catch-all for a failure the caller cannot act
// on. The underlying error is never attached: it routinely quotes the
// input that caused it.
func internalError(ctx context.Context, msg string) *connect.Error {
	return rpcError(ctx, ErrInternal, connect.CodeInternal, msg)
}

func auditResultCode(code pmv1.ErrorCode) string {
	return strings.ToLower(strings.TrimPrefix(code.String(), "ERROR_CODE_"))
}
