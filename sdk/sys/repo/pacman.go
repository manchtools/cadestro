package repo

import (
	"context"
	"fmt"
	"strings"

	"github.com/manchtools/cadestro/sdk/sys/fs"
)

const pacmanConf = "/etc/pacman.conf"

func removePacmanSection(content, name string) string {
	sectionHeader := "[" + name + "]"
	lines := strings.Split(content, "\n")
	var result []string
	inSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == sectionHeader {
			inSection = true
			continue
		}
		if inSection && strings.HasPrefix(trimmed, "[") {
			inSection = false
		}
		if !inSection {
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}

func (m *manager) applyPacman(ctx context.Context, name string, c *PacmanConfig) (Outcome, error) {
	var log strings.Builder
	confBytes, err := m.fsm.ReadFile(ctx, pacmanConf)
	if err != nil && !isReadAbsent(err) {
		return Outcome{}, fmt.Errorf("read pacman.conf: %w", err)
	}
	confStr := string(confBytes)

	var section strings.Builder
	fmt.Fprintf(&section, "\n[%s]\n", name)
	if c.SigLevel != "" {
		fmt.Fprintf(&section, "SigLevel = %s\n", c.SigLevel)
	}
	fmt.Fprintf(&section, "Server = %s\n", c.Server)

	newConf := confStr
	if strings.Contains(confStr, "["+name+"]") {
		newConf = removePacmanSection(confStr, name)
	}
	newConf += section.String()

	if newConf == confStr {
		fmt.Fprintf(&log, "repository %s already configured\n", name)
		return out(log.String(), false), nil
	}

	if err := m.fsm.WriteFile(ctx, pacmanConf, []byte(newConf), fs.WriteOptions{Mode: 0o644}); err != nil {
		return Outcome{}, fmt.Errorf("write pacman.conf: %w", err)
	}
	fmt.Fprintf(&log, "configured repository: %s\n", name)

	m.runNonFatal(ctx, &log, "warning: failed to sync repository database", "pacman", "-Sy", "--noconfirm")
	return out(log.String(), true), nil
}

func (m *manager) removePacman(ctx context.Context, name string) (Outcome, error) {

	if err := validatePacmanName(name); err != nil {
		return Outcome{}, err
	}
	var log strings.Builder
	confBytes, err := m.fsm.ReadFile(ctx, pacmanConf)
	if err != nil && !isReadAbsent(err) {
		return Outcome{}, fmt.Errorf("read pacman.conf: %w", err)
	}
	confStr := string(confBytes)
	if !strings.Contains(confStr, "["+name+"]") {
		fmt.Fprintf(&log, "repository %s not found, nothing to remove\n", name)
		return out(log.String(), false), nil
	}
	newConf := removePacmanSection(confStr, name)
	if err := m.fsm.WriteFile(ctx, pacmanConf, []byte(newConf), fs.WriteOptions{Mode: 0o644}); err != nil {
		return Outcome{}, fmt.Errorf("write pacman.conf: %w", err)
	}
	fmt.Fprintf(&log, "removed repository: %s\n", name)
	return out(log.String(), true), nil
}
