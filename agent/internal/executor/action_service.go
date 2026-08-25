package executor

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysservice "github.com/manchtools/cadestro/sdk/sys/service"
)

func (e *Executor) executeService(ctx context.Context, params *pb.ServiceParams) (*pb.CommandOutput, bool, error) {
	e.ensureDeps()
	if params == nil {
		return nil, false, fmt.Errorf("service params required")
	}

	if err := sysservice.ValidateUnitName(params.UnitName); err != nil {
		return &pb.CommandOutput{
			ExitCode: 1,
			Stderr:   err.Error() + "\n",
		}, false, err
	}

	if params.UnitName == "cadestrod.service" || params.UnitName == "cadestrod" {
		return &pb.CommandOutput{
			ExitCode: 1,
			Stderr:   "refusing to manage the cadestrod service\n",
		}, false, fmt.Errorf("cannot manage protected service: cadestrod")
	}

	var output strings.Builder
	changed := false

	if params.UnitContent != "" {

		unitPath := filepath.Join("/etc/systemd/system", params.UnitName)
		needsUpdate := true
		if existingContent, err := e.readFileWithSudo(ctx, unitPath); err == nil {
			existingHash := sha256.Sum256([]byte(existingContent))
			desiredHash := sha256.Sum256([]byte(params.UnitContent))
			if existingHash == desiredHash {
				needsUpdate = false
				output.WriteString(fmt.Sprintf("unit file %s is already up to date\n", params.UnitName))
			}
		}

		if needsUpdate {

			if out, err := e.requireWritableFS(ctx); err != nil {
				return out, false, err
			}

			if err := e.deps.service.WriteUnit(ctx, params.UnitName, params.UnitContent); err != nil {
				return nil, false, fmt.Errorf("write unit %s: %w", params.UnitName, err)
			}
			output.WriteString(fmt.Sprintf("updated unit file %s\n", params.UnitName))
			changed = true

			if err := e.deps.service.DaemonReload(ctx); err != nil {
				return nil, changed, fmt.Errorf("daemon-reload failed: %w", err)
			}
			output.WriteString("reloaded service manager\n")
		}
	}

	isEnabled := e.isUnitEnabled(ctx, params.UnitName)
	if params.Enable && !isEnabled {

		if e.isUnitMasked(ctx, params.UnitName) {
			return nil, changed, fmt.Errorf("enable: unit %s is masked (run 'systemctl unmask %s' first)", params.UnitName, params.UnitName)
		}
		if err := e.deps.service.Enable(ctx, params.UnitName); err != nil {
			return nil, changed, fmt.Errorf("enable: %w", err)
		}
		output.WriteString("enabled unit\n")
		changed = true
	} else if !params.Enable && isEnabled {
		if err := e.deps.service.Disable(ctx, params.UnitName); err != nil {

			return nil, false, fmt.Errorf("disable %s: %w", params.UnitName, err)
		}
		output.WriteString("disabled unit\n")
		changed = true
	}

	isActive := e.isUnitActive(ctx, params.UnitName)
	switch params.DesiredState {
	case pb.ServiceUnitState_SERVICE_UNIT_STATE_STARTED:
		if !isActive {
			if err := e.deps.service.Start(ctx, params.UnitName); err != nil {
				return nil, changed, fmt.Errorf("start: %w", err)
			}
			output.WriteString("started unit\n")
			changed = true
		} else {
			output.WriteString("unit is already running\n")
		}
	case pb.ServiceUnitState_SERVICE_UNIT_STATE_STOPPED:
		if isActive {
			if err := e.deps.service.Stop(ctx, params.UnitName); err != nil {
				return nil, changed, fmt.Errorf("stop: %w", err)
			}
			output.WriteString("stopped unit\n")
			changed = true
		} else {
			output.WriteString("unit is already stopped\n")
		}
	case pb.ServiceUnitState_SERVICE_UNIT_STATE_RESTARTED:

		if err := e.deps.service.Restart(ctx, params.UnitName); err != nil {
			return nil, changed, fmt.Errorf("restart: %w", err)
		}
		output.WriteString("restarted unit\n")
		changed = true
	default:
		if !changed {
			output.WriteString("unit is already in desired state\n")
		}
	}

	return &pb.CommandOutput{ExitCode: 0, Stdout: output.String()}, changed, nil
}

func (e *Executor) isUnitEnabled(ctx context.Context, unitName string) bool {
	enabled, err := e.deps.service.IsEnabled(ctx, unitName)
	if err != nil {
		e.logger.Debug("sysservice.IsEnabled failed; treating as not enabled",
			"unit", unitName, "error", err)
	}
	return enabled
}

func (e *Executor) isUnitMasked(ctx context.Context, unitName string) bool {
	masked, err := e.deps.service.IsMasked(ctx, unitName)
	if err != nil {
		e.logger.Warn("sysservice.IsMasked failed; treating as not masked",
			"unit", unitName, "error", err)
	}
	return masked
}

func (e *Executor) isUnitActive(ctx context.Context, unitName string) bool {
	active, err := e.deps.service.IsActive(ctx, unitName)
	if err != nil {
		e.logger.Debug("sysservice.IsActive failed; treating as not active",
			"unit", unitName, "error", err)
	}
	return active
}
