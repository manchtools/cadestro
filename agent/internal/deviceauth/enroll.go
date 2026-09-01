package deviceauth

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/manchtools/cadestro/agent/internal/credentials"
	sdk "github.com/manchtools/cadestro/contract"
	sdkcrypto "github.com/manchtools/cadestro/sdk/crypto"
)

type EnrollmentRequest struct {
	ServerURL string
	Token     string
	CAPin     string
	Hostname  string
	Version   string
}

type EnrollmentResult struct {
	Credentials     *credentials.Credentials
	AlreadyEnrolled bool
}

type registerAgent func(context.Context, string, string, string, string, []byte, ...sdk.ClientOption) (*sdk.RegisterAgentResult, error)

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

func Enroll(ctx context.Context, request EnrollmentRequest, store *credentials.Store) (*EnrollmentResult, error) {
	return enroll(ctx, request, store, sdk.RegisterAgent)
}

func enroll(ctx context.Context, request EnrollmentRequest, store *credentials.Store, register registerAgent) (*EnrollmentResult, error) {
	if store == nil {
		return nil, fmt.Errorf("credential store is required")
	}
	var stored *credentials.Credentials
	stored, err := store.Load()
	if err == nil {
		if stored.Ready() {
			return &EnrollmentResult{Credentials: stored, AlreadyEnrolled: true}, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("load credentials: %w", err)
	}
	if request.ServerURL == "" || request.Token == "" {
		return nil, fmt.Errorf("server URL and token are required")
	}
	if request.Hostname == "" || request.Version == "" {
		return nil, fmt.Errorf("hostname and version are required")
	}
	pin, err := normalizePin(request.CAPin)
	if err != nil {
		return nil, err
	}
	if err := sdk.ValidateHTTPSURL(request.ServerURL); err != nil {
		return nil, fmt.Errorf("server URL must be an https URL: %w", err)
	}

	var csrPEM, keyPEM []byte
	if stored != nil {
		if len(stored.PendingCSR) > 0 && len(stored.PendingPrivateKey) > 0 {
			csrPEM = append([]byte(nil), stored.PendingCSR...)
			keyPEM = append([]byte(nil), stored.PendingPrivateKey...)
		} else {
			return nil, fmt.Errorf("stored credentials are incomplete")
		}
	}
	if len(csrPEM) == 0 {
		csrPEM, keyPEM, err = sdkcrypto.GenerateCSR(request.Hostname)
		if err != nil {
			return nil, fmt.Errorf("generate CSR: %w", err)
		}
		if err := store.Save(ctx, &credentials.Credentials{PendingPrivateKey: keyPEM, PendingCSR: csrPEM}); err != nil {
			return nil, fmt.Errorf("save pending credentials: %w", err)
		}
	}

	result, err := register(ctx, request.ServerURL, request.Token, request.Hostname, request.Version, csrPEM, sdk.WithCAPin(pin))
	if err != nil {
		return nil, fmt.Errorf("register agent: %w", err)
	}
	if len(result.CACert) == 0 || len(result.Certificate) == 0 {
		return nil, fmt.Errorf("server did not provide mTLS certificates")
	}
	got, err := sdkcrypto.CAFingerprintFromPEM(result.CACert)
	if err != nil {
		return nil, fmt.Errorf("fingerprint server CA: %w", err)
	}
	if !strings.EqualFold(got, pin) {
		return nil, fmt.Errorf("CA fingerprint mismatch: the server CA does not match the pinned fingerprint")
	}
	creds := &credentials.Credentials{DeviceID: result.DeviceID, CACert: result.CACert, Certificate: result.Certificate, PrivateKey: keyPEM, AgentAddr: result.ControlURL}
	if !creds.Ready() {
		return nil, errors.New("server returned incomplete credentials")
	}
	if err := store.Save(ctx, creds); err != nil {
		return nil, fmt.Errorf("save credentials: %w", err)
	}
	return &EnrollmentResult{Credentials: creds}, nil
}
