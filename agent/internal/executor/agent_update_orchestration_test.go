package executor

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func agentScript(version string, selfTestExit int) []byte {
	return agentScriptUnitExit(version, selfTestExit, 0)
}

func agentScriptUnitExit(version string, selfTestExit, installUnitExit int) []byte {
	return []byte(fmt.Sprintf(`#!/bin/sh
case "$1" in
  version) echo %q ;;
  self-test) exit %d ;;
  install-unit) shift; echo "$@" > "$0.install-unit"; exit %d ;;
  *) exit 0 ;;
esac
`, version, selfTestExit, installUnitExit))
}

func sha256hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

type updateHarness struct {
	e            *Executor
	binaryPath   string
	oldBytes     []byte
	srv          *httptest.Server
	shutdownCh   chan struct{}
	shutdownOnce sync.Once
}

func newUpdateHarness(t *testing.T, runningVersion string, serveBody, sumsBody []byte) *updateHarness {
	t.Helper()
	if sumsBody == nil {
		sumsBody = []byte(sha256hex(serveBody) + "  agent\n")
	}
	dir := t.TempDir()
	binaryPath := filepath.Join(dir, "cadestrod")
	oldBytes := []byte("#!/bin/sh\necho OLD\n")
	if err := os.WriteFile(binaryPath, oldBytes, 0o755); err != nil {
		t.Fatalf("seed binary: %v", err)
	}

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate release signer: %v", err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		t.Fatalf("marshal release signer: %v", err)
	}
	previousReleaseKey := releaseSigningPublicKey
	releaseSigningPublicKey = base64.StdEncoding.EncodeToString(publicDER)
	t.Cleanup(func() { releaseSigningPublicKey = previousReleaseKey })
	signature := ed25519.Sign(privateKey, sumsBody)

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/agent":
			w.Write(serveBody)
		case "/sums":
			w.Write(sumsBody)
		case "/sums.sig":
			if r.URL.Query().Has("tampered") {
				altered := append([]byte(nil), signature...)
				altered[0] ^= 1
				w.Write(altered)
				return
			}
			w.Write(signature)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	h := &updateHarness{binaryPath: binaryPath, oldBytes: oldBytes, srv: srv, shutdownCh: make(chan struct{})}
	e := &Executor{logger: slog.Default(), now: time.Now}
	e.httpClient = srv.Client()

	prevRemoteClient := remoteHTTPClient
	remoteHTTPClient = srv.Client()
	t.Cleanup(func() { remoteHTTPClient = prevRemoteClient })
	e.SetUpdateConfig(&AgentUpdateConfig{
		Version:    runningVersion,
		DataDir:    t.TempDir(),
		BinaryPath: binaryPath,
		Shutdown:   func() { h.shutdownOnce.Do(func() { close(h.shutdownCh) }) },
	})
	h.e = e
	return h
}

func (h *updateHarness) params() *pb.AgentUpdateParams {
	return &pb.AgentUpdateParams{
		Amd64: &pb.AgentUpdateArch{
			BinaryUrl:   h.srv.URL + "/agent",
			ChecksumUrl: h.srv.URL + "/sums",
		},
		Arm64: &pb.AgentUpdateArch{
			BinaryUrl:   h.srv.URL + "/agent",
			ChecksumUrl: h.srv.URL + "/sums",
		},
	}
}

func (h *updateHarness) shutdownCalled() bool {
	select {
	case <-h.shutdownCh:
		return true
	case <-time.After(4 * time.Second):

		return false
	}
}

func (h *updateHarness) currentBinary(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(h.binaryPath)
	if err != nil {
		t.Fatalf("read binary: %v", err)
	}
	return b
}

func TestExecuteAgentUpdate_ChecksumMismatchAbortsSwap(t *testing.T) {
	genuine := agentScript("v2026.06.05", 0)
	tampered := append([]byte{}, genuine...)
	tampered[len(tampered)-2] ^= 0xff

	sums := []byte(sha256hex(genuine) + "  agent\n")
	h := newUpdateHarness(t, "v2026.06.01", tampered, sums)
	_, changed, err := h.e.executeAgentUpdate(context.Background(), h.params())

	if err == nil {
		t.Fatal("checksum mismatch must abort the update")
	}
	if changed {
		t.Error("changed must be false on checksum mismatch")
	}
	if got := h.currentBinary(t); string(got) != string(h.oldBytes) {
		t.Error("live binary must be unchanged on checksum mismatch")
	}
	if _, statErr := os.Stat(h.binaryPath + ".bak"); statErr == nil {
		t.Error(".bak must not be created when the swap aborts")
	}
}

