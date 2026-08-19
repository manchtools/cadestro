package enrollment

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	"github.com/go-playground/validator/v10"
	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/types/known/timestamppb"

	pmv1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	sdkvalidate "github.com/manchtools/cadestro/contract/validate"
	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/ca"
	"github.com/manchtools/cadestro/server/internal/mtls"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

var errCredentialRejected = errors.New("enrollment credential rejected")

// Config supplies enrollment's direct SQLite and PKI dependencies.
type Config struct {
	Store                   *store.Store
	CA                      *ca.CA
	Logger                  *slog.Logger
	Now                     func() time.Time
	ControlURL              string
	ControlSealingPublicKey []byte
	CloseStream             func(deviceID string)
}

// Handlers implements first enrollment and certificate renewal.
type Handlers struct {
	store                   *store.Store
	ca                      *ca.CA
	logger                  *slog.Logger
	now                     func() time.Time
	controlURL              string
	controlSealingPublicKey []byte
	closeStream             func(string)
	validator               *validator.Validate
}

// New constructs enrollment handlers and rejects incomplete boot wiring.
func New(cfg Config) *Handlers {
	if cfg.Store == nil || cfg.CA == nil {
		panic("enrollment: store and CA are required")
	}
	if err := ValidateControlURL(cfg.ControlURL); err != nil {
		panic(fmt.Sprintf("enrollment: invalid control URL: %v", err))
	}
	if len(cfg.ControlSealingPublicKey) != 32 {
		panic("enrollment: 32-byte control sealing public key is required")
	}
	if cfg.CloseStream == nil {
		panic("enrollment: stream closer is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Handlers{
		store: cfg.Store, ca: cfg.CA, logger: cfg.Logger, now: cfg.Now,
		controlURL:              cfg.ControlURL,
		controlSealingPublicKey: append([]byte(nil), cfg.ControlSealingPublicKey...),
		closeStream:             cfg.CloseStream, validator: sdkvalidate.NewValidator(),
	}
}

func validateRequest[T any](h *Handlers, ctx context.Context, req *connect.Request[T]) error {
	if req == nil || req.Msg == nil {
		return rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument, "request is required")
	}
	if detail, ok := sdkvalidate.Struct(h.validator, req.Msg); !ok {
		return rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument, detail)
	}
	return nil
}

func (h *Handlers) internal(ctx context.Context, operation string, err error) *connect.Error {
	h.logger.Error("enrollment RPC failed", "operation", operation, "error", err)
	return rpcError(ctx, errInternal, connect.CodeInternal, "internal error")
}

func originFingerprint(req connect.AnyRequest) string {
	if ip := auth.ClientIP(req); ip != "" {
		return auth.Fingerprint(ip)
	}
	return ""
}

func (h *Handlers) recordRejected(ctx context.Context, req connect.AnyRequest, procedure, fingerprint, reason string) error {
	_, err := h.store.RecordOperation(ctx, store.AuditOperation{
		Class: store.ClassRejectedAuthentication, ActorType: auth.AnonymousActorType,
		ActorFingerprint: fingerprint, Origin: auth.ControlRPCOrigin,
		OriginFingerprint: originFingerprint(req), RequestDescriptor: procedure,
		AuthorizationOutcome: store.AuthorizationDenied,
		Result:               store.ResultRejected, ResultCode: reason,
	})
	return err
}

