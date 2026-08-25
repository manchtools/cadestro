package firewall

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type ufw struct {
	base
}

var _ Manager = (*ufw)(nil)

var ufwStatusRuleRE = regexp.MustCompile(`^\[\s*(\d+)\]\s+(\S+)\s+(ALLOW|DENY|REJECT|LIMIT)(?:\s+(?:IN|OUT))?\s+(.+?)\s*#\s*(.+?)\s*$`)

func ufwCommentIdentity(namespace, id string) string {
	return namespace + ":" + id
}

// ApplyRule installs or updates rule. Any previous variant with the same ID is
// deleted first so the final ruleset has exactly one rule per ID.
func (u *ufw) ApplyRule(ctx context.Context, rule Rule) error {
	if err := validateRule(rule); err != nil {
		return err
	}

	args, err := ufwBuildAddArgs(u.ns, rule)
	if err != nil {
		return err
	}

	if status, err := u.ufwStatusNumbered(ctx); err == nil {
		if num, ok := ufwFindRuleNumber(status, u.ns, rule.ID); ok {
			if err := u.ufwDeleteByNumber(ctx, num); err != nil {
				return fmt.Errorf("ufw delete existing rule %d: %w", num, err)
			}
		}
	}
	if _, err := u.run(ctx, "ufw", args...); err != nil {
		return fmt.Errorf("ufw %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// RemoveRule deletes the rule with the given ID; an inactive ufw or a missing
// rule is a no-op.
func (u *ufw) RemoveRule(ctx context.Context, id string) error {
	if err := validateRuleID(id); err != nil {
		return err
	}
	status, err := u.ufwStatusNumbered(ctx)
	if err != nil {

		return nil
	}
	num, ok := ufwFindRuleNumber(status, u.ns, id)
	if !ok {
		return nil
	}
	return u.ufwDeleteByNumber(ctx, num)
}

// List returns every managed rule (comment prefix `<namespace>:`). An inactive
// ufw exits 0 with "Status: inactive" and is parsed below into an empty rule set
// (the firewall IS provisioned, it just holds no rules). Any error reaching here
// is a genuine can't-determine-state failure — ufw absent, or escalation denied —
// and propagates rather than being silently reported as "zero managed rules".
func (u *ufw) List(ctx context.Context) ([]Rule, error) {
	status, err := u.ufwStatusNumbered(ctx)
	if err != nil {
		return nil, err
	}
	return ufwParseStatus(status, u.ns)
}

func (u *ufw) ufwStatusNumbered(ctx context.Context) (string, error) {
	res, err := u.run(ctx, "ufw", "status", "numbered")
	if err != nil {
		return "", fmt.Errorf("ufw status numbered: %w", err)
	}
	return res.Stdout, nil
}

func (u *ufw) ufwDeleteByNumber(ctx context.Context, num int) error {
	if _, err := u.run(ctx, "ufw", "--force", "delete", strconv.Itoa(num)); err != nil {
		return fmt.Errorf("ufw --force delete %d: %w", num, err)
	}
	return nil
}

func ufwValidateRule(rule Rule) error {
	if rule.Port > 0 && rule.Protocol == ProtocolAny {
		return fmt.Errorf("%w: port %d set without a concrete protocol; ufw requires tcp or udp", ErrInvalidRule, rule.Port)
	}
	if rule.Port == 0 && rule.Protocol == ProtocolAny && rule.Source == "" && rule.Dest == "" {
		return fmt.Errorf("%w: ufw rule needs at least Port, Protocol, Source, or Dest", ErrInvalidRule)
	}
	return nil
}

func ufwBuildAddArgs(namespace string, rule Rule) ([]string, error) {
	if err := ufwValidateRule(rule); err != nil {
		return nil, err
	}
	verdict := "allow"
	if !rule.Allow {
		verdict = "deny"
	}
	args := []string{verdict}

	scoped := rule.Source != "" || rule.Dest != ""
	switch {
	case scoped:

		src := rule.Source
		if src == "" {
			src = "any"
		}
		dst := rule.Dest
		if dst == "" {
			dst = "any"
		}
		args = append(args, "from", src, "to", dst)
		if rule.Port > 0 {
			args = append(args, "port", strconv.Itoa(rule.Port))
		}
		if rule.Protocol == ProtocolTCP || rule.Protocol == ProtocolUDP {
			args = append(args, "proto", string(rule.Protocol))
		}
	case rule.Port > 0:

		args = append(args, fmt.Sprintf("%d/%s", rule.Port, rule.Protocol))
	default:

		args = append(args, "from", "any", "to", "any", "proto", string(rule.Protocol))
	}

	args = append(args, "comment", ufwCommentIdentity(namespace, rule.ID))
	return args, nil
}

func ufwFindRuleNumber(status, namespace, id string) (int, bool) {
	target := ufwCommentIdentity(namespace, id)
	for _, line := range strings.Split(status, "\n") {
		m := ufwStatusRuleRE.FindStringSubmatch(strings.TrimRight(line, " \t"))
		if m == nil {
			continue
		}
		if m[5] == target {
			n, err := strconv.Atoi(m[1])
			if err != nil {
				continue
			}
			return n, true
		}
	}
	return 0, false
}

func ufwParseStatus(status, namespace string) ([]Rule, error) {
	if strings.Contains(status, "Status: inactive") {
		return nil, nil
	}
	prefix := namespace + ":"
	var rules []Rule
	for _, line := range strings.Split(status, "\n") {
		m := ufwStatusRuleRE.FindStringSubmatch(strings.TrimRight(line, " \t"))
		if m == nil {
			continue
		}
		comment := m[5]
		id, ok := strings.CutPrefix(comment, prefix)
		if !ok {
			continue
		}
		rule := Rule{
			ID:    id,
			Allow: m[3] == "ALLOW",
		}

		ufwParseToColumn(m[2], &rule)

		from := strings.TrimSpace(m[4])
		if from != "" && from != "Anywhere" && from != "Anywhere (v6)" {
			rule.Source = from
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func ufwParseToColumn(col string, out *Rule) {
	col = strings.TrimSpace(col)
	if col == "Anywhere" || col == "Anywhere (v6)" {
		return
	}

	if slash := strings.Index(col, "/"); slash > 0 {
		if p, err := strconv.Atoi(col[:slash]); err == nil {
			out.Port = p
		}
		proto := col[slash+1:]
		if proto == "tcp" || proto == "udp" {
			out.Protocol = Protocol(proto)
		}
		return
	}

	if p, err := strconv.Atoi(col); err == nil {
		out.Port = p
		return
	}

	out.Dest = col
}