func TestExecuteAgentUpdate_SelfTestFailKeepsBinary(t *testing.T) {
	staged := agentScript("v2026.06.05", 1)
	h := newUpdateHarness(t, "v2026.06.01", staged, nil)

	_, changed, err := h.e.executeAgentUpdate(context.Background(), h.params())
	if err == nil {
		t.Fatal("self-test failure must abort the update")
	}
	if changed {
		t.Error("changed must be false on self-test failure")
	}
	if got := h.currentBinary(t); string(got) != string(h.oldBytes) {
		t.Error("live binary must be unchanged on self-test failure")
	}
}

func TestExecuteAgentUpdate_HappyPathSwapsAndShutsDown(t *testing.T) {
	staged := agentScript("v2026.06.05", 0)
	h := newUpdateHarness(t, "v2026.06.01", staged, nil)

	_, changed, err := h.e.executeAgentUpdate(context.Background(), h.params())
	if err != nil {
		t.Fatalf("happy-path update failed: %v", err)
	}
	if !changed {
		t.Error("changed must be true on a successful update")
	}
	if got := h.currentBinary(t); string(got) != string(staged) {
		t.Error("live binary must be the staged bytes after swap")
	}
	bak, err := os.ReadFile(h.binaryPath + ".bak")
	if err != nil {
		t.Fatalf("read .bak: %v", err)
	}
	if string(bak) != string(h.oldBytes) {
		t.Error(".bak must hold the previous binary")
	}
	if !h.shutdownCalled() {
		t.Error("Shutdown must be invoked after a successful update")
	}
}

func TestExecuteAgentUpdate_RefreshesUnitFromNewBinary(t *testing.T) {
	staged := agentScript("v2026.06.05", 0)
	h := newUpdateHarness(t, "v2026.06.01", staged, nil)

	_, changed, err := h.e.executeAgentUpdate(context.Background(), h.params())
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if !changed {
		t.Error("changed must be true")
	}
	argv, err := os.ReadFile(h.binaryPath + ".install-unit")
	if err != nil {
		t.Fatalf("install-unit was not invoked on the new binary: %v", err)
	}
	if !strings.Contains(string(argv), "--data-dir="+h.e.updateCfg.DataDir) {
		t.Errorf("install-unit argv = %q, want --data-dir=%s", string(argv), h.e.updateCfg.DataDir)
	}
	if !h.shutdownCalled() {
		t.Error("Shutdown must still be invoked after the unit refresh")
	}
}

func TestExecuteAgentUpdate_UnitInstallFailureIsFailOpen(t *testing.T) {
	staged := agentScriptUnitExit("v2026.06.05", 0, 1)
	h := newUpdateHarness(t, "v2026.06.01", staged, nil)

	_, changed, err := h.e.executeAgentUpdate(context.Background(), h.params())
	if err != nil {
		t.Fatalf("update must succeed despite install-unit failure: %v", err)
	}
	if !changed {
		t.Error("changed must be true")
	}
	if !h.shutdownCalled() {
		t.Error("Shutdown must be invoked despite install-unit failure")
	}
}

func TestExecuteAgentUpdate_RefusesNoIntegritySource(t *testing.T) {
	staged := agentScript("v2026.06.05", 0)
	h := newUpdateHarness(t, "v2026.06.01", staged, nil)

	noIntegrity := &pb.AgentUpdateParams{
		Amd64: &pb.AgentUpdateArch{BinaryUrl: h.srv.URL + "/agent"},
		Arm64: &pb.AgentUpdateArch{BinaryUrl: h.srv.URL + "/agent"},
	}
	_, changed, err := h.e.executeAgentUpdate(context.Background(), noIntegrity)
	if err == nil {
		t.Fatal("an action with no integrity source must be refused")
	}
	if changed {
		t.Error("changed must be false")
	}
	if got := h.currentBinary(t); string(got) != string(h.oldBytes) {
		t.Error("live binary must be unchanged")
	}
}

