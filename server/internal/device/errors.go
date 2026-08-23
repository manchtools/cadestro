// Package device implements the device CRUD portion of the control service.
package device

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	pmv1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/middleware"
)

const (
	errNotAuthenticated        = pmv1.ErrorCode_ERROR_CODE_NOT_AUTHENTICATED
	errPermissionDenied        = pmv1.ErrorCode_ERROR_CODE_PERMISSION_DENIED
	errValidationFailed        = pmv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED
	errInvalidPageToken        = pmv1.ErrorCode_ERROR_CODE_INVALID_PAGE_TOKEN
	errInternal                = pmv1.ErrorCode_ERROR_CODE_INTERNAL_ERROR
	errDeviceNotFound          = pmv1.ErrorCode_ERROR_CODE_DEVICE_NOT_FOUND
	errQueryResultMissing      = pmv1.ErrorCode_ERROR_CODE_QUERY_RESULT_NOT_FOUND
	errExecutionNotFound       = pmv1.ErrorCode_ERROR_CODE_EXECUTION_NOT_FOUND
	errActionNotFound          = pmv1.ErrorCode_ERROR_CODE_ACTION_NOT_FOUND
	errLpsPasswordNotFound     = pmv1.ErrorCode_ERROR_CODE_LPS_PASSWORD_NOT_FOUND
	errLuksKeyNotFound         = pmv1.ErrorCode_ERROR_CODE_LUKS_KEY_NOT_FOUND
	errRevocationPending       = pmv1.ErrorCode_ERROR_CODE_LUKS_KEY_REVOCATION_PENDING
	errAlreadyRevoked          = pmv1.ErrorCode_ERROR_CODE_LUKS_KEY_ALREADY_REVOKED
	errDeviceUnavailable       = pmv1.ErrorCode_ERROR_CODE_DEVICE_UNAVAILABLE
	errDeviceNotConnected      = pmv1.ErrorCode_ERROR_CODE_DEVICE_NOT_CONNECTED
	errTerminalUsernameMissing = pmv1.ErrorCode_ERROR_CODE_TERMINAL_LINUX_USERNAME_NOT_SET
	errTerminalSessionMissing  = pmv1.ErrorCode_ERROR_CODE_TERMINAL_SESSION_NOT_FOUND
	errUserNotFound            = pmv1.ErrorCode_ERROR_CODE_USER_NOT_FOUND
	errUserGroupMissing        = pmv1.ErrorCode_ERROR_CODE_USER_GROUP_NOT_FOUND
)

func rpcError(ctx context.Context, code pmv1.ErrorCode, connectCode connect.Code, message string) *connect.Error {
	err := connect.NewError(connectCode, errors.New(message))
	detail, detailErr := connect.NewErrorDetail(&pmv1.ErrorDetail{
		Code: code, RequestId: middleware.RequestIDFromContext(ctx),
	})
	if detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

func notFound(ctx context.Context, code pmv1.ErrorCode, message string) *connect.Error {
	return rpcError(ctx, code, connect.CodeNotFound, message)
}
