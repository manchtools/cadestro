package deviceauth

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"

	"github.com/manchtools/cadestro/agent/internal/credentials"
	sdk "github.com/manchtools/cadestro/contract"
	sdkcrypto "github.com/manchtools/cadestro/sdk/crypto"
)

type credentialStore interface {
	Exists() bool
	Load() (*credentials.Credentials, error)
	Save(context.Context, *credentials.Credentials) error
}

type EnrollHandler struct {
	cadestrov1connect.UnimplementedDeviceAuthServiceHandler

	hostname   string
	version    string
	credStore  credentialStore
	logger     *slog.Logger
	onEnrolled func(creds *credentials.Credentials)

	rateMu       sync.Mutex
	lastAttempts []time.Time

	enrollMu sync.Mutex

	statusMu       sync.Mutex
	cachedDeviceID string
	statusCached   bool

	now func() time.Time

	registerOpts []sdk.ClientOption
}

func NewEnrollHandler(hostname, version string, credStore *credentials.Store, logger *slog.Logger, onEnrolled func(*credentials.Credentials)) *EnrollHandler {
	return &EnrollHandler{
		hostname:   hostname,
		version:    version,
		credStore:  credStore,
		logger:     logger,
		onEnrolled: onEnrolled,
		now:        time.Now,
	}
}

func normalizePin(pin string) (string, error) {
	normalized := strings.ReplaceAll(strings.TrimSpace(pin), ":", "")
	if len(normalized) != 64 {
		return "", fmt.Errorf("CA fingerprint pin is required and must contain 64 hexadecimal characters")
	}
	if _, err := hex.DecodeString(normalized); err != nil {
		return "", fmt.Errorf("CA fingerprint pin must contain 64 hexadecimal characters")
	}
	return strings.ToLower(normalized), nil
}

