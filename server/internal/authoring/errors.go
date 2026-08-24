package authoring

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/middleware"
)

const (
	errNotAuthenticated         = cadestrov1.ErrorCode_ERROR_CODE_NOT_AUTHENTICATED
	errPermissionDenied         = cadestrov1.ErrorCode_ERROR_CODE_PERMISSION_DENIED
	errValidationFailed         = cadestrov1.ErrorCode_ERROR_CODE_VALIDATION_FAILED
	errInvalidPageToken         = cadestrov1.ErrorCode_ERROR_CODE_INVALID_PAGE_TOKEN
	errInternal                 = cadestrov1.ErrorCode_ERROR_CODE_INTERNAL_ERROR
	errActionNotFound           = cadestrov1.ErrorCode_ERROR_CODE_ACTION_NOT_FOUND
	errActionSetNotFound        = cadestrov1.ErrorCode_ERROR_CODE_ACTION_SET_NOT_FOUND
	errActionAlreadyInSet       = cadestrov1.ErrorCode_ERROR_CODE_ACTION_ALREADY_IN_SET
	errActionSetMemberNotFound  = cadestrov1.ErrorCode_ERROR_CODE_ACTION_SET_MEMBER_NOT_FOUND
	errDefinitionNotFound       = cadestrov1.ErrorCode_ERROR_CODE_DEFINITION_NOT_FOUND
	errActionSetAlreadyInDef    = cadestrov1.ErrorCode_ERROR_CODE_ACTION_SET_ALREADY_IN_DEFINITION
	errDefinitionMemberNotFound = cadestrov1.ErrorCode_ERROR_CODE_DEFINITION_MEMBER_NOT_FOUND
	errCannotModifySystemAction = cadestrov1.ErrorCode_ERROR_CODE_CANNOT_MODIFY_SYSTEM_ACTION
)

func authoringRPCError(ctx context.Context, code cadestrov1.ErrorCode, connectCode connect.Code, message string) *connect.Error {
	err := connect.NewError(connectCode, errors.New(message))
	detail, detailErr := connect.NewErrorDetail(&cadestrov1.ErrorDetail{
		Code: code, RequestId: middleware.RequestIDFromContext(ctx),
	})
	if detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

func authoringNotFound(ctx context.Context, code cadestrov1.ErrorCode, message string) *connect.Error {
	return authoringRPCError(ctx, code, connect.CodeNotFound, message)
}
