package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/types/known/timestamppb"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/ca"
	"github.com/manchtools/cadestro/server/internal/mtls"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

func tokenHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func (service *Service) Register(ctx context.Context, request *connect.Request[cadestrov1.RegisterRequest]) (*connect.Response[cadestrov1.RegisterResponse], error) {
	identityKey, err := ca.EnrollmentIdentityFromCSR(request.Msg.GetCsr())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid certificate signing request"))
	}
	existing, err := service.store.Queries().FindDeviceByIdentityKey(ctx, identityKey)
	if err == nil {
		return connect.NewResponse(&cadestrov1.RegisterResponse{
			DeviceId: &cadestrov1.DeviceId{Value: existing.ID}, CaCert: service.ca.CACertPEM(),
			Certificate: existing.ActiveCertificatePem, ControlUrl: service.agentURL,
		}), nil
	}
	if !store.IsNotFound(err) {
		return nil, service.internal("find enrollment device", err)
	}
	now := service.now().UTC()
	token, err := service.store.Queries().GetUsableRegistrationToken(ctx, db.GetUsableRegistrationTokenParams{ValueHash: tokenHash(request.Msg.GetToken()), ExpiresAt: now})
	if err != nil {
		if store.IsNotFound(err) {
			return nil, connect.NewError(connect.CodePermissionDenied, errors.New("invalid registration token"))
		}
		return nil, service.internal("get registration token", err)
	}
	deviceID := ulid.Make().String()
	certificate, err := service.ca.IssueCertificateFromCSR(deviceID, request.Msg.GetCsr())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid certificate signing request"))
	}
	serial, err := ca.SerialFromPEM(certificate.CertPEM)
	if err != nil {
		return nil, service.internal("read issued certificate", err)
	}
	err = service.store.Transaction(ctx, func(queries *db.Queries) error {
		if _, err := consumeRegistrationToken(ctx, queries, token, now); err != nil {
			return err
		}
		if _, err := queries.CreateDevice(ctx, db.CreateDeviceParams{
			ID: deviceID, Hostname: request.Msg.GetHostname(), AgentVersion: request.Msg.GetAgentVersion(),
			IdentityPublicKey: identityKey, ActiveCertificatePem: certificate.CertPEM, ActiveCertSerial: serial,
			CertExpiresAt: certificate.NotAfter, RegisteredAt: now,
		}); err != nil {
			return err
		}
		return queries.CreateAuditEvent(ctx, db.CreateAuditEventParams{
			ID: ulid.Make().String(), EventType: "device.registered", StreamType: "device", StreamID: deviceID,
			ActorType: "registration_token", ActorID: token.ID, OccurredAt: now,
		})
	})
	if err != nil {
		if store.IsConflict(err) {
			return nil, connect.NewError(connect.CodePermissionDenied, errors.New("invalid registration token"))
		}
		return nil, service.internal("register device", err)
	}
	return connect.NewResponse(&cadestrov1.RegisterResponse{
		DeviceId: &cadestrov1.DeviceId{Value: deviceID}, CaCert: service.ca.CACertPEM(),
		Certificate: certificate.CertPEM, ControlUrl: service.agentURL,
	}), nil
}

func consumeRegistrationToken(ctx context.Context, queries *db.Queries, token *db.RegistrationToken, now time.Time) (*db.RegistrationToken, error) {
	consumed, err := queries.ConsumeRegistrationToken(ctx, db.ConsumeRegistrationTokenParams{ID: token.ID, ExpiresAt: now})
	if store.IsNotFound(err) {
		return nil, errors.New("registration token was consumed concurrently")
	}
	if err != nil {
		return nil, err
	}
	if consumed.MaxUses > 0 && consumed.CurrentUses >= consumed.MaxUses {
		if _, err := queries.DeleteRegistrationToken(ctx, consumed.ID); err != nil {
			return nil, err
		}
	}
	return consumed, nil
}

func (service *Service) RenewCertificate(ctx context.Context, request *connect.Request[cadestrov1.RenewCertificateRequest]) (*connect.Response[cadestrov1.RenewCertificateResponse], error) {
	peer, peerOK := mtls.PeerCertificateFromContext(ctx)
	deviceID, deviceOK := mtls.DeviceIDFromContext(ctx)
	if !peerOK || !deviceOK {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("certificate not recognized"))
	}
	peerClass, err := mtls.PeerClassFromCert(peer)
	if err != nil || peerClass != mtls.PeerClassAgent || ca.AssertCSRMatchesCert(peer, request.Msg.GetCsr()) != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("certificate not recognized"))
	}
	peerSerial, err := ca.SerialFromCert(peer)
	if err != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("certificate not recognized"))
	}
	device, err := service.store.Queries().GetDevice(ctx, deviceID)
	if err != nil || device.ActiveCertSerial != peerSerial {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("certificate not recognized"))
	}
	if device.PendingCertSerial != nil {
		if device.PendingCertExpiresAt == nil || len(device.PendingCertificatePem) == 0 {
			return nil, service.internal("read pending certificate", errors.New("pending certificate is incomplete"))
		}
		return connect.NewResponse(&cadestrov1.RenewCertificateResponse{
			Certificate: device.PendingCertificatePem, NotAfter: timestamppb.New(*device.PendingCertExpiresAt),
		}), nil
	}
	certificate, err := service.ca.IssueCertificateFromCSR(deviceID, request.Msg.GetCsr())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid certificate signing request"))
	}
	serial, err := ca.SerialFromPEM(certificate.CertPEM)
	if err != nil {
		return nil, service.internal("read renewed certificate", err)
	}
	rows, err := service.store.Queries().SetPendingDeviceCertificate(ctx, db.SetPendingDeviceCertificateParams{
		PendingCertificatePem: certificate.CertPEM, PendingCertSerial: &serial, PendingCertExpiresAt: &certificate.NotAfter,
		ID: deviceID, ActiveCertSerial: peerSerial,
	})
	if err != nil {
		return nil, service.internal("store renewed certificate", err)
	}
	if rows != 1 {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("certificate not recognized"))
	}
	if err := service.audit(ctx, "device.certificate_renewed", "device", deviceID, "device", deviceID); err != nil {
		return nil, service.internal("audit certificate renewal", err)
	}
	return connect.NewResponse(&cadestrov1.RenewCertificateResponse{Certificate: certificate.CertPEM, NotAfter: timestamppb.New(certificate.NotAfter)}), nil
}
