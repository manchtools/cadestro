package firewall

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/manchtools/cadestro/sdk/sys/exec"
)

const nftTestNamespace = "fwtest"

func skipIfNotNftablesUsable(t *testing.T) {
	t.Helper()
	if _, err := os.Stat("/usr/sbin/nft"); err != nil {
		if _, err2 := os.Stat("/usr/bin/nft"); err2 != nil {
			t.Skip("nft binary not available on this system")
		}
	}
	if os.Geteuid() != 0 {
		t.Skip("nftables integration tests require root")
	}
}

func TestNftBuildScript_AcceptTCP(t *testing.T) {
	got, err := nftBuildApplyScriptStrict(nftTestNamespace, Rule{
		ID:       "ssh-in",
		Allow:    true,
		Protocol: ProtocolTCP,
		Port:     22,
	}, 0)
	if err != nil {
		t.Fatalf("nftBuildApplyScriptStrict(accept tcp): %v", err)
	}
	want := strings.Join([]string{
		`add table inet fwtest_filter`,
		`add chain inet fwtest_filter input { type filter hook input priority 0; policy accept; }`,
		`add rule inet fwtest_filter input tcp dport 22 accept comment "ssh-in"`,
	}, "\n") + "\n"
	if got != want {
		t.Fatalf("nftBuildApplyScriptStrict:\n--- got  ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestNftBuildScript_DropUDP(t *testing.T) {
	got, err := nftBuildApplyScriptStrict(nftTestNamespace, Rule{
		ID:       "block-dns",
		Allow:    false,
		Protocol: ProtocolUDP,
		Port:     53,
	}, 0)
	if err != nil {
		t.Fatalf("nftBuildApplyScriptStrict(drop udp): %v", err)
	}
	if !strings.Contains(got, `add rule inet fwtest_filter input udp dport 53 drop comment "block-dns"`) {
		t.Fatalf("missing expected rule line:\n%s", got)
	}
}

func TestNftBuildScript_WithSourceAndDest(t *testing.T) {
	got, err := nftBuildApplyScriptStrict(nftTestNamespace, Rule{
		ID:       "from-vpn",
		Allow:    true,
		Protocol: ProtocolTCP,
		Port:     443,
		Source:   "10.0.0.0/8",
		Dest:     "192.168.1.10",
	}, 0)
	if err != nil {
		t.Fatalf("nftBuildApplyScriptStrict(source and dest): %v", err)
	}
	want := `add rule inet fwtest_filter input ip saddr 10.0.0.0/8 ip daddr 192.168.1.10 tcp dport 443 accept comment "from-vpn"`
	if !strings.Contains(got, want) {
		t.Fatalf("missing expected rule line:\n%s", got)
	}
}

func TestNftBuildScript_AnyProtocolNoPort(t *testing.T) {
	got, err := nftBuildApplyScriptStrict(nftTestNamespace, Rule{
		ID:     "trusted-net",
		Allow:  true,
		Source: "172.16.0.0/12",
	}, 0)
	if err != nil {
		t.Fatalf("nftBuildApplyScriptStrict(any protocol no port): %v", err)
	}
	want := `add rule inet fwtest_filter input ip saddr 172.16.0.0/12 accept comment "trusted-net"`
	if !strings.Contains(got, want) {
		t.Fatalf("missing expected rule line:\n%s", got)
	}
}

func TestNftBuildScript_ReplacesExistingHandle(t *testing.T) {
	got, err := nftBuildApplyScriptStrict(nftTestNamespace, Rule{
		ID:       "ssh-in",
		Allow:    true,
		Protocol: ProtocolTCP,
		Port:     22,
	}, 17)
	if err != nil {
		t.Fatalf("nftBuildApplyScriptStrict(replace handle): %v", err)
	}
	if !strings.Contains(got, `delete rule inet fwtest_filter input handle 17`) {
		t.Fatalf("missing delete-of-old-handle line:\n%s", got)
	}
	if !strings.Contains(got, `add rule inet fwtest_filter input tcp dport 22 accept comment "ssh-in"`) {
		t.Fatalf("missing add-new-rule line:\n%s", got)
	}
}

func TestNftBuildScript_WithIPv6Source(t *testing.T) {
	got, err := nftBuildApplyScriptStrict(nftTestNamespace, Rule{
		ID:       "from-vpn6",
		Allow:    true,
		Protocol: ProtocolTCP,
		Port:     443,
		Source:   "2001:db8::/32",
	}, 0)
	if err != nil {
		t.Fatalf("nftBuildApplyScriptStrict(IPv6 source): %v", err)
	}
	want := `add rule inet fwtest_filter input ip6 saddr 2001:db8::/32 tcp dport 443 accept comment "from-vpn6"`
	if !strings.Contains(got, want) {
		t.Fatalf("missing IPv6 rule line:\n%s", got)
	}
}

func TestNftBuildScript_WithIPv6BareAddress(t *testing.T) {
	got, err := nftBuildApplyScriptStrict(nftTestNamespace, Rule{
		ID:     "from-localhost6",
		Allow:  true,
		Source: "::1",
	}, 0)
	if err != nil {
		t.Fatalf("nftBuildApplyScriptStrict(bare IPv6): %v", err)
	}
	if !strings.Contains(got, `ip6 saddr ::1`) {
		t.Fatalf("missing ip6 saddr line:\n%s", got)
	}
}

func TestNftBuildScript_RejectsMixedIPFamilies(t *testing.T) {
	_, err := nftBuildApplyScriptStrict(nftTestNamespace, Rule{
		ID:     "mixed-fam",
		Allow:  true,
		Source: "10.0.0.0/8",
		Dest:   "2001:db8::1",
	}, 0)
	if err == nil {
		t.Fatalf("nftBuildApplyScriptStrict(mixed v4 source + v6 dest) = nil; want ErrInvalidRule")
	}
	if !errors.Is(err, ErrInvalidRule) {
		t.Fatalf("err = %v; want ErrInvalidRule", err)
	}
}

func TestNftBuildScript_RejectsInvalidSource(t *testing.T) {
	_, err := nftBuildApplyScriptStrict(nftTestNamespace, Rule{
		ID:     "bad-src",
		Allow:  true,
		Source: "not-an-ip",
	}, 0)
	if !errors.Is(err, ErrInvalidRule) {
		t.Fatalf("nftBuildApplyScriptStrict(garbage source) = %v; want ErrInvalidRule", err)
	}
}

func TestNftBuildScript_RejectsAnyProtocolWithPort(t *testing.T) {
	script, err := nftBuildApplyScriptStrict(nftTestNamespace, Rule{
		ID:    "any-22",
		Allow: true,
		Port:  22,
	}, 0)
	if err == nil {
		t.Fatalf("nftBuildApplyScriptStrict(port without protocol) returned nil err; got script:\n%s", script)
	}
	if !strings.Contains(err.Error(), "protocol") {
		t.Fatalf("err = %v; want a message mentioning protocol", err)
	}
}

func TestNftParseRules_ReturnsAllInTable(t *testing.T) {

	input := `{
		"nftables": [
			{"metainfo": {"version": "1.0.0"}},
			{"table": {"family": "inet", "name": "fwtest_filter", "handle": 1}},
			{"chain": {"family": "inet", "table": "fwtest_filter", "name": "input", "handle": 2,
				"type": "filter", "hook": "input", "prio": 0, "policy": "accept"}},
			{"rule": {"family": "inet", "table": "fwtest_filter", "chain": "input", "handle": 8,
				"expr": [
					{"match": {"op": "==", "left": {"payload": {"protocol": "tcp", "field": "dport"}}, "right": 22}},
					{"accept": null}
				]}},
			{"rule": {"family": "inet", "table": "fwtest_filter", "chain": "input", "handle": 9,
				"expr": [
					{"match": {"op": "==", "left": {"payload": {"protocol": "tcp", "field": "dport"}}, "right": 443}},
					{"accept": null}
				],
				"comment": "web-https"}}
		]
	}`

	rules, err := nftParseRules(json.RawMessage(input))
	if err != nil {
		t.Fatalf("nftParseRules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("got %d rules; want 1 (only the commented one)", len(rules))
	}
	want := Rule{
		ID:       "web-https",
		Allow:    true,
		Protocol: ProtocolTCP,
		Port:     443,
	}
	if !reflect.DeepEqual(rules[0], want) {
		t.Fatalf("rules[0] = %+v; want %+v", rules[0], want)
	}
}

func TestNftParseRules_NoTableYet(t *testing.T) {
	rules, err := nftParseRules(json.RawMessage(`{"nftables":[]}`))
	if err != nil {
		t.Fatalf("nftParseRules(empty): %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("rules = %v; want empty", rules)
	}
}

func TestNftFindRuleHandle(t *testing.T) {
	input := `{"nftables":[
		{"rule": {"family": "inet", "table": "fwtest_filter", "chain": "input", "handle": 9,
			"expr": [{"accept": null}], "comment": "web-https"}}
	]}`
	handle, found := nftFindRuleHandle(json.RawMessage(input), "web-https")
	if !found || handle != 9 {
		t.Fatalf("nftFindRuleHandle(web-https) = (%d, %v); want (9, true)", handle, found)
	}
	if _, found := nftFindRuleHandle(json.RawMessage(input), "no-such"); found {
		t.Fatal("nftFindRuleHandle(missing) reported found=true")
	}
}

func nftIntegrationBackend(t *testing.T) *nftables {
	t.Helper()
	r, err := exec.NewRunner(exec.Direct)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	return &nftables{base: base{ns: nftTestNamespace, cmd: cmd{r: r}}}
}

func TestNftablesIntegration_ApplyListRemoveCycle(t *testing.T) {
	skipIfNotNftablesUsable(t)
	n := nftIntegrationBackend(t)
	t.Cleanup(func() { _ = n.nftDeleteManagedTable(context.Background()) })

	ctx := context.Background()
	rule := Rule{ID: "test-ssh", Allow: true, Protocol: ProtocolTCP, Port: 22}

	if err := n.ApplyRule(ctx, rule); err != nil {
		t.Fatalf("ApplyRule: %v", err)
	}
	rules, err := n.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rules) != 1 || rules[0].ID != "test-ssh" {
		t.Fatalf("List = %+v; want [{ID:test-ssh ...}]", rules)
	}
	if err := n.RemoveRule(ctx, "test-ssh"); err != nil {
		t.Fatalf("RemoveRule: %v", err)
	}
	rules, _ = n.List(ctx)
	if len(rules) != 0 {
		t.Fatalf("after remove: List = %+v; want empty", rules)
	}
}

func TestNftablesIntegration_ApplyIsIdempotent(t *testing.T) {
	skipIfNotNftablesUsable(t)
	n := nftIntegrationBackend(t)
	t.Cleanup(func() { _ = n.nftDeleteManagedTable(context.Background()) })

	ctx := context.Background()
	rule := Rule{ID: "idemp", Allow: true, Protocol: ProtocolTCP, Port: 8080}
	for i := 0; i < 3; i++ {
		if err := n.ApplyRule(ctx, rule); err != nil {
			t.Fatalf("ApplyRule #%d: %v", i, err)
		}
	}
	rules, _ := n.List(ctx)
	if len(rules) != 1 {
		t.Fatalf("after 3 applies: rules = %+v; want exactly 1", rules)
	}
}

func TestNftablesIntegration_RemoveOnMissingIsNoOp(t *testing.T) {
	skipIfNotNftablesUsable(t)
	n := nftIntegrationBackend(t)
	t.Cleanup(func() { _ = n.nftDeleteManagedTable(context.Background()) })

	if err := n.RemoveRule(context.Background(), "never-applied"); err != nil {
		t.Fatalf("RemoveRule(missing) = %v; want nil", err)
	}
}
