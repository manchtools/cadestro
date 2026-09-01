package deviceauth

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/manchtools/cadestro/agent/internal/credentials"
	sdk "github.com/manchtools/cadestro/contract"
	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/manchtools/cadestro/sdk/crypto"
	"github.com/manchtools/cadestro/sdk/cryptotest"
)

func testStore(t *testing.T) *credentials.Store {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return credentials.NewStore(dir)
}

func TestEnrollRejectsInvalidRequestFields(t *testing.T) {
	base := EnrollmentRequest{ServerURL: "https://control.example.test", Token: "token", CAPin: strings.Repeat("a", 64), Hostname: "host", Version: "dev"}
	for name, mutate := range map[string]func(*EnrollmentRequest){
		"missing server":   func(r *EnrollmentRequest) { r.ServerURL = "" },
		"non-https server": func(r *EnrollmentRequest) { r.ServerURL = "http://control.example.test" },
		"missing token":    func(r *EnrollmentRequest) { r.Token = "" },
		"missing pin":      func(r *EnrollmentRequest) { r.CAPin = "" },
		"non-hex pin":      func(r *EnrollmentRequest) { r.CAPin = strings.Repeat("g", 64) },
		"missing hostname": func(r *EnrollmentRequest) { r.Hostname = "" },
		"missing version":  func(r *EnrollmentRequest) { r.Version = "" },
	} {
		t.Run(name, func(t *testing.T) {
			input := base
			mutate(&input)
			store := testStore(t)
			calls := 0
			register := func(context.Context, string, string, string, string, []byte, ...sdk.ClientOption) (*sdk.RegisterAgentResult, error) {
				calls++
				return nil, errors.New("register must not be called")
			}
			if _, err := enroll(context.Background(), input, store, register); err == nil {
				t.Fatal("expected enrollment validation error")
			}
			if calls != 0 {
				t.Fatal("invalid enrollment input reached registration")
			}
			if _, err := store.Load(); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("invalid input wrote credentials: %v", err)
			}
		})
	}
}