func TestExecuteAgentUpdate_SignedManifest(t *testing.T) {
	staged := agentScript("v2026.06.05", 0)

	sums := []byte(sha256hex(staged) + "  agent\n")
	h := newUpdateHarness(t, "v2026.06.01", staged, sums)

	p := &pb.AgentUpdateParams{
		Amd64: &pb.AgentUpdateArch{BinaryUrl: h.srv.URL + "/agent", ChecksumUrl: h.srv.URL + "/sums"},
		Arm64: &pb.AgentUpdateArch{BinaryUrl: h.srv.URL + "/agent", ChecksumUrl: h.srv.URL + "/sums"},
	}
	_, changed, err := h.e.executeAgentUpdate(context.Background(), p)
	if err != nil {
		t.Fatalf("signed-manifest update failed: %v", err)
	}
	if !changed {
		t.Error("changed must be true on a successful checksum_url-verified update")
	}
	if got := h.currentBinary(t); string(got) != string(staged) {
		t.Error("binary must be swapped to the staged bytes")
	}
	bak, err := os.ReadFile(h.binaryPath + ".bak")
	if err != nil {
		t.Fatalf("read .bak: %v", err)
	}
	if string(bak) != string(h.oldBytes) {
		t.Error(".bak must hold the previous binary")
	}
	if !h.shutdownCalled() {
		t.Error("Shutdown must be invoked after a successful update")
	}
}

func TestExecuteAgentUpdate_ChecksumURLMismatchRejected(t *testing.T) {
	staged := agentScript("v2026.06.05", 0)
	tampered := append([]byte{}, staged...)
	tampered[len(tampered)-2] ^= 0xff

	sums := []byte(sha256hex(staged) + "  agent\n")
	h := newUpdateHarness(t, "v2026.06.01", tampered, sums)

	p := &pb.AgentUpdateParams{
		Amd64: &pb.AgentUpdateArch{BinaryUrl: h.srv.URL + "/agent", ChecksumUrl: h.srv.URL + "/sums"},
		Arm64: &pb.AgentUpdateArch{BinaryUrl: h.srv.URL + "/agent", ChecksumUrl: h.srv.URL + "/sums"},
	}
	_, changed, err := h.e.executeAgentUpdate(context.Background(), p)
	if err == nil {
		t.Fatal("a checksum_url mismatch must abort the update")
	}
	if changed {
		t.Error("changed must be false")
	}
	if got := h.currentBinary(t); string(got) != string(h.oldBytes) {
		t.Error("live binary must be unchanged")
	}
}

func TestExecuteAgentUpdate_ChecksumURLSignatureRejected(t *testing.T) {
	staged := agentScript("v2026.06.05", 0)
	sums := []byte(sha256hex(staged) + "  agent\n")
	h := newUpdateHarness(t, "v2026.06.01", staged, sums)
	p := &pb.AgentUpdateParams{
		Amd64: &pb.AgentUpdateArch{
			BinaryUrl: h.srv.URL + "/agent", ChecksumUrl: h.srv.URL + "/sums?tampered=true",
		},
		Arm64: &pb.AgentUpdateArch{
			BinaryUrl: h.srv.URL + "/agent", ChecksumUrl: h.srv.URL + "/sums?tampered=true",
		},
	}

	_, changed, err := h.e.executeAgentUpdate(context.Background(), p)
	if err == nil {
		t.Fatal("a checksum manifest with an invalid publisher signature must be rejected")
	}
	if changed {
		t.Fatal("invalid release signature changed the installed binary")
	}
	if got := h.currentBinary(t); string(got) != string(h.oldBytes) {
		t.Fatal("invalid release signature replaced the installed binary")
	}
}

func TestExecuteAgentUpdate_HTTPSourceRejected(t *testing.T) {
	staged := agentScript("v2026.06.05", 0)
	h := newUpdateHarness(t, "v2026.06.01", staged, nil)
	p := h.params()
	p.Amd64.BinaryUrl = "http://example.com/agent"
	p.Arm64.BinaryUrl = "http://example.com/agent"

	_, changed, err := h.e.executeAgentUpdate(context.Background(), p)
	if err == nil {
		t.Fatal("http binary_url must be rejected")
	}
	if changed {
		t.Error("changed must be false")
	}
	if got := h.currentBinary(t); string(got) != string(h.oldBytes) {
		t.Error("live binary must be unchanged")
	}
}
