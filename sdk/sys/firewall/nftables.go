package firewall

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
)

type nftables struct {
	base
}

var _ Manager = (*nftables)(nil)

const (
	nftFamily = "inet"
	nftChain  = "input"
)

func nftTableName(namespace string) string {
	return namespace + "_filter"
}

func nftAddressFamily(addr string) (string, error) {
	if ip, _, err := net.ParseCIDR(addr); err == nil {
		if ip.To4() != nil {
			return "ip", nil
		}
		return "ip6", nil
	}
	if ip := net.ParseIP(addr); ip != nil {
		if ip.To4() != nil {
			return "ip", nil
		}
		return "ip6", nil
	}
	return "", fmt.Errorf("%w: %q is not a valid IP address or CIDR", ErrInvalidRule, addr)
}

// ApplyRule installs or updates rule. The whole change (table + chain ensure,
// optional delete-of-previous, add) goes into one atomic `nft -f -` batch.
func (n *nftables) ApplyRule(ctx context.Context, rule Rule) error {
	if err := validateRule(rule); err != nil {
		return err
	}

	var handle int64
	if raw, lerr := n.nftListJSON(ctx); lerr == nil {
		if h, ok := nftFindRuleHandle(raw, rule.ID); ok {
			handle = h
		}
	}

	script, err := nftBuildApplyScriptStrict(n.ns, rule, handle)
	if err != nil {
		return err
	}
	return n.nftRunScript(ctx, script)
}

// RemoveRule deletes the rule with the given ID; a missing table or rule is a
// no-op (the post-condition "absent" already holds).
func (n *nftables) RemoveRule(ctx context.Context, id string) error {
	if err := validateRuleID(id); err != nil {
		return err
	}
	raw, err := n.nftListJSON(ctx)
	if err != nil {
		if isNoTable(err) {
			return nil
		}
		return err
	}
	handle, ok := nftFindRuleHandle(raw, id)
	if !ok {
		return nil
	}
	script := fmt.Sprintf("delete rule %s %s %s handle %d\n", nftFamily, nftTableName(n.ns), nftChain, handle)
	return n.nftRunScript(ctx, script)
}

// List returns every managed rule in this namespace's table. A missing table is
// an explicit absence: it returns a wrapped os.ErrNotExist rather than an empty
// slice, so a caller can never confuse "this namespace was never provisioned"
// with "provisioned, currently zero rules". Callers that want absent-as-empty
// opt in with errors.Is(err, os.ErrNotExist).
func (n *nftables) List(ctx context.Context) ([]Rule, error) {
	raw, err := n.nftListJSON(ctx)
	if err != nil {
		if isNoTable(err) {
			return nil, fmt.Errorf("list nftables rules: namespace %q has no table: %w", n.ns, os.ErrNotExist)
		}
		return nil, err
	}
	return nftParseRules(raw)
}

func (n *nftables) nftListJSON(ctx context.Context) ([]byte, error) {
	res, err := n.run(ctx, "nft", "-j", "list", "table", nftFamily, nftTableName(n.ns))
	if err != nil {
		return nil, fmt.Errorf("nft list table: %w", err)
	}
	return []byte(res.Stdout), nil
}

func (n *nftables) nftRunScript(ctx context.Context, script string) error {
	if _, err := n.runStdin(ctx, script, "nft", "-f", "-"); err != nil {
		return fmt.Errorf("nft -f -: %w", err)
	}
	return nil
}

func (n *nftables) nftDeleteManagedTable(ctx context.Context) error {
	script := fmt.Sprintf("delete table %s %s\n", nftFamily, nftTableName(n.ns))
	_, err := n.runStdin(ctx, script, "nft", "-f", "-")
	return err
}

