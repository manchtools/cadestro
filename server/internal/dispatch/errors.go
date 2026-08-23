package dispatch

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	pmv1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/middleware"
)

const (
	errNotAuthenticated      = pmv1.ErrorCode_ERROR_CODE_NOT_AUTHENTICATED
	errPermissionDenied      = pmv1.ErrorCode_ERROR_CODE_PERMISSION_DENIED
	errValidationFailed      = pmv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED
	errDeviceNotFound        = pmv1.ErrorCode_ERROR_CODE_DEVICE_NOT_FOUND
	errDeviceUnavailable     = pmv1.ErrorCode_ERROR_CODE_DEVICE_UNAVAILABLE
	errActionNotFound        = pmv1.ErrorCode_ERROR_CODE_ACTION_NOT_FOUND
	errActionSetMissing      = pmv1.ErrorCode_ERROR_CODE_ACTION_SET_NOT_FOUND
	errDefinitionMissing     = pmv1.ErrorCode_ERROR_CODE_DEFINITION_NOT_FOUND
	errDeviceGroupMissing    = pmv1.ErrorCode_ERROR_CODE_DEVICE_GROUP_NOT_FOUND
	errAssignedSourceMissing = pmv1.ErrorCode_ERROR_CODE_ASSIGNMENT_SOURCE_NOT_FOUND
	errInternal              = pmv1.ErrorCode_ERROR_CODE_INTERNAL_ERROR
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
