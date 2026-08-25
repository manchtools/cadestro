// Package registrationtoken implements direct audited CRUD for device
// registration tokens.
package registrationtoken

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/middleware"
)

const (
	errNotAuthenticated = cadestrov1.ErrorCode_ERROR_CODE_NOT_AUTHENTICATED
	errPermissionDenied = cadestrov1.ErrorCode_ERROR_CODE_PERMISSION_DENIED
	errValidationFailed = cadestrov1.ErrorCode_ERROR_CODE_VALIDATION_FAILED
	errInvalidPageToken = cadestrov1.ErrorCode_ERROR_CODE_INVALID_PAGE_TOKEN
	errInternal         = cadestrov1.ErrorCode_ERROR_CODE_INTERNAL_ERROR
	errTokenNotFound    = cadestrov1.ErrorCode_ERROR_CODE_TOKEN_NOT_FOUND
	errUserNotFound     = cadestrov1.ErrorCode_ERROR_CODE_USER_NOT_FOUND
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
