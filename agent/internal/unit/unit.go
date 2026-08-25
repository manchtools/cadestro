package unit

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"
	"text/template"
)

const ServiceName = "cadestrod"

const UnitName = ServiceName + ".service"

const restrictRealtimeMinVersion = 257

//go:embed cadestrod.service.tmpl
var unitTemplate string

var tmpl = template.Must(template.New(UnitName).Parse(unitTemplate))

type Manager interface {
	Version(ctx context.Context) (int, error)
	ReadUnit(ctx context.Context, unit string) (string, error)
	WriteUnit(ctx context.Context, unit, content string) error
	DaemonReload(ctx context.Context) error
	NeedsReload(ctx context.Context, unit string) (bool, error)
}

type Params struct {
	BinaryPath       string
	DataDir          string
	RestrictRealtime bool
}

func Render(p Params) (string, error) {
	if err := validateUnitPath("BinaryPath", p.BinaryPath); err != nil {
		return "", err
	}
	if err := validateUnitPath("DataDir", p.DataDir); err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, p); err != nil {
		return "", fmt.Errorf("unit render: %w", err)
	}
	return buf.String(), nil
}

func validateUnitPath(field, value string) error {
	if !strings.HasPrefix(value, "/") {
		return fmt.Errorf("unit render: %s %q must be an absolute path", field, value)
	}
	for _, r := range value {
		switch {
		case r <= 0x20 || r == 0x7f:
			return fmt.Errorf("unit render: %s %q contains whitespace or a control character", field, value)
		case r == '"' || r == '\'' || r == '\\' || r == '%' || r == '$':

			return fmt.Errorf("unit render: %s %q contains %q, which systemd unit syntax interprets", field, value, string(r))
		}
	}
	return nil
}

func Reconcile(ctx context.Context, mgr Manager, logger *slog.Logger, p Params) (bool, error) {
	return sync(ctx, mgr, logger, p, false)
}

func EnsureInstalled(ctx context.Context, mgr Manager, logger *slog.Logger, p Params) error {
	_, err := sync(ctx, mgr, logger, p, true)
	return err
}

func sync(ctx context.Context, mgr Manager, logger *slog.Logger, p Params, createIfMissing bool) (bool, error) {

	absent := false
	onDisk, err := mgr.ReadUnit(ctx, UnitName)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if !createIfMissing {
			logger.Debug("no unit file on disk; skipping unit reconcile", "unit", UnitName)
			return false, nil
		}
		absent = true
	case err != nil:
		return false, fmt.Errorf("read unit %s: %w", UnitName, err)
	}

	if ver, err := mgr.Version(ctx); err != nil {
		logger.Warn("systemd version probe failed; rendering RestrictRealtime=false as a precaution", "error", err)
		p.RestrictRealtime = false
	} else {
		p.RestrictRealtime = ver >= restrictRealtimeMinVersion
	}

	rendered, err := Render(p)
	if err != nil {
		return false, err
	}

	if !absent && onDisk == rendered {

		pending, nrErr := mgr.NeedsReload(ctx, UnitName)
		if nrErr != nil {
			logger.Warn("could not check for a pending daemon-reload; continuing", "unit", UnitName, "error", nrErr)
			return false, nil
		}
		if pending {
			logger.Warn("unit file is current but systemd's loaded config is stale (an earlier daemon-reload failed?); completing the reload", "unit", UnitName)
			if err := mgr.DaemonReload(ctx); err != nil {
				return false, fmt.Errorf("retry daemon-reload for %s: %w", UnitName, err)
			}
		}
		return false, nil
	}

	if err := mgr.WriteUnit(ctx, UnitName, rendered); err != nil {
		return false, fmt.Errorf("write unit %s: %w", UnitName, err)
	}
	if err := mgr.DaemonReload(ctx); err != nil {
		return true, fmt.Errorf("daemon-reload after writing %s (unit IS updated on disk): %w", UnitName, err)
	}
	return true, nil
}
