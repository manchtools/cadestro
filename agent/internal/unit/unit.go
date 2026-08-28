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

//go:embed cadestrod.service.tmpl
var unitTemplate string

var tmpl = template.Must(template.New(UnitName).Parse(unitTemplate))

type Manager interface {
	ReadUnit(ctx context.Context, unit string) (string, error)
	WriteUnit(ctx context.Context, unit, content string) error
	DaemonReload(ctx context.Context) error
	NeedsReload(ctx context.Context, unit string) (bool, error)
}

type Params struct {
	BinaryPath string
	DataDir    string
}

func Render(params Params) (string, error) {
	if err := validateUnitPath("BinaryPath", params.BinaryPath); err != nil {
		return "", err
	}
	if err := validateUnitPath("DataDir", params.DataDir); err != nil {
		return "", err
	}
	var output bytes.Buffer
	if err := tmpl.Execute(&output, params); err != nil {
		return "", fmt.Errorf("render unit: %w", err)
	}
	return output.String(), nil
}

func validateUnitPath(field, value string) error {
	if !strings.HasPrefix(value, "/") {
		return fmt.Errorf("%s must be an absolute path", field)
	}
	if strings.ContainsAny(value, " \t\r\n\"'\\%$") {
		return fmt.Errorf("%s contains a character interpreted by systemd", field)
	}
	return nil
}

func Reconcile(ctx context.Context, manager Manager, logger *slog.Logger, params Params) (bool, error) {
	return sync(ctx, manager, logger, params, false)
}

func EnsureInstalled(ctx context.Context, manager Manager, logger *slog.Logger, params Params) error {
	_, err := sync(ctx, manager, logger, params, true)
	return err
}

func sync(ctx context.Context, manager Manager, logger *slog.Logger, params Params, create bool) (bool, error) {
	current, err := manager.ReadUnit(ctx, UnitName)
	if errors.Is(err, fs.ErrNotExist) && !create {
		return false, nil
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("read unit: %w", err)
	}
	rendered, err := Render(params)
	if err != nil {
		return false, err
	}
	if current == rendered {
		pending, err := manager.NeedsReload(ctx, UnitName)
		if err != nil {
			logger.Warn("check unit reload state", "error", err)
			return false, nil
		}
		if pending {
			if err := manager.DaemonReload(ctx); err != nil {
				return false, fmt.Errorf("reload systemd: %w", err)
			}
		}
		return false, nil
	}
	if err := manager.WriteUnit(ctx, UnitName, rendered); err != nil {
		return false, fmt.Errorf("write unit: %w", err)
	}
	if err := manager.DaemonReload(ctx); err != nil {
		return true, fmt.Errorf("reload systemd: %w", err)
	}
	return true, nil
}