// Register binds one reusable token to one canonical Ed25519 device identity.
// The device relation is the token's durable use record; no mutable token
// counter or human ownership is involved.
func (h *Handlers) Register(ctx context.Context, req *connect.Request[pmv1.RegisterRequest]) (*connect.Response[pmv1.RegisterResponse], error) {
	if err := validateRequest(h, ctx, req); err != nil {
		return nil, err
	}
	identityKey, err := ca.EnrollmentIdentityFromCSR(req.Msg.Csr)
	if err != nil {
		return nil, rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument, "invalid certificate signing request")
	}
	tokenDigest := sha256.Sum256([]byte(req.Msg.Token))
	tokenFingerprint := hex.EncodeToString(tokenDigest[:])
	now := h.now().UTC()
	op := store.AuditOperation{
		Class: store.ClassMutation, ActorType: "registration_token",
		ActorFingerprint: tokenFingerprint, Origin: auth.ControlRPCOrigin,
		OriginFingerprint:    originFingerprint(req),
		RequestDescriptor:    cadestrov1connect.ControlServiceRegisterProcedure,
		AuthorizationOutcome: store.AuthorizationAllowed,
		AuthorizationDetail:  "registration_token", Result: store.ResultSuccess, ResultCode: "OK",
	}
	var device db.Device
	var cert *ca.Certificate
	_, err = h.store.WithAudit(ctx, op, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		// Validate token usability even on a retry. Revocation and expiry must
		// never be bypassed by an identity that enrolled earlier.
		existing, findErr := tx.FindEnrollmentDevice(ctx, db.FindEnrollmentDeviceParams{
			ValueHash: tokenFingerprint, ReservedName: store.BootstrapAdminTokenName,
			EnrolledAt: now, IdentityPublicKey: identityKey,
		})
		if findErr == nil {
			if subtle.ConstantTimeCompare(existing.AgentSealingPublicKey, req.Msg.AgentSealingPublicKey) != 1 {
				return errCredentialRejected
			}
			device = existing
			if len(device.CertificatePem) > 0 {
				cert = &ca.Certificate{CertPEM: append([]byte(nil), device.CertificatePem...)}
			} else {
				issued, issueErr := h.ca.IssueCertificateFromCSR(device.ID, req.Msg.Csr)
				if issueErr != nil {
					return fmt.Errorf("issue retry enrollment certificate: %w", issueErr)
				}
				cert = issued
				if _, issueErr = tx.ReplaceDeviceCertificate(ctx, db.ReplaceDeviceCertificateParams{
					ID: device.ID, NewCertificatePem: issued.CertPEM, NewFingerprint: &issued.Fingerprint,
					NewNotAfter: &issued.NotAfter, OldFingerprint: nil,
				}); issueErr != nil {
					return fmt.Errorf("store retry enrollment certificate: %w", issueErr)
				}
			}
			return nil
		}
		if !store.IsNotFound(findErr) {
			return fmt.Errorf("find enrollment device: %w", findErr)
		}

		deviceID := ulid.Make().String()
		created, insertErr := tx.InsertEnrolledDevice(ctx, db.InsertEnrolledDeviceParams{
			ID: deviceID, Hostname: req.Msg.Hostname, AgentVersion: req.Msg.AgentVersion,
			SealingKey:        append([]byte(nil), req.Msg.AgentSealingPublicKey...),
			IdentityPublicKey: identityKey, EnrolledAt: &now, ValueHash: tokenFingerprint,
			ReservedName: store.BootstrapAdminTokenName,
		})
		if store.IsNotFound(insertErr) {
			return errCredentialRejected
		}
		if insertErr != nil {
			// Any conditional insert conflict means this attempt lost a
			// concurrent enrollment decision. Re-read/retry is intentionally
			// not implicit here: a soft-deleted identity must never resurrect,
			// and a caller with a different key must not receive another
			// certificate from a failed reservation.
			if store.IsConflict(insertErr) {
				return errCredentialRejected
			}
			return fmt.Errorf("reserve enrollment token: %w", insertErr)
		}
		device = created
		issued, issueErr := h.ca.IssueCertificateFromCSR(device.ID, req.Msg.Csr)
		if issueErr != nil {
			return fmt.Errorf("issue enrollment certificate: %w", issueErr)
		}
		cert = issued
		if _, issueErr = tx.ReplaceDeviceCertificate(ctx, db.ReplaceDeviceCertificateParams{
			ID: device.ID, NewCertificatePem: issued.CertPEM, NewFingerprint: &issued.Fingerprint,
			NewNotAfter: &issued.NotAfter, OldFingerprint: nil,
		}); issueErr != nil {
			return fmt.Errorf("store enrollment certificate: %w", issueErr)
		}
		rec.Effect(store.AuditEffect{ResourceType: "device", ResourceID: device.ID, Action: "CREATE",
			Outcome: store.EffectApplied, ChangedFields: []string{"hostname", "agent_version", "enrollment_identity_public_key", "cert_fingerprint", "cert_not_after", "registration_token_id"}})
		return nil
	})
	if errors.Is(err, errCredentialRejected) {
		if auditErr := h.recordRejected(ctx, req, cadestrov1connect.ControlServiceRegisterProcedure, tokenFingerprint, "INVALID_REGISTRATION_TOKEN"); auditErr != nil {
			return nil, h.internal(ctx, "audit rejected registration", auditErr)
		}
		return nil, rpcError(ctx, errPermissionDenied, connect.CodePermissionDenied, "invalid registration token")
	}
	if err != nil {
		return nil, h.internal(ctx, "commit enrollment", err)
	}
	if cert == nil || len(cert.CertPEM) == 0 {
		return nil, h.internal(ctx, "commit enrollment", errors.New("certificate missing after enrollment"))
	}
	return connect.NewResponse(&pmv1.RegisterResponse{
		DeviceId: &pmv1.DeviceId{Value: device.ID}, CaCert: h.ca.CACertPEM(),
		Certificate: cert.CertPEM, ControlUrl: h.controlURL,
		ControlSealingPublicKey: append([]byte(nil), h.controlSealingPublicKey...),
	}), nil
}

