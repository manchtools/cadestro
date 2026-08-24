package execution

import (
	"context"
	"errors"

	"buf.build/go/protovalidate"
	"github.com/oklog/ulid/v2"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/store"
)

var ErrInvalidInput = errors.New("invalid execution result")

type Config struct {
	Store *store.Store
}
type Service struct {
	store     *store.Store
	validator protovalidate.Validator
}

func New(cfg Config) *Service {
	if cfg.Store == nil {
		panic("execution: store is required")
	}
	return &Service{store: cfg.Store, validator: protovalidate.GlobalValidator}
}

func (s *Service) ApplyActionResult(ctx context.Context, deviceID string, result *cadestrov1.ActionResult) error {
	if ctx == nil || !validID(deviceID) || result == nil || result.ActionId == nil ||
		!validID(result.ActionId.Value) || !validID(result.RunId) || !validID(result.OccurrenceId) {
		return ErrInvalidInput
	}
	if err := s.validator.Validate(result); err != nil || len(result.Metadata) != 0 {
		return ErrInvalidInput
	}
	if _, terminal := resultStatus(result.Status); !terminal {
		return ErrInvalidInput
	}
	return s.store.RecordPolicyActionResult(ctx, deviceID, result)
}

func validID(id string) bool {
	_, err := ulid.ParseStrict(id)
	return err == nil
}

func resultStatus(status cadestrov1.ExecutionStatus) (string, bool) {
	switch status {
	case cadestrov1.ExecutionStatus_EXECUTION_STATUS_SUCCESS:
		return "success", true
	case cadestrov1.ExecutionStatus_EXECUTION_STATUS_FAILED:
		return "failed", true
	case cadestrov1.ExecutionStatus_EXECUTION_STATUS_SKIPPED:
		return "skipped", true
	case cadestrov1.ExecutionStatus_EXECUTION_STATUS_TIMEOUT:
		return "timeout", true
	case cadestrov1.ExecutionStatus_EXECUTION_STATUS_NOT_APPLICABLE:
		return "not_applicable", true
	case cadestrov1.ExecutionStatus_EXECUTION_STATUS_INDETERMINATE:
		return "indeterminate", true
	default:
		return "", false
	}
}