func TestEnrollReturnsExistingCredentials(t *testing.T) {
	store := testStore(t)
	want := &credentials.Credentials{DeviceID: "device", CACert: []byte("ca"), Certificate: []byte("certificate"), PrivateKey: []byte("key"), AgentAddr: "https://agent.example.test"}
	if err := store.Save(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	got, err := Enroll(context.Background(), EnrollmentRequest{}, store)
	if err != nil {
		t.Fatal(err)
	}
	if !got.AlreadyEnrolled || got.Credentials.DeviceID != want.DeviceID {
		t.Fatalf("existing enrollment result = %+v", got)
	}
}

func TestEnrollRejectsNilAndIncompleteStore(t *testing.T) {
	register := func(context.Context, string, string, string, string, []byte, ...sdk.ClientOption) (*sdk.RegisterAgentResult, error) {
		t.Fatal("register called")
		return nil, nil
	}
	request := EnrollmentRequest{ServerURL: "https://control.example.test", Token: "token", CAPin: strings.Repeat("a", 64), Hostname: "host", Version: "dev"}
	if _, err := enroll(context.Background(), request, nil, register); err == nil {
		t.Fatal("nil store was accepted")
	}
	store := testStore(t)
	if err := store.Save(context.Background(), &credentials.Credentials{DeviceID: "incomplete"}); err != nil {
		t.Fatal(err)
	}
	if _, err := enroll(context.Background(), request, store, register); err == nil {
		t.Fatal("incomplete stored credentials were accepted")
	}
}

func TestDirectEnrollSuccessAndSecurityFailures(t *testing.T) {
	ca := cryptotest.CAPEM(t, "direct-enroll")
	pin, err := crypto.CAFingerprintFromPEM(ca)
	if err != nil {
		t.Fatal(err)
	}
	var colonized strings.Builder
	for i := 0; i < len(pin); i += 2 {
		if i > 0 {
			colonized.WriteByte(':')
		}
		colonized.WriteString(strings.ToUpper(pin[i : i+2]))
	}
	request := EnrollmentRequest{ServerURL: "https://control.example.test", Token: "token", CAPin: colonized.String(), Hostname: "host", Version: "v1"}
	var calls int
	register := func(_ context.Context, serverURL, token, hostname, version string, csr []byte, options ...sdk.ClientOption) (*sdk.RegisterAgentResult, error) {
		calls++
		if serverURL != request.ServerURL || token != request.Token || hostname != request.Hostname || version != request.Version {
			t.Fatalf("registration inputs were not bound")
		}
		block, _ := pem.Decode(csr)
		parsed, err := x509.ParseCertificateRequest(block.Bytes)
		if err != nil || parsed.CheckSignature() != nil {
			t.Fatalf("registration CSR is not signed: %v", err)
		}
		return &sdk.RegisterAgentResult{DeviceID: "device", CACert: ca, Certificate: []byte("certificate"), ControlURL: "https://agent.example.test"}, nil
	}
	store := testStore(t)
	result, err := enroll(context.Background(), request, store, register)
	if err != nil {
		t.Fatal(err)
	}
	if result.AlreadyEnrolled || result.Credentials.DeviceID != "device" || calls != 1 {
		t.Fatalf("fresh enrollment result = %+v calls=%d", result, calls)
	}
	loaded, err := store.Load()
	if err != nil || len(loaded.PendingCSR) != 0 || loaded.DeviceID != "device" {
		t.Fatalf("final credentials = %+v err=%v", loaded, err)
	}

}

type registrationHandler struct {
	cadestrov1connect.UnimplementedControlServiceHandler
	ca []byte
}

func (h registrationHandler) Register(context.Context, *connect.Request[cadestrov1.RegisterRequest]) (*connect.Response[cadestrov1.RegisterResponse], error) {
	return connect.NewResponse(&cadestrov1.RegisterResponse{DeviceId: &cadestrov1.DeviceId{Value: "device"}, CaCert: h.ca, Certificate: []byte("certificate"), ControlUrl: "https://agent.example.test"}), nil
}

func TestEnrollUsesWebPKIForRegistrationAndPinsReturnedCA(t *testing.T) {
	_, tlsCAKey, tlsCACert := cryptotest.GenCA(t, "TLS CA")
	tlsLeafPEM, tlsLeafKeyPEM := cryptotest.GenLeaf(t, tlsCACert, tlsCAKey, "localhost", true)
	certificate, err := tls.X509KeyPair(tlsLeafPEM, tlsLeafKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	certificate.Certificate = append(certificate.Certificate, tlsCACert.Raw)
	returnedCA := cryptotest.CAPEM(t, "returned CA")
	path, handler := cadestrov1connect.NewControlServiceHandler(registrationHandler{ca: returnedCA})
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	server := httptest.NewUnstartedServer(mux)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS13}
	server.StartTLS()
	defer server.Close()
	serverURL := strings.Replace(server.URL, "127.0.0.1", "localhost", 1)
	request := EnrollmentRequest{ServerURL: serverURL, Token: "token", CAPin: mustFingerprint(t, returnedCA), Hostname: "host", Version: "v1"}
	register := func(ctx context.Context, controlURL, token, hostname, version string, csr []byte, _ ...sdk.ClientOption) (*sdk.RegisterAgentResult, error) {
		return sdk.RegisterAgent(ctx, controlURL, token, hostname, version, csr, sdk.WithHTTPClient(server.Client()))
	}
	result, err := enroll(context.Background(), request, testStore(t), register)
	if err != nil || result.Credentials.DeviceID != "device" {
		t.Fatalf("enrollment result = %+v, error = %v", result, err)
	}
}

func mustFingerprint(t *testing.T, certPEM []byte) string {
	t.Helper()
	fingerprint, err := crypto.CAFingerprintFromPEM(certPEM)
	if err != nil {
		t.Fatal(err)
	}
	return fingerprint
}

func TestDirectEnrollPinMismatchAndMissingCertificatesKeepPending(t *testing.T) {
	ca := cryptotest.CAPEM(t, "direct-enroll-mismatch")
	otherCA := cryptotest.CAPEM(t, "direct-enroll-other")
	pin, err := crypto.CAFingerprintFromPEM(ca)
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range []*sdk.RegisterAgentResult{
		{DeviceID: "", CACert: ca, Certificate: []byte("certificate"), ControlURL: "https://agent.example.test"},
		{DeviceID: "device", CACert: otherCA, Certificate: []byte("certificate")},
		{DeviceID: "device", CACert: nil, Certificate: []byte("certificate")},
		{DeviceID: "device", CACert: ca, Certificate: nil},
		{DeviceID: "device", CACert: ca, Certificate: []byte("certificate"), ControlURL: ""},
	} {
		store := testStore(t)
		_, err := enroll(context.Background(), EnrollmentRequest{ServerURL: "https://control.example.test", Token: "token", CAPin: pin, Hostname: "host", Version: "v1"}, store, func(context.Context, string, string, string, string, []byte, ...sdk.ClientOption) (*sdk.RegisterAgentResult, error) {
			return result, nil
		})
		if err == nil {
			t.Fatal("invalid registration result was accepted")
		}
		pending, loadErr := store.Load()
		if loadErr != nil || len(pending.PendingCSR) == 0 || len(pending.PendingPrivateKey) == 0 {
			t.Fatalf("pending identity was not retained: %+v %v", pending, loadErr)
		}
	}
}

func TestDirectEnrollRetryReusesPendingCSR(t *testing.T) {
	ca := cryptotest.CAPEM(t, "direct-enroll-retry")
	pin, err := crypto.CAFingerprintFromPEM(ca)
	if err != nil {
		t.Fatal(err)
	}
	request := EnrollmentRequest{ServerURL: "https://control.example.test", Token: "token", CAPin: pin, Hostname: "host", Version: "v1"}
	store := testStore(t)
	var csrs [][]byte
	calls := 0
	register := func(_ context.Context, _ string, _ string, _ string, _ string, csr []byte, _ ...sdk.ClientOption) (*sdk.RegisterAgentResult, error) {
		calls++
		csrs = append(csrs, append([]byte(nil), csr...))
		if calls == 1 {
			return nil, errors.New("response lost")
		}
		return &sdk.RegisterAgentResult{DeviceID: "device", CACert: ca, Certificate: []byte("certificate"), ControlURL: "https://agent.example.test"}, nil
	}
	if _, err := enroll(context.Background(), request, store, register); err == nil {
		t.Fatal("first registration error was not returned")
	}
	result, err := enroll(context.Background(), request, store, register)
	if err != nil || result.AlreadyEnrolled || !result.Credentials.Ready() || len(csrs) != 2 || !bytes.Equal(csrs[0], csrs[1]) {
		t.Fatalf("retry result=%+v err=%v calls=%d identical=%v", result, err, len(csrs), len(csrs) == 2 && bytes.Equal(csrs[0], csrs[1]))
	}
}