func nftBuildApplyScriptStrict(namespace string, rule Rule, replaceHandle int64) (string, error) {
	if rule.Port > 0 && rule.Protocol == ProtocolAny {
		return "", fmt.Errorf("%w: port %d set without a concrete protocol; nft requires tcp or udp", ErrInvalidRule, rule.Port)
	}

	table := nftTableName(namespace)
	var b strings.Builder

	fmt.Fprintf(&b, "add table %s %s\n", nftFamily, table)
	fmt.Fprintf(&b, "add chain %s %s %s { type filter hook input priority 0; policy accept; }\n",
		nftFamily, table, nftChain)

	if replaceHandle > 0 {
		fmt.Fprintf(&b, "delete rule %s %s %s handle %d\n",
			nftFamily, table, nftChain, replaceHandle)
	}

	var parts []string
	parts = append(parts, "add rule", nftFamily, table, nftChain)

	var srcFam, dstFam string
	if rule.Source != "" {
		fam, err := nftAddressFamily(rule.Source)
		if err != nil {
			return "", fmt.Errorf("source %q: %w", rule.Source, err)
		}
		srcFam = fam
		parts = append(parts, fam, "saddr", rule.Source)
	}
	if rule.Dest != "" {
		fam, err := nftAddressFamily(rule.Dest)
		if err != nil {
			return "", fmt.Errorf("dest %q: %w", rule.Dest, err)
		}
		dstFam = fam
		parts = append(parts, fam, "daddr", rule.Dest)
	}
	if srcFam != "" && dstFam != "" && srcFam != dstFam {
		return "", fmt.Errorf("%w: source family %s differs from dest family %s; a rule that mixes IPv4 and IPv6 match expressions can never match a real packet", ErrInvalidRule, srcFam, dstFam)
	}
	if rule.Protocol == ProtocolTCP || rule.Protocol == ProtocolUDP {
		parts = append(parts, string(rule.Protocol))
		if rule.Port > 0 {
			parts = append(parts, "dport", fmt.Sprintf("%d", rule.Port))
		}
	}

	verdict := "accept"
	if !rule.Allow {
		verdict = "drop"
	}
	parts = append(parts, verdict)

	parts = append(parts, "comment", fmt.Sprintf(`"%s"`, rule.ID))

	b.WriteString(strings.Join(parts, " "))
	b.WriteString("\n")
	return b.String(), nil
}

type nftRuleObject struct {
	Family  string            `json:"family"`
	Table   string            `json:"table"`
	Chain   string            `json:"chain"`
	Handle  int64             `json:"handle"`
	Comment string            `json:"comment"`
	Expr    []json.RawMessage `json:"expr"`
}

type nftListItem struct {
	Table *json.RawMessage `json:"table,omitempty"`
	Chain *json.RawMessage `json:"chain,omitempty"`
	Rule  *nftRuleObject   `json:"rule,omitempty"`
}

type nftListEnvelope struct {
	Nftables []nftListItem `json:"nftables"`
}

func nftParseRules(raw []byte) ([]Rule, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var env nftListEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("unmarshal nft list: %w", err)
	}
	var rules []Rule
	for _, item := range env.Nftables {
		if item.Rule == nil {
			continue
		}

		if item.Rule.Comment == "" {
			continue
		}
		rule := Rule{ID: item.Rule.Comment}
		applyExprToRule(item.Rule.Expr, &rule)
		rules = append(rules, rule)
	}
	return rules, nil
}

func nftFindRuleHandle(raw []byte, id string) (int64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var env nftListEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return 0, false
	}
	for _, item := range env.Nftables {
		if item.Rule == nil {
			continue
		}
		if item.Rule.Comment == id {
			return item.Rule.Handle, true
		}
	}
	return 0, false
}

func applyExprToRule(expr []json.RawMessage, out *Rule) {
	for _, e := range expr {
		var verdict struct {
			Accept json.RawMessage `json:"accept,omitempty"`
			Drop   json.RawMessage `json:"drop,omitempty"`
		}
		if err := json.Unmarshal(e, &verdict); err == nil {
			if verdict.Accept != nil {
				out.Allow = true
			} else if verdict.Drop != nil {
				out.Allow = false
			}
		}
		var match struct {
			Match *struct {
				Op   string `json:"op"`
				Left struct {
					Payload *struct {
						Protocol string `json:"protocol"`
						Field    string `json:"field"`
					} `json:"payload"`
				} `json:"left"`
				Right json.RawMessage `json:"right"`
			} `json:"match"`
		}
		if err := json.Unmarshal(e, &match); err == nil && match.Match != nil {
			if pl := match.Match.Left.Payload; pl != nil {
				switch pl.Field {
				case "dport":
					out.Protocol = Protocol(pl.Protocol)
					var port int
					_ = json.Unmarshal(match.Match.Right, &port)
					out.Port = port
				case "saddr":
					out.Source = nftDecodeAddr(match.Match.Right)
				case "daddr":
					out.Dest = nftDecodeAddr(match.Match.Right)
				}
			}
		}
	}
}

func nftDecodeAddr(raw json.RawMessage) string {
	var bare string
	if err := json.Unmarshal(raw, &bare); err == nil {
		return bare
	}
	var p struct {
		Prefix *struct {
			Addr string `json:"addr"`
			Len  int    `json:"len"`
		} `json:"prefix"`
	}
	if err := json.Unmarshal(raw, &p); err == nil && p.Prefix != nil {
		return fmt.Sprintf("%s/%d", p.Prefix.Addr, p.Prefix.Len)
	}
	return ""
}
