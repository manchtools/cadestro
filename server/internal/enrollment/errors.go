// Package enrollment implements direct audited device enrollment and
// certificate renewal.
package enrollment

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/middleware"
)

const (
	errValidationFailed = cadestrov1.ErrorCode_ERROR_CODE_VALIDATION_FAILED
	errPermissionDenied = cadestrov1.ErrorCode_ERROR_CODE_PERMISSION_DENIED
	errInternal         = cadestrov1.ErrorCode_ERROR_CODE_INTERNAL_ERROR
)

func rpcError(ctx context.Context, code cadestrov1.ErrorCode, connectCode connect.Code, message string) *connect.Error {
	err := connect.NewError(connectCode, errors.New(message))
	detail, detailErr := connect.NewErrorDetail(&cadestrov1.ErrorDetail{
		Code: code, RequestId: middleware.RequestIDFromContext(ctx),
	})
	if detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}
