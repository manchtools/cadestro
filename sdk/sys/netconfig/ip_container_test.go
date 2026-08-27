//go:build container

package netconfig

import (
	"context"
	osexec "os/exec"
	"strings"
	"testing"
	"time"

	"github.com/manchtools/cadestro/sdk/sys/exec"
)

func TestGet_ParsesRealIpJSON_Container(t *testing.T) {
	if _, err := osexec.LookPath("ip"); err != nil {
		t.Skip("iproute2 `ip` not on PATH")
	}

	const testAddr = "192.0.2.5"

	if out, err := osexec.Command("ip", "addr", "add", testAddr+"/32", "dev", "lo").CombinedOutput(); err != nil {
		t.Skipf("cannot add test address (need CAP_NET_ADMIN?): %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = osexec.Command("ip", "addr", "del", testAddr+"/32", "dev", "lo").Run()
	})

	r, err := exec.NewRunner(exec.Direct)
	if err != nil {
		t.Fatalf("NewRunner(Direct): %v", err)
	}
	m, err := New(SystemdNetworkd, r)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg, err := m.Get(ctx, "lo")
	if err != nil {
		t.Fatalf("Get(lo): %v", err)
	}
	found := false
	for _, a := range cfg.Addresses {
		if strings.HasPrefix(a, testAddr) {
			found = true
		}
	}
	if !found {
		t.Errorf("Get(lo).Addresses = %v; want one starting %q (real `ip -j addr` parse drifted?)", cfg.Addresses, testAddr)
	}
}
