package dns

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/fs"
)

var resolvConfPath = "/run/systemd/resolve/resolv.conf"

const (
	resolvedDropInDir  = "/etc/systemd/resolved.conf.d"
	resolvedDropInPath = resolvedDropInDir + "/10-cadestro.conf"
)

type resolvedManager struct {
	r   exec.Runner
	fsm fsManager
}

// Get reads and parses the active resolver configuration.
func (m *resolvedManager) Get(ctx context.Context) (State, error) {
	data, err := os.ReadFile(resolvConfPath)
	if err != nil {
		return State{}, fmt.Errorf("read %s: %w", resolvConfPath, err)
	}
	return parseResolvConf(data), nil
}

// Apply validates cfg, then installs it. A scoped Interface uses runtime
// resolvectl per-link settings; an empty Interface writes the persistent global
// drop-in and restarts resolved to pick it up.
func (m *resolvedManager) Apply(ctx context.Context, cfg Config) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}

	if cfg.Interface != "" {
		if err := runPriv(ctx, m.r, "resolvectl", resolvectlDNSArgs(cfg.Interface, cfg.Nameservers)...); err != nil {
			return fmt.Errorf("resolvectl dns %s: %w", cfg.Interface, err)
		}
		if len(cfg.SearchDomains) > 0 {
			if err := runPriv(ctx, m.r, "resolvectl", resolvectlDomainArgs(cfg.Interface, cfg.SearchDomains)...); err != nil {

				if rvErr := runPriv(ctx, m.r, "resolvectl", "revert", cfg.Interface); rvErr != nil {
					return fmt.Errorf("resolvectl domain %s failed and resetting the link failed too (%v), so %s is left with the new nameservers and the old search domains: %w", cfg.Interface, rvErr, cfg.Interface, err)
				}
				return fmt.Errorf("resolvectl domain %s (link reset to systemd-resolved's per-link defaults, not to its pre-call state): %w", cfg.Interface, err)
			}
		}
		return nil
	}

	body, err := renderDropIn(cfg)
	if err != nil {
		return err
	}
	if err := m.fsm.Mkdir(ctx, resolvedDropInDir, fs.MkdirOptions{Mode: 0o755, Owner: "root", Group: "root", Recursive: true}); err != nil {
		return fmt.Errorf("create %s: %w", resolvedDropInDir, err)
	}
	if err := m.fsm.WriteFile(ctx, resolvedDropInPath, body, fs.WriteOptions{Mode: 0o644, Owner: "root", Group: "root"}); err != nil {
		return fmt.Errorf("write %s: %w", resolvedDropInPath, err)
	}
	if err := runPriv(ctx, m.r, "systemctl", "restart", "systemd-resolved"); err != nil {
		return fmt.Errorf("restart systemd-resolved: %w", err)
	}
	return nil
}

func resolvectlDNSArgs(iface string, nameservers []string) []string {
	return append([]string{"dns", iface}, exec.SeparatePositionals(nil, nameservers...)...)
}

func resolvectlDomainArgs(iface string, domains []string) []string {
	return append([]string{"domain", iface}, exec.SeparatePositionals(nil, domains...)...)
}

var renderDropIn = renderResolvedDropIn

func renderResolvedDropIn(cfg Config) ([]byte, error) {
	var b strings.Builder
	b.WriteString("# Managed by cadestrod — do not edit by hand.\n")
	b.WriteString("[Resolve]\n")
	if len(cfg.Nameservers) > 0 {
		v := strings.Join(cfg.Nameservers, " ")
		if strings.ContainsAny(v, "\n\r") {
			return nil, fmt.Errorf("%w: nameserver list contains a newline", ErrInvalidConfig)
		}
		b.WriteString("DNS=" + v + "\n")
	}
	if len(cfg.SearchDomains) > 0 {
		v := strings.Join(cfg.SearchDomains, " ")
		if strings.ContainsAny(v, "\n\r") {
			return nil, fmt.Errorf("%w: search-domain list contains a newline", ErrInvalidConfig)
		}
		b.WriteString("Domains=" + v + "\n")
	}
	return []byte(b.String()), nil
}

func parseResolvConf(data []byte) State {
	var st State
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		fields := strings.Fields(line)

		for i, f := range fields {
			if strings.HasPrefix(f, "#") || strings.HasPrefix(f, ";") {
				fields = fields[:i]
				break
			}
		}
		switch fields[0] {
		case "nameserver":
			if len(fields) >= 2 {
				st.Nameservers = append(st.Nameservers, fields[1])
			}
		case "search", "domain":
			st.SearchDomains = append([]string(nil), fields[1:]...)
		}
	}
	return st
}
