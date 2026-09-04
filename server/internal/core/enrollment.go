package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/ca"
	"github.com/manchtools/cadestro/server/internal/mtls"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

var (
	errCertificateNotRecognized  = errors.New("certificate not recognized")
	errInvalidCertificateRequest = errors.New("invalid certificate signing request")
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
			ID: ulid.Make().String(), EventType: cadestrov1.AuditEventType_AUDIT_EVENT_TYPE_DEVICE_REGISTERED, StreamType: cadestrov1.AuditStreamType_AUDIT_STREAM_TYPE_DEVICE, StreamID: deviceID,
			ActorType: cadestrov1.AuditActorType_AUDIT_ACTOR_TYPE_REGISTRATION_TOKEN, ActorID: token.ID, OccurredAt: now,
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
	if token.MaxUses > 0 && token.CurrentUses+1 >= token.MaxUses {
		consumed, err := queries.ConsumeFinalRegistrationToken(ctx, db.ConsumeFinalRegistrationTokenParams{ID: token.ID, ExpiresAt: now})
		if store.IsNotFound(err) {
			return nil, errors.New("registration token was consumed concurrently")
		}
		if err != nil {
			return nil, err
		}
		consumed.CurrentUses++
		return consumed, nil
	}
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
	var certificate []byte
	err = service.store.Transaction(ctx, func(queries *db.Queries) error {
		device, err := queries.GetDevice(ctx, deviceID)
		if err != nil {
			if store.IsNotFound(err) {
				return errCertificateNotRecognized
			}
			return fmt.Errorf("get renewal device: %w", err)
		}
		if device.ActiveCertSerial != peerSerial {
			return errCertificateNotRecognized
		}
		if device.PendingCertSerial != nil {
			if device.PendingCertExpiresAt == nil || len(device.PendingCertificatePem) == 0 {
				return errors.New("pending certificate is incomplete")
			}
			certificate = device.PendingCertificatePem
			return nil
		}
		issued, err := service.ca.IssueCertificateFromCSR(deviceID, request.Msg.GetCsr())
		if err != nil {
			return errInvalidCertificateRequest
		}
		serial, err := ca.SerialFromPEM(issued.CertPEM)
		if err != nil {
			return fmt.Errorf("read renewed certificate: %w", err)
		}
		rows, err := queries.SetPendingDeviceCertificate(ctx, db.SetPendingDeviceCertificateParams{
			PendingCertificatePem: issued.CertPEM, PendingCertSerial: &serial, PendingCertExpiresAt: &issued.NotAfter,
			ID: deviceID, ActiveCertSerial: peerSerial,
		})
		if err != nil {
			return fmt.Errorf("store renewed certificate: %w", err)
		}
		if rows != 1 {
			return errCertificateNotRecognized
		}
		if err := service.audit(ctx, queries, cadestrov1.AuditEventType_AUDIT_EVENT_TYPE_DEVICE_CERTIFICATE_RENEWED, cadestrov1.AuditStreamType_AUDIT_STREAM_TYPE_DEVICE, deviceID, cadestrov1.AuditActorType_AUDIT_ACTOR_TYPE_DEVICE, deviceID); err != nil {
			return err
		}
		certificate = issued.CertPEM
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, errCertificateNotRecognized):
			return nil, connect.NewError(connect.CodePermissionDenied, errCertificateNotRecognized)
		case errors.Is(err, errInvalidCertificateRequest):
			return nil, connect.NewError(connect.CodeInvalidArgument, errInvalidCertificateRequest)
		default:
			return nil, service.internal("renew certificate", err)
		}
	}
	return connect.NewResponse(&cadestrov1.RenewCertificateResponse{Certificate: certificate}), nil
}
