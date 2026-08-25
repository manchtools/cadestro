//go:build integration

package dns_test

import (
	"context"
	"os"
	osexec "os/exec"
	"slices"
	"testing"

	"github.com/manchtools/cadestro/sdk/sys/dns"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

func systemdRunning() bool {
	_, err := os.Stat("/run/systemd/system")
	return err == nil
}

func resolvedActive() bool {
	if _, err := osexec.LookPath("resolvectl"); err != nil {
		return false
	}
	_, err := os.Stat("/run/systemd/resolve/resolv.conf")
	return err == nil
}

func TestDetect_Integration(t *testing.T) {
	backends := dns.Detect(context.Background())
	for _, b := range backends {
		if b != dns.Resolved && b != dns.NetworkManager {
			t.Errorf("Detect returned an unexpected backend %v", b)
		}
	}
	if _, err := osexec.LookPath("resolvectl"); err == nil {
		if !slices.Contains(backends, dns.Resolved) {
			t.Errorf("resolvectl on PATH but Detect did not report Resolved: %v", backends)
		}
	}
}

func TestResolvedGet_Integration(t *testing.T) {
	if !resolvedActive() {
		t.Skip("systemd-resolved not active here; Resolved Get not exercisable")
	}
	m := newResolved(t)
	if _, err := m.Get(context.Background()); err != nil {
		t.Fatalf("Get against real systemd-resolved resolv.conf: %v", err)
	}
}

func TestResolvedApply_Global_Integration(t *testing.T) {
	if !systemdRunning() || !resolvedActive() {
		t.Skip("requires a live systemd + systemd-resolved (test-sys container)")
	}
	m := newResolved(t)
	ctx := context.Background()

	const wantV4 = "192.0.2.53"
	const wantV6 = "2001:db8::53"
	const wantDomain = "corp.example"

	t.Cleanup(func() { restoreResolved(t) })

	if err := m.Apply(ctx, dns.Config{
		Nameservers:   []string{wantV4, wantV6},
		SearchDomains: []string{wantDomain},
	}); err != nil {
		t.Fatalf("Apply(global) against real systemd-resolved: %v", err)
	}

	st, err := m.Get(ctx)
	if err != nil {
		t.Fatalf("Get after Apply: %v", err)
	}
	if !slices.Contains(st.Nameservers, wantV4) {
		t.Errorf("applied nameserver %q not reflected in resolv.conf; got %v", wantV4, st.Nameservers)
	}
	if !slices.Contains(st.Nameservers, wantV6) {
		t.Errorf("applied nameserver %q not reflected in resolv.conf; got %v", wantV6, st.Nameservers)
	}
	if !slices.Contains(st.SearchDomains, wantDomain) {
		t.Errorf("applied search domain %q not reflected in resolv.conf; got %v", wantDomain, st.SearchDomains)
	}
}

func TestResolvedApply_PerLink_Integration(t *testing.T) {
	if !systemdRunning() || !resolvedActive() {
		t.Skip("requires a live systemd + systemd-resolved (test-sys container)")
	}
	iface := firstRealLink(t)
	if iface == "" {
		t.Skip("no non-loopback link to scope per-link DNS to")
	}
	m := newResolved(t)

	t.Cleanup(func() { revertLink(t, iface) })
	err := m.Apply(context.Background(), dns.Config{
		Nameservers: []string{"192.0.2.53"},
		Interface:   iface,
	})
	if err != nil {

		t.Skipf("per-link resolvectl dns %s not applicable here: %v", iface, err)
	}
	t.Logf("per-link Apply on %s accepted by real resolvectl", iface)
}

func newResolved(t *testing.T) dns.Manager {
	t.Helper()

	r, err := sysexec.NewRunner(sysexec.Sudo)
	if err != nil {
		t.Fatalf("NewRunner(Sudo): %v", err)
	}
	m, err := dns.New(dns.Resolved, r)
	if err != nil {
		t.Fatalf("New(Resolved): %v", err)
	}
	return m
}

func restoreResolved(t *testing.T) {
	t.Helper()
	r, err := sysexec.NewRunner(sysexec.Sudo)
	if err != nil {
		t.Logf("restore: NewRunner: %v", err)
		return
	}
	ctx := context.Background()
	if _, err := r.Run(ctx, sysexec.Command{
		Name: "rm", Args: []string{"-f", "/etc/systemd/resolved.conf.d/10-cadestro.conf"}, Escalate: true,
	}); err != nil {
		t.Logf("restore: rm drop-in: %v", err)
	}
	if _, err := r.Run(ctx, sysexec.Command{
		Name: "systemctl", Args: []string{"restart", "systemd-resolved"}, Escalate: true,
	}); err != nil {
		t.Logf("restore: restart resolved: %v", err)
	}
}

func revertLink(t *testing.T, iface string) {
	t.Helper()
	r, err := sysexec.NewRunner(sysexec.Sudo)
	if err != nil {
		t.Logf("revert: NewRunner: %v", err)
		return
	}
	if _, err := r.Run(context.Background(), sysexec.Command{
		Name: "resolvectl", Args: []string{"revert", iface}, Escalate: true,
	}); err != nil {
		t.Logf("revert: resolvectl revert %s: %v", iface, err)
	}
}

func firstRealLink(t *testing.T) string {
	t.Helper()
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if name := e.Name(); name != "lo" {
			return name
		}
	}
	return ""
}
