package device

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/middleware"
)

const (
	errNotAuthenticated        = cadestrov1.ErrorCode_ERROR_CODE_NOT_AUTHENTICATED
	errPermissionDenied        = cadestrov1.ErrorCode_ERROR_CODE_PERMISSION_DENIED
	errValidationFailed        = cadestrov1.ErrorCode_ERROR_CODE_VALIDATION_FAILED
	errInvalidPageToken        = cadestrov1.ErrorCode_ERROR_CODE_INVALID_PAGE_TOKEN
	errInternal                = cadestrov1.ErrorCode_ERROR_CODE_INTERNAL_ERROR
	errDeviceNotFound          = cadestrov1.ErrorCode_ERROR_CODE_DEVICE_NOT_FOUND
	errQueryResultMissing      = cadestrov1.ErrorCode_ERROR_CODE_QUERY_RESULT_NOT_FOUND
	errActionNotFound          = cadestrov1.ErrorCode_ERROR_CODE_ACTION_NOT_FOUND
	errLpsPasswordNotFound     = cadestrov1.ErrorCode_ERROR_CODE_LPS_PASSWORD_NOT_FOUND
	errLuksKeyNotFound         = cadestrov1.ErrorCode_ERROR_CODE_LUKS_KEY_NOT_FOUND
	errRevocationPending       = cadestrov1.ErrorCode_ERROR_CODE_LUKS_KEY_REVOCATION_PENDING
	errAlreadyRevoked          = cadestrov1.ErrorCode_ERROR_CODE_LUKS_KEY_ALREADY_REVOKED
	errDeviceUnavailable       = cadestrov1.ErrorCode_ERROR_CODE_DEVICE_UNAVAILABLE
	errDeviceNotConnected      = cadestrov1.ErrorCode_ERROR_CODE_DEVICE_NOT_CONNECTED
	errTerminalUsernameMissing = cadestrov1.ErrorCode_ERROR_CODE_TERMINAL_LINUX_USERNAME_NOT_SET
	errTerminalSessionMissing  = cadestrov1.ErrorCode_ERROR_CODE_TERMINAL_SESSION_NOT_FOUND
	errUserNotFound            = cadestrov1.ErrorCode_ERROR_CODE_USER_NOT_FOUND
	errUserGroupMissing        = cadestrov1.ErrorCode_ERROR_CODE_USER_GROUP_NOT_FOUND
)

func rpcError(ctx context.Context, code cadestrov1.ErrorCode, connectCode connect.Code, message string) *connect.Error {
	err := connect.NewError(connectCode, errors.New(message))
	detail, detailErr := connect.NewErrorDetail(&cadestrov1.ErrorDetail{
		Code: code, RequestId: &cadestrov1.RequestId{Value: middleware.RequestIDFromContext(ctx)},
	})
	if detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

func notFound(ctx context.Context, code cadestrov1.ErrorCode, message string) *connect.Error {
	return rpcError(ctx, code, connect.CodeNotFound, message)
}
