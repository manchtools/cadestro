package authoring

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	pmv1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/middleware"
)

const (
	errNotAuthenticated         = pmv1.ErrorCode_ERROR_CODE_NOT_AUTHENTICATED
	errPermissionDenied         = pmv1.ErrorCode_ERROR_CODE_PERMISSION_DENIED
	errValidationFailed         = pmv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED
	errInvalidPageToken         = pmv1.ErrorCode_ERROR_CODE_INVALID_PAGE_TOKEN
	errInternal                 = pmv1.ErrorCode_ERROR_CODE_INTERNAL_ERROR
	errActionNotFound           = pmv1.ErrorCode_ERROR_CODE_ACTION_NOT_FOUND
	errActionSetNotFound        = pmv1.ErrorCode_ERROR_CODE_ACTION_SET_NOT_FOUND
	errActionAlreadyInSet       = pmv1.ErrorCode_ERROR_CODE_ACTION_ALREADY_IN_SET
	errActionSetMemberNotFound  = pmv1.ErrorCode_ERROR_CODE_ACTION_SET_MEMBER_NOT_FOUND
	errDefinitionNotFound       = pmv1.ErrorCode_ERROR_CODE_DEFINITION_NOT_FOUND
	errActionSetAlreadyInDef    = pmv1.ErrorCode_ERROR_CODE_ACTION_SET_ALREADY_IN_DEFINITION
	errDefinitionMemberNotFound = pmv1.ErrorCode_ERROR_CODE_DEFINITION_MEMBER_NOT_FOUND
	errCannotModifySystemAction = pmv1.ErrorCode_ERROR_CODE_CANNOT_MODIFY_SYSTEM_ACTION
)

func authoringRPCError(ctx context.Context, code pmv1.ErrorCode, connectCode connect.Code, message string) *connect.Error {
	err := connect.NewError(connectCode, errors.New(message))
	detail, detailErr := connect.NewErrorDetail(&pmv1.ErrorDetail{
		Code: code, RequestId: middleware.RequestIDFromContext(ctx),
	})
	if detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

func authoringNotFound(ctx context.Context, code pmv1.ErrorCode, message string) *connect.Error {
	return authoringRPCError(ctx, code, connect.CodeNotFound, message)
}
