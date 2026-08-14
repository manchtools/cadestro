package executor

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/manchtools/cadestro/agent/internal/credentials"
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sdkcrypto "github.com/manchtools/cadestro/sdk/crypto"
	"github.com/manchtools/cadestro/sdk/sys/network"
)

// executeWifi splices the action ID into a filesystem path
// (network.CertBaseDir/<id> for EAP-TLS certificates) and into the
// pm-wifi-<id> NetworkManager connection name. Like the sudo/ssh/sshd
// executors it must run the action ID through validateActionIDForFilesystem
// BEFORE building any path, not merely reject the empty string.
//
// The "wrong" inputs are sourced from intent (path-meaningful characters and
// the 64-char ceiling), NOT from the validator's regex. Each must be refused
// before any NetworkManager call — the function returns at the validation
// check, before conName/certDir are computed, so no connection is created and
// no cert directory is written.
func TestExecuteWifi_RejectsUnsafeActionID(t *testing.T) {
	e := NewExecutor(nil)
	ctx := context.Background()
	// Non-nil params so the nil-params guard isn't what trips; the action-ID
	// gate must reject before params are ever read.
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

	// correct: a valid alphanumeric ULID passes the same gate executeWifi
	// consults, so legitimate WiFi actions are not broken by the new check.
	if err := validateActionIDForFilesystem("01ARZ3NDEKTSV4RRFFQ69G5FAV"); err != nil {
		t.Errorf("valid ULID action ID rejected by the gate: %v", err)
	}
}

// wifiCertPath writes EAP-TLS key material under the SDK's network.CertBaseDir
// while the rest of this agent's state lives under credentials.DefaultDataDir.
// Nothing in either module forces those two to agree, and for one rename they
// did not: the agent moved to /var/lib/cadestro while the SDK constant still
// pointed at the predecessor root. The agent then scattered private keys into a
// directory its own installer never creates, chmods, or uninstalls — outside
// the 0700 data dir, and outside the tree sys/remote will wipe.
//
// This asserts the invariant across the module boundary rather than restating
// either literal, so a future move of one constant fails here instead of
// silently splitting the agent's state in two again.
func TestCertBaseDirLivesUnderTheAgentDataDir(t *testing.T) {
	root := strings.TrimSuffix(credentials.DefaultDataDir, "/") + "/"
	if !strings.HasPrefix(network.CertBaseDir, root) {
		t.Fatalf("network.CertBaseDir = %q is not under credentials.DefaultDataDir = %q;"+
			" the agent would write EAP-TLS keys outside its own managed data directory",
			network.CertBaseDir, credentials.DefaultDataDir)
	}
	// The base dir must be a real subdirectory, not the data dir itself — the
	// prefix check above is satisfied by equality-plus-slash only if something
	// stripped the leaf, which would put cert dirs directly in the state root.
	if filepath.Clean(network.CertBaseDir) == filepath.Clean(credentials.DefaultDataDir) {
		t.Fatalf("network.CertBaseDir must be a subdirectory of %q, not the directory itself",
			credentials.DefaultDataDir)
	}
	// And the path executeWifi actually builds inherits that root, so the
	// invariant covers the call site and not just the constant.
	certDir := wifiCertPath("01ARZ3NDEKTSV4RRFFQ69G5FAV")
	if !strings.HasPrefix(certDir, root) {
		t.Fatalf("wifiCertPath = %q escapes the agent data directory %q", certDir, credentials.DefaultDataDir)
	}
}

func TestExecuteSealedWifi_RejectsWrongFieldContext(t *testing.T) {
	agentKey, err := sdkcrypto.GenerateX25519()
	if err != nil {
		t.Fatal(err)
	}
	controlKey, err := sdkcrypto.GenerateX25519()
	if err != nil {
		t.Fatal(err)
	}
	e := NewExecutor(nil)
	const deviceID = "01HXDEVICE0000000000000000"
	const actionID = "01HXWIFISEAL00000000000000"
	e.SetDeviceID(deviceID)
	if err := e.ConfigureSealing(agentKey.Bytes(), controlKey.PublicKey().Bytes()); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name      string
		authType  pb.WifiAuthType
		sealField string
		params    func(*pb.SealedValue) *pb.WifiParams
	}{
		{
			name: "PSK", authType: pb.WifiAuthType_WIFI_AUTH_TYPE_PSK, sealField: "client_key",
			params: func(value *pb.SealedValue) *pb.WifiParams { return &pb.WifiParams{Psk: value} },
		},
		{
			name: "EAP-TLS", authType: pb.WifiAuthType_WIFI_AUTH_TYPE_EAP_TLS, sealField: "psk",
			params: func(value *pb.SealedValue) *pb.WifiParams { return &pb.WifiParams{ClientKey: value} },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			aad, info, err := sdkcrypto.FieldSealContext(sdkcrypto.DirectionControlToAgent,
				"cadestro.v1.WifiParams", tc.sealField, deviceID, actionID)
			if err != nil {
				t.Fatal(err)
			}
			ciphertext, err := sdkcrypto.SealToPublicKey(agentKey.PublicKey(), []byte("credential"), aad, info)
			if err != nil {
				t.Fatal(err)
			}
			params := tc.params(&pb.SealedValue{Version: 1, Ciphertext: ciphertext})
			params.AuthType = tc.authType
			_, _, err = e.executeSealedWifi(context.Background(), params,
				pb.DesiredState_DESIRED_STATE_PRESENT, actionID)
			if err == nil || !strings.Contains(err.Error(), "open WiFi credential") {
				t.Fatalf("executeSealedWifi() error = %v, want sealed-field rejection", err)
			}
		})
	}
}

func TestExecuteSealedWifi_RejectsUnsafeActionIDBeforeOpeningCredential(t *testing.T) {
	e := NewExecutor(nil)
	_, _, err := e.executeSealedWifi(context.Background(), &pb.WifiParams{
		AuthType: pb.WifiAuthType_WIFI_AUTH_TYPE_PSK,
	}, pb.DesiredState_DESIRED_STATE_PRESENT, "../../etc")
	if err == nil || !strings.Contains(err.Error(), "action ID") {
		t.Fatalf("executeSealedWifi() error = %v, want action-ID rejection before sealed-field access", err)
	}
}
