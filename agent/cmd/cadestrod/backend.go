package main

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	osexec "os/exec"
	"time"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

var geteuidFn = os.Geteuid

func randomBackoff() time.Duration {
	span := int64(maxInitialBackoff - minInitialBackoff)
	if span <= 0 {
		return minInitialBackoff
	}
	return minInitialBackoff + time.Duration(rand.Int64N(span))
}

func applyBackendOverrides(cfg *Config, logger *slog.Logger) (sysexec.PrivilegeBackend, error) {
	resolved, err := setPrivilegeBackend(cfg.PrivilegeBackend, logger)
	if err != nil {
		return resolved, err
	}

	if _, err := osexec.LookPath("systemctl"); err != nil {
		return resolved, fmt.Errorf("service actions require systemctl on PATH: %w", err)
	}
	logger.Info("service manager ready", "manager", "systemd")

	if _, err := osexec.LookPath("cryptsetup"); err != nil {
		logger.Warn("cryptsetup not found; encryption actions are unavailable", "error", err)
	}
	return resolved, nil
}

func setPrivilegeBackend(backend string, logger *slog.Logger) (sysexec.PrivilegeBackend, error) {
	var (
		privilegeTool string
		resolved      sysexec.PrivilegeBackend
	)
	switch backend {
	case "root":

		if euid := geteuidFn(); euid != 0 {
			return sysexec.Direct, fmt.Errorf("privilege backend %q selected but process euid is %d; run as root, or use the sudo/doas backend", backend, euid)
		}
		resolved = sysexec.Direct
		privilegeTool = ""
	case "doas":
		resolved = sysexec.Doas
		privilegeTool = "doas"
	case "sudo":
		resolved = sysexec.Sudo
		privilegeTool = "sudo"
	case "":
		if geteuidFn() == 0 {
			resolved = sysexec.Direct
			privilegeTool = ""
		} else {
			resolved = sysexec.Sudo
			privilegeTool = "sudo"
		}
	default:
		return sysexec.Direct, fmt.Errorf("unknown privilege backend %q", backend)
	}

	if privilegeTool == "" {

		logger.Info("privilege backend set", "backend", "root")
		return resolved, nil
	}
	if _, err := osexec.LookPath(privilegeTool); err != nil {
		return resolved, fmt.Errorf("privilege backend %q selected but %q is not on PATH: %w",
			privilegeTool, privilegeTool, err)
	}
	logger.Info("privilege backend set", "backend", privilegeTool)
	return resolved, nil
}