// RenewCertificate atomically advances the tracked identity and revokes its
// predecessor. The conditional update absorbs concurrent renewal attempts.
func (h *Handlers) RenewCertificate(ctx context.Context, req *connect.Request[pmv1.RenewCertificateRequest]) (*connect.Response[pmv1.RenewCertificateResponse], error) {
	if err := validateRequest(h, ctx, req); err != nil {
		return nil, err
	}
	deviceID, err := h.ca.VerifyCertificate(req.Msg.CurrentCertificate)
	if err != nil {
		return nil, h.rejectCertificate(ctx, req, "INVALID_DEVICE_CERTIFICATE")
	}
	presentedClass, err := ca.PeerClassFromPEM(req.Msg.CurrentCertificate)
	if err != nil || presentedClass != mtls.PeerClassAgent {
		return nil, h.rejectCertificate(ctx, req, "INVALID_DEVICE_CERTIFICATE")
	}
	if err := ca.AssertCSRMatchesCertKey(req.Msg.CurrentCertificate, req.Msg.Csr); err != nil {
		return nil, h.rejectCertificate(ctx, req, "CERTIFICATE_KEY_MISMATCH")
	}
	newCert, err := h.ca.IssueCertificateFromCSR(deviceID, req.Msg.Csr)
	if err != nil {
		if errors.Is(err, ca.ErrInvalidCSR) {
			return nil, rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument, "invalid certificate signing request")
		}
		return nil, h.internal(ctx, "issue renewed certificate", err)
	}
	oldFingerprint, err := ca.FingerprintFromPEM(req.Msg.CurrentCertificate)
	if err != nil {
		return nil, h.internal(ctx, "fingerprint current certificate", err)
	}
	oldNotAfter, err := ca.NotAfterFromPEM(req.Msg.CurrentCertificate)
	if err != nil {
		return nil, h.internal(ctx, "read current certificate expiry", err)
	}
	newFingerprint, newNotAfter := newCert.Fingerprint, newCert.NotAfter
	op := store.AuditOperation{
		Class: store.ClassMutation, ActorType: "device", ActorID: deviceID,
		ActorFingerprint: oldFingerprint, Origin: auth.ControlRPCOrigin,
		OriginFingerprint:    originFingerprint(req),
		RequestDescriptor:    cadestrov1connect.ControlServiceRenewCertificateProcedure,
		AuthorizationOutcome: store.AuthorizationAllowed,
		AuthorizationDetail:  "device_certificate", Result: store.ResultSuccess, ResultCode: "OK",
	}
	_, err = h.store.WithAudit(ctx, op, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		if _, err := tx.ReplaceDeviceCertificate(ctx, db.ReplaceDeviceCertificateParams{
			NewCertificatePem: nil, NewFingerprint: &newFingerprint, NewNotAfter: &newNotAfter,
			ID: deviceID, OldFingerprint: &oldFingerprint,
		}); store.IsNotFound(err) {
			return errCredentialRejected
		} else if err != nil {
			return fmt.Errorf("replace device certificate: %w", err)
		}
		if err := store.RevokeInTx(ctx, tx, oldFingerprint, oldNotAfter, "superseded by renewal"); err != nil {
			return err
		}
		rec.Effect(store.AuditEffect{
			ResourceType: "device", ResourceID: deviceID, Action: "UPDATE",
			Outcome: store.EffectApplied, ChangedFields: []string{"cert_fingerprint", "cert_not_after"},
			EvidenceKind: "certificate", EvidenceFingerprint: newFingerprint,
		})
		rec.Effect(store.AuditEffect{
			ResourceType: "device_certificate", ResourceID: deviceID, Action: "REVOKE",
			Outcome: store.EffectApplied, EvidenceKind: "certificate", EvidenceFingerprint: oldFingerprint,
		})
		return nil
	})
	if errors.Is(err, errCredentialRejected) {
		return nil, h.rejectCertificate(ctx, req, "CERTIFICATE_NOT_CURRENT")
	}
	if err != nil {
		return nil, h.internal(ctx, "commit certificate renewal", err)
	}
	h.closeStream(deviceID)
	return connect.NewResponse(&pmv1.RenewCertificateResponse{
		Certificate: newCert.CertPEM, NotAfter: timestamppb.New(newCert.NotAfter),
		CaCertificate: h.ca.CACertPEM(),
	}), nil
}

func (h *Handlers) rejectCertificate(ctx context.Context, req *connect.Request[pmv1.RenewCertificateRequest], reason string) error {
	digest := sha256.Sum256(req.Msg.CurrentCertificate)
	if err := h.recordRejected(ctx, req, cadestrov1connect.ControlServiceRenewCertificateProcedure, hex.EncodeToString(digest[:]), reason); err != nil {
		return h.internal(ctx, "audit rejected renewal", err)
	}
	return rpcError(ctx, errPermissionDenied, connect.CodePermissionDenied, "certificate not recognized")
}
