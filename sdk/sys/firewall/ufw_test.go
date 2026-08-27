package firewall

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/manchtools/cadestro/sdk/sys/exec"
)

const ufwTestNamespace = "fwtest"

func skipIfNotUFWUsable(t *testing.T) {
	t.Helper()
	if _, err := os.Stat("/usr/sbin/ufw"); err != nil {
		if _, err := os.Stat("/usr/bin/ufw"); err != nil {
			t.Skip("ufw not available on this system")
		}
	}
	if os.Geteuid() != 0 {
		t.Skip("ufw integration tests require root")
	}
}

func TestUFWBuildAddArgs_SimplePortAllow(t *testing.T) {
	got, err := ufwBuildAddArgs(ufwTestNamespace, Rule{
		ID:       "ssh-in",
		Allow:    true,
		Protocol: ProtocolTCP,
		Port:     22,
	})
	if err != nil {
		t.Fatalf("ufwBuildAddArgs: %v", err)
	}
	want := []string{"allow", "22/tcp", "comment", "fwtest:ssh-in"}
	assertArgsEqual(t, got, want)
}

func TestUFWBuildAddArgs_Deny(t *testing.T) {
	got, err := ufwBuildAddArgs(ufwTestNamespace, Rule{
		ID:       "block-ssh",
		Allow:    false,
		Protocol: ProtocolTCP,
		Port:     22,
	})
	if err != nil {
		t.Fatalf("ufwBuildAddArgs: %v", err)
	}

	want := []string{"deny", "22/tcp", "comment", "fwtest:block-ssh"}
	assertArgsEqual(t, got, want)
}

func TestUFWBuildAddArgs_SourceScope(t *testing.T) {
	got, err := ufwBuildAddArgs(ufwTestNamespace, Rule{
		ID:       "from-lan",
		Allow:    true,
		Protocol: ProtocolTCP,
		Port:     22,
		Source:   "10.0.0.0/8",
	})
	if err != nil {
		t.Fatalf("ufwBuildAddArgs: %v", err)
	}

	want := []string{"allow", "from", "10.0.0.0/8", "to", "any", "port", "22", "proto", "tcp", "comment", "fwtest:from-lan"}
	assertArgsEqual(t, got, want)
}

func TestUFWBuildAddArgs_DestScope(t *testing.T) {
	got, err := ufwBuildAddArgs(ufwTestNamespace, Rule{
		ID:       "to-host",
		Allow:    true,
		Protocol: ProtocolTCP,
		Port:     22,
		Dest:     "192.168.1.1",
	})
	if err != nil {
		t.Fatalf("ufwBuildAddArgs: %v", err)
	}

	want := []string{"allow", "from", "any", "to", "192.168.1.1", "port", "22", "proto", "tcp", "comment", "fwtest:to-host"}
	assertArgsEqual(t, got, want)
}

func TestUFWBuildAddArgs_RejectsPortWithoutProto(t *testing.T) {
	_, err := ufwBuildAddArgs(ufwTestNamespace, Rule{
		ID:       "ambiguous",
		Allow:    true,
		Port:     22,
		Protocol: ProtocolAny,
	})
	if !errors.Is(err, ErrInvalidRule) {
		t.Fatalf("want ErrInvalidRule for port-without-proto, got %v", err)
	}
}

func TestUFWBuildAddArgs_RejectsEmpty(t *testing.T) {
	_, err := ufwBuildAddArgs(ufwTestNamespace, Rule{
		ID:    "empty",
		Allow: true,
	})
	if !errors.Is(err, ErrInvalidRule) {
		t.Fatalf("want ErrInvalidRule for empty-rule, got %v", err)
	}
}

func TestUFWFindRuleNumber_ByID(t *testing.T) {
	status := `Status: active

     To                         Action      From
     --                         ------      ----
[ 1] 22/tcp                     ALLOW IN    Anywhere                   # fwtest:ssh-in
[ 2] 53/udp                     ALLOW IN    Anywhere                   # fwtest:dns
[ 3] 22/tcp                     DENY IN     10.0.0.0/8                 # fwtest:block-net
[ 4] 80/tcp                     ALLOW IN    Anywhere                   # cockpit-managed
[ 5] 443/tcp                    ALLOW IN    Anywhere                   # other:web-https
`
	cases := map[string]int{
		"ssh-in":    1,
		"dns":       2,
		"block-net": 3,
	}
	for id, wantNum := range cases {
		gotNum, ok := ufwFindRuleNumber(status, ufwTestNamespace, id)
		if !ok {
			t.Errorf("ufwFindRuleNumber(%q) not found", id)
			continue
		}
		if gotNum != wantNum {
			t.Errorf("ufwFindRuleNumber(%q) = %d, want %d", id, gotNum, wantNum)
		}
	}

	if _, ok := ufwFindRuleNumber(status, ufwTestNamespace, "nonexistent"); ok {
		t.Errorf("ufwFindRuleNumber(nonexistent) reported found")
	}

	if _, ok := ufwFindRuleNumber(status, ufwTestNamespace, "web-https"); ok {
		t.Errorf("ufwFindRuleNumber should not match other-namespace rules")
	}

	if _, ok := ufwFindRuleNumber(status, ufwTestNamespace, "cockpit-managed"); ok {
		t.Errorf("ufwFindRuleNumber should not match non-namespace rules")
	}
}

