package auth

import (
	"context"
	"fmt"

	"github.com/manchtools/cadestro/server/internal/store"
)

type AuditOperationRecorder interface {
	RecordOperation(ctx context.Context, op store.AuditOperation, effects ...store.AuditEffect) (store.AuditRecord, error)
}

const AnonymousActorType = "anonymous"

const ControlRPCOrigin = "control_rpc"

func NewRejectionRecorder(st AuditOperationRecorder) RejectionRecorder {
	return &storeRejectionRecorder{st: st}
}

type storeRejectionRecorder struct{ st AuditOperationRecorder }

func (r *storeRejectionRecorder) RecordRejectedAuthentication(ctx context.Context, att RejectedAuthentication) error {
	_, err := r.st.RecordOperation(ctx, store.AuditOperation{
		Class:                store.ClassRejectedAuthentication,
		ActorType:            AnonymousActorType,
		ActorFingerprint:     att.CredentialFingerprint,
		Origin:               ControlRPCOrigin,
		OriginFingerprint:    att.OriginFingerprint,
		RequestDescriptor:    att.Procedure,
		AuthorizationOutcome: store.AuthorizationDenied,
		Result:               store.ResultRejected,
		ResultCode:           att.Reason,
	})
	if err != nil {
		return fmt.Errorf("record rejected authentication: %w", err)
	}
	return nil
}
