package executor

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/manchtools/cadestro/agent/internal/credentials"
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/sdk/sys/network"
)

func TestExecuteWifi_RejectsUnsafeActionID(t *testing.T) {
	e := NewExecutor(nil)
	ctx := context.Background()

	params := &pb.WifiParams{Ssid: "corp-net"}

	unsafe := []struct {
		name string
		id   string
	}{
		{"parent traversal", "../../etc"},
		{"embedded slash", "a/b"},
		{"shell separator", "a;b"},
		{"over 64 chars", strings.Repeat("a", 65)},
		{"empty", ""},
	}
	for _, tc := range unsafe {
		t.Run(tc.name, func(t *testing.T) {
			out, changed, err := e.executeWifi(ctx, params, pb.DesiredState_DESIRED_STATE_PRESENT, tc.id, "", "")
			if err == nil {
				t.Fatalf("executeWifi(id=%q) = nil error, want rejection", tc.id)
			}
			if !strings.Contains(err.Error(), "action ID") {
				t.Errorf("error = %q, want a validateActionIDForFilesystem message naming the action ID", err)
			}
			if changed {
				t.Error("changed must be false on a rejected action ID")
			}
			if out != nil {
				t.Errorf("output must be nil on rejection, got %v", out)
			}
		})
	}

	if err := validateActionIDForFilesystem("01ARZ3NDEKTSV4RRFFQ69G5FAV"); err != nil {
		t.Errorf("valid ULID action ID rejected by the gate: %v", err)
	}
}

func TestCertBaseDirLivesUnderTheAgentDataDir(t *testing.T) {
	root := strings.TrimSuffix(credentials.DefaultDataDir, "/") + "/"
	if !strings.HasPrefix(network.CertBaseDir, root) {
		t.Fatalf("network.CertBaseDir = %q is not under credentials.DefaultDataDir = %q;"+
			" the agent would write EAP-TLS keys outside its own managed data directory",
			network.CertBaseDir, credentials.DefaultDataDir)
	}

	if filepath.Clean(network.CertBaseDir) == filepath.Clean(credentials.DefaultDataDir) {
		t.Fatalf("network.CertBaseDir must be a subdirectory of %q, not the directory itself",
			credentials.DefaultDataDir)
	}

	certDir := wifiCertPath("01ARZ3NDEKTSV4RRFFQ69G5FAV")
	if !strings.HasPrefix(certDir, root) {
		t.Fatalf("wifiCertPath = %q escapes the agent data directory %q", certDir, credentials.DefaultDataDir)
	}
}

func TestExecuteWifiActionRejectsUnsafeActionIDBeforeOpeningCredential(t *testing.T) {
	e := NewExecutor(nil)
	_, _, err := e.executeWifiAction(context.Background(), &pb.WifiParams{
		AuthType: pb.WifiAuthType_WIFI_AUTH_TYPE_PSK,
	}, pb.DesiredState_DESIRED_STATE_PRESENT, "../../etc")
	if err == nil || !strings.Contains(err.Error(), "action ID") {
		t.Fatalf("executeWifiAction() error = %v, want action-ID rejection before credential access", err)
	}
}