func (h *EnrollHandler) Enroll(ctx context.Context, req *connect.Request[cadestrov1.EnrollRequest]) (*connect.Response[cadestrov1.EnrollResponse], error) {

	h.rateMu.Lock()
	now := h.now()
	cutoff := now.Add(-1 * time.Minute)
	var recent []time.Time
	for _, t := range h.lastAttempts {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	recent = append(recent, now)
	h.lastAttempts = recent
	count := len(recent)
	h.rateMu.Unlock()

	if count > 5 {
		h.logger.Warn("enrollment rate limit exceeded")
		return connect.NewResponse(&cadestrov1.EnrollResponse{
			Success: false,
			Error:   "rate limit exceeded, try again later",
		}), nil
	}

	h.enrollMu.Lock()
	defer h.enrollMu.Unlock()

	h.logger.Info("enrollment request received", "server_url", req.Msg.ServerUrl)

	if req.Msg.ServerUrl == "" || req.Msg.Token == "" {
		return connect.NewResponse(&cadestrov1.EnrollResponse{
			Success: false,
			Error:   "server_url and token are required",
		}), nil
	}
	pin, err := normalizePin(req.Msg.CaFingerprintPin)
	if err != nil {
		return connect.NewResponse(&cadestrov1.EnrollResponse{Success: false, Error: err.Error()}), nil
	}

	if err := sdk.ValidateHTTPSURL(req.Msg.ServerUrl); err != nil {
		return connect.NewResponse(&cadestrov1.EnrollResponse{
			Success: false,
			Error:   fmt.Sprintf("server_url must be an https URL: %v", err),
		}), nil
	}

	var csrPEM, keyPEM []byte
	if h.credStore.Exists() {
		creds, err := h.credStore.Load()
		if err != nil {
			return connect.NewResponse(&cadestrov1.EnrollResponse{
				Success: false,
				Error:   fmt.Sprintf("failed to load credentials: %v", err),
			}), nil
		}
		if len(creds.PendingCSR) > 0 && len(creds.PendingPrivateKey) > 0 {
			csrPEM = append([]byte(nil), creds.PendingCSR...)
			keyPEM = append([]byte(nil), creds.PendingPrivateKey...)
		} else {
			return connect.NewResponse(&cadestrov1.EnrollResponse{
				Success:  true,
				DeviceId: &cadestrov1.DeviceId{Value: creds.DeviceID},
				Error:    "agent is already enrolled",
			}), nil
		}
	}

	if len(csrPEM) == 0 {

		h.logger.Debug("generating key pair and CSR")
		csrPEM, keyPEM, err = sdkcrypto.GenerateCSR(h.hostname)
		if err != nil {
			h.logger.Error("failed to generate CSR", "error", err)
			return connect.NewResponse(&cadestrov1.EnrollResponse{
				Success: false,
				Error:   fmt.Sprintf("failed to generate CSR: %v", err),
			}), nil
		}

		if err := h.credStore.Save(ctx, &credentials.Credentials{
			PendingPrivateKey: keyPEM,
			PendingCSR:        csrPEM,
		}); err != nil {
			h.logger.Error("failed to save pending enrollment identity", "error", err)
			return connect.NewResponse(&cadestrov1.EnrollResponse{
				Success: false,
				Error:   fmt.Sprintf("failed to save credentials: %v", err),
			}), nil
		}
	}

	registerOpts := h.registerOpts
	if len(registerOpts) == 0 {
		registerOpts = []sdk.ClientOption{sdk.WithCAPin(pin)}
	}
	result, err := sdk.RegisterAgent(ctx, req.Msg.ServerUrl, req.Msg.Token, h.hostname, h.version, csrPEM, registerOpts...)
	if err != nil {
		h.logger.Error("registration failed", "error", err)
		return connect.NewResponse(&cadestrov1.EnrollResponse{
			Success: false,
			Error:   fmt.Sprintf("registration failed: %v", err),
		}), nil
	}

	if len(result.CACert) == 0 || len(result.Certificate) == 0 {
		return connect.NewResponse(&cadestrov1.EnrollResponse{
			Success: false,
			Error:   "server did not provide mTLS certificates",
		}), nil
	}

	got, fpErr := sdkcrypto.CAFingerprintFromPEM(result.CACert)
	if fpErr != nil {
		return connect.NewResponse(&cadestrov1.EnrollResponse{
			Success: false,
			Error:   fmt.Sprintf("cannot fingerprint server CA: %v", fpErr),
		}), nil
	}
	if !strings.EqualFold(got, pin) {
		h.logger.Error("enrollment CA fingerprint mismatch — refusing to trust server CA",
			"expected_pin", pin, "server_ca_fingerprint", got)
		return connect.NewResponse(&cadestrov1.EnrollResponse{
			Success: false,
			Error:   "CA fingerprint mismatch: the server CA does not match the pinned fingerprint",
		}), nil
	}

	creds := &credentials.Credentials{
		DeviceID:    result.DeviceID,
		CACert:      result.CACert,
		Certificate: result.Certificate,
		PrivateKey:  keyPEM,
		AgentAddr:   result.ControlURL,
	}

	if err := h.credStore.Save(ctx, creds); err != nil {
		h.logger.Error("failed to save credentials", "error", err)
		return connect.NewResponse(&cadestrov1.EnrollResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to save credentials: %v", err),
		}), nil
	}

	h.logger.Info("enrollment successful", "device_id", result.DeviceID, "control", result.ControlURL)

	h.statusMu.Lock()
	h.cachedDeviceID = result.DeviceID
	h.statusCached = true
	h.statusMu.Unlock()

	if h.onEnrolled != nil {
		h.onEnrolled(creds)
	}

	return connect.NewResponse(&cadestrov1.EnrollResponse{
		Success:  true,
		DeviceId: &cadestrov1.DeviceId{Value: result.DeviceID},
	}), nil
}

func (h *EnrollHandler) GetEnrollmentStatus(_ context.Context, _ *connect.Request[cadestrov1.GetEnrollmentStatusRequest]) (*connect.Response[cadestrov1.GetEnrollmentStatusResponse], error) {
	h.statusMu.Lock()
	defer h.statusMu.Unlock()

	if h.statusCached {
		return connect.NewResponse(&cadestrov1.GetEnrollmentStatusResponse{
			Enrolled: true,
			DeviceId: &cadestrov1.DeviceId{Value: h.cachedDeviceID},
		}), nil
	}

	if !h.credStore.Exists() {
		return connect.NewResponse(&cadestrov1.GetEnrollmentStatusResponse{
			Enrolled: false,
		}), nil
	}

	creds, err := h.credStore.Load()
	if err != nil || creds.DeviceID == "" {

		return connect.NewResponse(&cadestrov1.GetEnrollmentStatusResponse{
			Enrolled: false,
		}), nil
	}

	h.cachedDeviceID = creds.DeviceID
	h.statusCached = true
	return connect.NewResponse(&cadestrov1.GetEnrollmentStatusResponse{
		Enrolled: true,
		DeviceId: &cadestrov1.DeviceId{Value: creds.DeviceID},
	}), nil
}
