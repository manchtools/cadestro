package enrollment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/manchtools/cadestro/contract"
	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/ca"
	"github.com/manchtools/cadestro/server/internal/mtls"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

var errCredentialRejected = errors.New("enrollment credential rejected")

type Config struct {
	Store      *store.Store
	CA         *ca.CA
	Logger     *slog.Logger
	Now        func() time.Time
	ControlURL string
}

type Handlers struct {
	store      *store.Store
	ca         *ca.CA
	logger     *slog.Logger
	now        func() time.Time
	controlURL string
}

func New(cfg Config) *Handlers {
	if cfg.Store == nil || cfg.CA == nil {
		panic("enrollment: store and CA are required")
	}
	if err := contract.ValidateHTTPSURL(cfg.ControlURL); err != nil {
		panic(fmt.Sprintf("enrollment: invalid control URL: %v", err))
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Handlers{
		store: cfg.Store, ca: cfg.CA, logger: cfg.Logger, now: cfg.Now,
		controlURL: cfg.ControlURL,
	}
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

func (h *Handlers) Register(ctx context.Context, req *connect.Request[cadestrov1.RegisterRequest]) (*connect.Response[cadestrov1.RegisterResponse], error) {
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

		existing, findErr := tx.FindEnrollmentDevice(ctx, db.FindEnrollmentDeviceParams{
			ValueHash: tokenFingerprint, ReservedName: store.BootstrapAdminTokenName,
			EnrolledAt: now, IdentityPublicKey: identityKey,
		})
		if findErr == nil {
			device = existing
			if len(device.CertificatePem) > 0 {
				cert = &ca.Certificate{CertPEM: append([]byte(nil), device.CertificatePem...)}
			} else {
				issued, issueErr := h.ca.IssueCertificateFromCSR(device.ID, req.Msg.Csr)
				if issueErr != nil {
					return fmt.Errorf("issue retry enrollment certificate: %w", issueErr)
				}
				cert = issued
				serial, serialErr := ca.SerialFromPEM(issued.CertPEM)
				if serialErr != nil {
					return fmt.Errorf("read enrollment certificate serial: %w", serialErr)
				}
				if _, issueErr = tx.SetActiveDeviceCertificate(ctx, db.SetActiveDeviceCertificateParams{
					ID: device.ID, CertificatePem: issued.CertPEM, Serial: &serial,
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
			IdentityPublicKey: identityKey, EnrolledAt: &now, ValueHash: tokenFingerprint,
			ReservedName: store.BootstrapAdminTokenName,
		})
		if store.IsNotFound(insertErr) {
			return errCredentialRejected
		}
		if insertErr != nil {

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
		serial, serialErr := ca.SerialFromPEM(issued.CertPEM)
		if serialErr != nil {
			return fmt.Errorf("read enrollment certificate serial: %w", serialErr)
		}
		if _, issueErr = tx.SetActiveDeviceCertificate(ctx, db.SetActiveDeviceCertificateParams{
			ID: device.ID, CertificatePem: issued.CertPEM, Serial: &serial,
		}); issueErr != nil {
			return fmt.Errorf("store enrollment certificate: %w", issueErr)
		}
		rec.Effect(store.AuditEffect{ResourceType: "device", ResourceID: device.ID, Action: "CREATE",
			Outcome: store.EffectApplied, ChangedFields: []string{"hostname", "agent_version", "enrollment_identity_public_key", "active_cert_serial", "registration_token_id"}})
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
	return connect.NewResponse(&cadestrov1.RegisterResponse{
		DeviceId: &cadestrov1.DeviceId{Value: device.ID}, CaCert: h.ca.CACertPEM(),
		Certificate: cert.CertPEM, ControlUrl: h.controlURL,
	}), nil
}

func (h *Handlers) RenewCertificate(ctx context.Context, req *connect.Request[cadestrov1.RenewCertificateRequest]) (*connect.Response[cadestrov1.RenewCertificateResponse], error) {
	peer, ok := mtls.PeerCertificateFromContext(ctx)
	if !ok {
		return nil, h.rejectCertificate(ctx, req, "MISSING_TLS_PEER")
	}
	deviceID, ok := mtls.DeviceIDFromContext(ctx)
	if !ok {
		return nil, h.rejectCertificate(ctx, req, "MISSING_TLS_IDENTITY")
	}
	presentedClass, err := mtls.PeerClassFromCert(peer)
	if err != nil || presentedClass != mtls.PeerClassAgent {
		return nil, h.rejectCertificate(ctx, req, "INVALID_DEVICE_CERTIFICATE")
	}
	if err := ca.AssertCSRMatchesCert(peer, req.Msg.Csr); err != nil {
		return nil, h.rejectCertificate(ctx, req, "CERTIFICATE_KEY_MISMATCH")
	}
	if _, err := ca.EnrollmentIdentityFromCSR(req.Msg.Csr); err != nil {
		return nil, rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument, "invalid certificate signing request")
	}
	peerSerial, err := ca.SerialFromCert(peer)
	if err != nil {
		return nil, h.rejectCertificate(ctx, req, "INVALID_DEVICE_CERTIFICATE")
	}
	op := store.AuditOperation{
		Class: store.ClassMutation, ActorType: "device", ActorID: deviceID,
		ActorFingerprint: ca.FingerprintFromCert(peer), Origin: auth.ControlRPCOrigin,
		OriginFingerprint:    originFingerprint(req),
		RequestDescriptor:    cadestrov1connect.ControlServiceRenewCertificateProcedure,
		AuthorizationOutcome: store.AuthorizationAllowed,
		AuthorizationDetail:  "device_certificate", Result: store.ResultSuccess, ResultCode: "OK",
	}
	var newCert *ca.Certificate
	_, err = h.store.WithAudit(ctx, op, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		current, err := tx.GetDevice(ctx, deviceID)
		if err != nil {
			return err
		}
		if current.ActiveCertSerial == nil || *current.ActiveCertSerial != peerSerial {
			return errCredentialRejected
		}
		if current.PendingCertSerial != nil {
			if len(current.PendingCertificatePem) == 0 {
				return errors.New("pending certificate is incomplete")
			}
			pendingPEM := append([]byte(nil), current.PendingCertificatePem...)
			notAfter, err := ca.NotAfterFromPEM(pendingPEM)
			if err != nil {
				return err
			}
			newCert = &ca.Certificate{CertPEM: pendingPEM, NotAfter: notAfter}
			return nil
		}
		issued, err := h.ca.IssueCertificateFromCSR(deviceID, req.Msg.Csr)
		if err != nil {
			return err
		}
		newCert = issued
		pendingSerial, err := ca.SerialFromPEM(issued.CertPEM)
		if err != nil {
			return err
		}
		if _, err := tx.SetPendingDeviceCertificate(ctx, db.SetPendingDeviceCertificateParams{
			ID: deviceID, ActiveSerial: &peerSerial, CertificatePem: issued.CertPEM,
			Serial: &pendingSerial,
		}); store.IsNotFound(err) {
			return errCredentialRejected
		} else if err != nil {
			return fmt.Errorf("store pending certificate: %w", err)
		}
		rec.Effect(store.AuditEffect{
			ResourceType: "device", ResourceID: deviceID, Action: "UPDATE",
			Outcome: store.EffectApplied, ChangedFields: []string{"pending_cert_serial"},
			EvidenceKind: "certificate", EvidenceFingerprint: issued.Fingerprint,
		})
		return nil
	})
	if errors.Is(err, errCredentialRejected) {
		return nil, h.rejectCertificate(ctx, req, "CERTIFICATE_NOT_CURRENT")
	}
	if err != nil {
		return nil, h.internal(ctx, "commit certificate renewal", err)
	}
	if newCert == nil || len(newCert.CertPEM) == 0 {
		return nil, h.internal(ctx, "commit certificate renewal", errors.New("pending certificate missing"))
	}
	return connect.NewResponse(&cadestrov1.RenewCertificateResponse{
		Certificate: newCert.CertPEM, NotAfter: timestamppb.New(newCert.NotAfter),
	}), nil
}

func (h *Handlers) rejectCertificate(ctx context.Context, req *connect.Request[cadestrov1.RenewCertificateRequest], reason string) error {
	fingerprint := ""
	if peer, ok := mtls.PeerCertificateFromContext(ctx); ok {
		fingerprint = ca.FingerprintFromCert(peer)
	} else {
		digest := sha256.Sum256(req.Msg.Csr)
		fingerprint = hex.EncodeToString(digest[:])
	}
	if err := h.recordRejected(ctx, req, cadestrov1connect.ControlServiceRenewCertificateProcedure, fingerprint, reason); err != nil {
		return h.internal(ctx, "audit rejected renewal", err)
	}
	return rpcError(ctx, errPermissionDenied, connect.CodePermissionDenied, "certificate not recognized")
}