func TestUFWParseStatus_PicksOutNamespacedRules(t *testing.T) {
	status := `Status: active

     To                         Action      From
     --                         ------      ----
[ 1] 22/tcp                     ALLOW IN    Anywhere                   # fwtest:ssh-in
[ 2] 53/udp                     ALLOW IN    Anywhere                   # fwtest:dns
[ 3] 80/tcp                     ALLOW IN    Anywhere                   # cockpit-managed
[ 4] 22/tcp                     DENY IN     10.0.0.0/8                 # fwtest:block-net
[ 5] 443/tcp                    ALLOW IN    Anywhere                   # other:web-https
`
	rules, err := ufwParseStatus(status, ufwTestNamespace)
	if err != nil {
		t.Fatalf("ufwParseStatus: %v", err)
	}
	ids := make(map[string]Rule)
	for _, r := range rules {
		ids[r.ID] = r
	}
	for _, want := range []string{"ssh-in", "dns", "block-net"} {
		if _, ok := ids[want]; !ok {
			t.Errorf("missing in-namespace rule %q in parsed output: %+v", want, rules)
		}
	}
	if _, ok := ids["cockpit-managed"]; ok {
		t.Errorf("non-namespace rule leaked into List output")
	}
	if _, ok := ids["web-https"]; ok {
		t.Errorf("other-namespace rule leaked into List output")
	}

	if r, ok := ids["block-net"]; ok && r.Allow {
		t.Errorf("block-net round-tripped with Allow=true; want false")
	}
}

func TestUFWParseStatus_InactiveReturnsEmpty(t *testing.T) {
	rules, err := ufwParseStatus("Status: inactive\n", ufwTestNamespace)
	if err != nil {
		t.Fatalf("ufwParseStatus(inactive): %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("inactive ufw should yield zero rules, got %+v", rules)
	}
}

func assertArgsEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("args length mismatch: got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q (full got=%v)", i, got[i], want[i], got)
		}
	}
}

func ufwIntegrationManager(t *testing.T) Manager {
	t.Helper()
	r, err := exec.NewRunner(exec.Direct)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	return newMgr(t, UFW, ufwTestNamespace, r)
}

func TestUFWIntegration_ApplyListRemoveCycle(t *testing.T) {
	skipIfNotUFWUsable(t)
	ctx := context.Background()
	m := ufwIntegrationManager(t)
	rule := Rule{ID: "test-rule", Allow: true, Protocol: ProtocolTCP, Port: 12345}
	t.Cleanup(func() { _ = m.RemoveRule(ctx, rule.ID) })

	if err := m.ApplyRule(ctx, rule); err != nil {
		t.Fatalf("ApplyRule: %v", err)
	}
	rules, err := m.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, r := range rules {
		if r.ID == rule.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("applied rule not visible in List: %+v", rules)
	}
	if err := m.RemoveRule(ctx, rule.ID); err != nil {
		t.Fatalf("RemoveRule: %v", err)
	}
}

func TestUFWIntegration_ApplyIsIdempotent(t *testing.T) {
	skipIfNotUFWUsable(t)
	ctx := context.Background()
	m := ufwIntegrationManager(t)
	rule := Rule{ID: "idemp", Allow: true, Protocol: ProtocolTCP, Port: 12346}
	t.Cleanup(func() { _ = m.RemoveRule(ctx, rule.ID) })

	for i := 0; i < 3; i++ {
		if err := m.ApplyRule(ctx, rule); err != nil {
			t.Fatalf("ApplyRule #%d: %v", i, err)
		}
	}
	rules, _ := m.List(ctx)
	count := 0
	for _, r := range rules {
		if r.ID == rule.ID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("after 3 applies: rule appears %d times; want exactly 1", count)
	}
}
