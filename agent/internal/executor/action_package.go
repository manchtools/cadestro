package executor

import (
	"context"
	"fmt"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/sdk/pkg"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

func (e *Executor) executePackage(ctx context.Context, params *pb.PackageActionParams, state pb.DesiredState) (*pb.CommandOutput, error) {
	if params == nil {
		return nil, fmt.Errorf("package params required")
	}
	if e.pkgManager == nil {
		return nil, fmt.Errorf("no supported package manager found")
	}
	installed, err := e.pkgManager.IsInstalled(ctx, params.GetName())
	if err != nil {
		return nil, fmt.Errorf("probe package state: %w", err)
	}
	switch state {
	case pb.DesiredState_DESIRED_STATE_PRESENT:
		if installed && params.GetVersion() == "" {
			return &pb.CommandOutput{Stdout: fmt.Sprintf("package %s is already installed", params.GetName())}, nil
		}
		if installed {
			version, err := e.pkgManager.InstalledVersion(ctx, params.GetName())
			if err != nil {
				return nil, fmt.Errorf("read installed package version: %w", err)
			}
			if version == params.GetVersion() {
				return &pb.CommandOutput{Stdout: fmt.Sprintf("package %s version %s is already installed", params.GetName(), version)}, nil
			}
		}
		if _, err := e.pkgManager.Update(ctx); err != nil {
			return nil, fmt.Errorf("update package index: %w", err)
		}
		run, err := e.pkgManager.Install(ctx, pkg.InstallOptions{}, pkg.InstallSpec{Name: params.GetName(), Version: params.GetVersion()})
		return commandResult(run, err)
	case pb.DesiredState_DESIRED_STATE_ABSENT:
		if !installed {
			return &pb.CommandOutput{Stdout: fmt.Sprintf("package %s is already absent", params.GetName())}, nil
		}
		run, err := e.pkgManager.Remove(ctx, pkg.RemoveOptions{}, params.GetName())
		return commandResult(run, err)
	default:
		return nil, fmt.Errorf("unsupported package desired state: %s", state)
	}
}

func commandResult(run sysexec.Result, err error) (*pb.CommandOutput, error) {
	output := &pb.CommandOutput{ExitCode: int32(run.ExitCode), Stdout: run.Stdout, Stderr: run.Stderr}
	if err != nil {
		return output, err
	}
	if run.ExitCode != 0 {
		return output, fmt.Errorf("package manager exited with status %d", run.ExitCode)
	}
	return output, nil
}
