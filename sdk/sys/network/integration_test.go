//go:build integration

package network_test

import (
	"context"
	"os"
	osexec "os/exec"
	"strings"
	"testing"
	"time"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/network"
)

const keyfileDir = "/etc/NetworkManager/system-connections"

func requireNM(t *testing.T) {
	t.Helper()
	bail := t.Skipf
	if os.Getenv("CADESTRO_NM_REQUIRED") == "1" {
		bail = t.Fatalf
	}
	if os.Geteuid() != 0 {
		bail("not root; keyfile write + nmcli reload need root")
		return
	}
	if _, err := osexec.LookPath("nmcli"); err != nil {
		bail("nmcli not present; NetworkManager backend not exercisable")
		return
	}
	if len(network.Detect(context.Background())) == 0 {
		bail("nmcli present but NetworkManager not usable here")
		return
	}
}

func TestApplyPSK_Integration(t *testing.T) {
	requireNM(t)
	r, err := sysexec.NewRunner(sysexec.Direct)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	m, err := network.New(network.NetworkManager, r)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const name = "cadestro-test-wifi"
	const ssid = "CadestroTestNet"
	const pskValue = "cadestro-test-passphrase-123"
	keyfile := keyfileDir + "/" + name + ".nmconnection"
	t.Cleanup(func() { _ = m.Delete(context.Background(), name, network.DeleteOptions{}) })

	psk, err := sysexec.NewSecret(pskValue)
	if err != nil {
		t.Fatalf("NewSecret: %v", err)
	}

	changed, err := m.Apply(ctx, network.Profile{Name: name, SSID: ssid, AuthType: network.AuthPSK, PSK: psk})
	if err != nil {
		t.Fatalf("Apply(PSK) against real NetworkManager: %v", err)
	}
	if !changed {
		t.Error("first Apply should report changed=true")
	}

	fi, err := os.Stat(keyfile)
	if err != nil {
		t.Fatalf("stat keyfile: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("keyfile mode = %o, want 0600 (the PSK file must not be world/group readable)", perm)
	}
	body := string(mustRead(t, keyfile))
	if !strings.Contains(body, "psk="+pskValue) {
		t.Error("keyfile does not carry the PSK in [wifi-security] (the secure provisioning sink)")
	}
	if !strings.Contains(body, "ssid="+ssid) {
		t.Errorf("keyfile missing ssid=%s", ssid)
	}

	exists, err := m.ConnectionExists(ctx, name)
	if err != nil {
		t.Fatalf("ConnectionExists: %v", err)
	}
	if !exists {
		t.Error("NetworkManager did not pick up the connection after `nmcli connection reload`")
	}
	settings, err := m.Settings(ctx, name)
	if err != nil {
		t.Fatalf("Settings: %v", err)
	}
	if len(settings) == 0 {
		t.Error("Settings returned no keys for the real connection")
	}

	if err := m.Delete(ctx, name, network.DeleteOptions{}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if exists, err := m.ConnectionExists(ctx, name); err != nil {
		t.Errorf("ConnectionExists after Delete: %v", err)
	} else if exists {
		t.Error("connection still present after Delete")
	}
	if _, err := os.Stat(keyfile); !os.IsNotExist(err) {
		t.Errorf("keyfile still present after Delete: %v", err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}
