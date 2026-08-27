package netconfig

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/manchtools/cadestro/sdk/sys/fs"
)

const networkDir = "/etc/systemd/network"

type networkdBackend struct {
	base
	fsm fsManager
}

// Apply validates cfg, writes /etc/systemd/network/<name>.network, and reloads
// networkd so the unit takes effect.
func (b *networkdBackend) Apply(ctx context.Context, cfg InterfaceConfig) error {
	if err := validateInterfaceConfig(cfg); err != nil {
		return err
	}
	body := renderNetworkUnit(cfg)
	path := networkDir + "/" + cfg.Name + ".network"
	if err := b.fsm.WriteFile(ctx, path, []byte(body), fs.WriteOptions{Mode: 0o644, Owner: "root", Group: "root"}); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := runPriv(ctx, b.r, "networkctl", "reload"); err != nil {
		return fmt.Errorf("networkctl reload: %w", err)
	}
	return nil
}

func renderNetworkUnit(cfg InterfaceConfig) string {
	var b strings.Builder
	b.WriteString("# Managed by cadestrod — do not edit by hand.\n")
	b.WriteString("[Match]\nName=" + cfg.Name + "\n\n")
	b.WriteString("[Network]\n")
	if cfg.Mode == DHCP {
		b.WriteString("DHCP=yes\n")
	} else {
		for _, a := range cfg.Addresses {
			b.WriteString("Address=" + a + "\n")
		}
		if cfg.Gateway != "" {
			b.WriteString("Gateway=" + cfg.Gateway + "\n")
		}
	}
	for _, d := range cfg.DNS {
		b.WriteString("DNS=" + d + "\n")
	}
	if cfg.MTU != 0 {
		b.WriteString("\n[Link]\nMTUBytes=" + strconv.Itoa(cfg.MTU) + "\n")
	}
	for _, rt := range cfg.Routes {
		b.WriteString("\n[Route]\n")

		if rt.Destination != "default" {
			b.WriteString("Destination=" + rt.Destination + "\n")
		}
		b.WriteString("Gateway=" + rt.Gateway + "\n")
		if rt.Metric != 0 {
			b.WriteString("Metric=" + strconv.Itoa(rt.Metric) + "\n")
		}
	}
	return b.String()
}
