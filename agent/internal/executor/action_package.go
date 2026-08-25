package executor

import (
	"context"
	"fmt"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/sdk/pkg"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

func (e *Executor) executePackage(ctx context.Context, params *pb.PackageParams, state pb.DesiredState) (*pb.CommandOutput, bool, error) {
	e.ensureDeps()
	if params == nil {
		return nil, false, fmt.Errorf("package params required")
	}

	mgr := e.pkgManagerForCtx(ctx)
	if mgr == nil {
		return nil, false, fmt.Errorf("no supported package manager found")
	}
	pkgName := e.getPackageNameForManager(params)
	if pkgName == "" {
		return nil, false, notApplicable("no package name configured for this package manager")
	}
	switch state {
	case pb.DesiredState_DESIRED_STATE_PRESENT:
		return e.ensurePackagePresent(ctx, mgr, params, pkgName)
	case pb.DesiredState_DESIRED_STATE_ABSENT:
		return e.ensurePackageAbsent(ctx, mgr, params, pkgName)
	default:
		return nil, false, fmt.Errorf("unknown desired state: %v", state)
	}
}

func (e *Executor) ensurePackagePresent(ctx context.Context, mgr pkg.Manager, params *pb.PackageParams, pkgName string) (*pb.CommandOutput, bool, error) {

	isInstalled, err := mgr.IsInstalled(ctx, pkgName)
	if err != nil {
		return nil, false, fmt.Errorf("probe package state for %s: %w", pkgName, err)
	}
	if isInstalled {
		if out, changed, err := e.checkPackageVersionAndPin(ctx, mgr, params, pkgName); out != nil {
			return out, changed, err
		}
	}

	if out, err := e.requireWritableFS(ctx); err != nil {
		return out, false, err
	}
	if _, updateErr := mgr.Update(ctx); updateErr != nil {
		e.logger.Warn("package index update failed, continuing with install", "error", updateErr)
	}

	spec := pkg.InstallSpec{Name: pkgName, Version: params.Version}
	options := pkg.InstallOptions{AllowDowngrade: params.AllowDowngrade}
	result, err := mgr.Install(ctx, options, spec)

	if err == nil && params.Pin {
		if _, pinErr := e.pinPackage(ctx, mgr, pkgName); pinErr != nil {

			result.Stderr += fmt.Sprintf("\nfailed to pin package: %v", pinErr)
			err = fmt.Errorf("install succeeded but pin failed: %w", pinErr)
		}
	}
	return packageResult(result, err)
}

func (e *Executor) checkPackageVersionAndPin(ctx context.Context, mgr pkg.Manager, params *pb.PackageParams, pkgName string) (*pb.CommandOutput, bool, error) {
	versionStr := ""
	if params.Version != "" {
		installedVersion, err := mgr.InstalledVersion(ctx, pkgName)
		if err != nil {

			return &pb.CommandOutput{ExitCode: 1, Stderr: fmt.Sprintf("read installed version for %s: %v", pkgName, err)},
				false, fmt.Errorf("read installed version for %s: %w", pkgName, err)
		}
		if installedVersion != params.Version {
			return nil, false, nil
		}
		versionStr = " version " + params.Version
	}
	if params.Pin {
		changed, pinErr := e.ensurePackagePinned(ctx, mgr, pkgName)
		if pinErr != nil {
			return &pb.CommandOutput{
				ExitCode: 1,
				Stderr:   fmt.Sprintf("failed to pin package: %v", pinErr),
			}, false, pinErr
		}
		msg := fmt.Sprintf("package %s%s is already installed and pinned", pkgName, versionStr)
		if changed {
			msg = fmt.Sprintf("package %s%s was already installed, pinned", pkgName, versionStr)
		}
		return &pb.CommandOutput{ExitCode: 0, Stdout: msg}, changed, nil
	}
	return &pb.CommandOutput{
		ExitCode: 0,
		Stdout:   fmt.Sprintf("package %s%s is already installed", pkgName, versionStr),
	}, false, nil
}

func (e *Executor) ensurePackageAbsent(ctx context.Context, mgr pkg.Manager, _ *pb.PackageParams, pkgName string) (*pb.CommandOutput, bool, error) {

	isInstalled, err := mgr.IsInstalled(ctx, pkgName)
	if err != nil {
		return nil, false, fmt.Errorf("probe package state for %s: %w", pkgName, err)
	}
	if !isInstalled {
		return &pb.CommandOutput{
			ExitCode: 0,
			Stdout:   fmt.Sprintf("package %s is already not installed", pkgName),
		}, false, nil
	}
	if out, err := e.requireWritableFS(ctx); err != nil {
		return out, false, err
	}
	if _, err := e.ensurePackageUnpinned(ctx, mgr, pkgName); err != nil {
		e.logger.Warn("ensurePackageAbsent: failed to unpin package before removal",
			"package", pkgName, "error", err)
	}
	result, err := mgr.Remove(ctx, pkg.RemoveOptions{}, pkgName)
	return packageResult(result, err)
}

func packageResult(result sysexec.Result, err error) (*pb.CommandOutput, bool, error) {
	out := &pb.CommandOutput{
		ExitCode: int32(result.ExitCode),
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
	}
	if err != nil {
		if result.ExitCode == 0 {

			out.ExitCode = 1
			if out.Stderr == "" {
				out.Stderr = err.Error()
			}
		}
		return out, false, err
	}
	return out, true, nil
}

func (e *Executor) getPackageNameForManager(params *pb.PackageParams) string {

	switch e.pkgBackend {
	case pkg.Apt:
		if params.AptName != "" {
			return params.AptName
		}
	case pkg.Dnf:
		if params.DnfName != "" {
			return params.DnfName
		}
	case pkg.Dnf5:
		if params.DnfName != "" {
			return params.DnfName
		}
	case pkg.Pacman:
		if params.PacmanName != "" {
			return params.PacmanName
		}
	case pkg.Zypper:
		if params.ZypperName != "" {
			return params.ZypperName
		}
	}

	return params.Name
}

func (e *Executor) isPackagePinned(ctx context.Context, mgr pkg.Manager, pkgName string) (bool, error) {
	if mgr == nil {
		return false, fmt.Errorf("no package manager available")
	}
	return mgr.IsPinned(ctx, pkgName)
}

func (e *Executor) pinPackage(ctx context.Context, mgr pkg.Manager, pkgName string) (bool, error) {
	if mgr == nil {
		return false, fmt.Errorf("no package manager available")
	}

	isPinned, err := mgr.IsPinned(ctx, pkgName)
	if err != nil {
		return false, fmt.Errorf("check pin status: %w", err)
	}
	if isPinned {
		return false, nil
	}

	if _, err = mgr.Pin(ctx, pkgName); err != nil {
		return false, fmt.Errorf("pin package: %w", err)
	}
	return true, nil
}

func (e *Executor) unpinPackage(ctx context.Context, mgr pkg.Manager, pkgName string) (bool, error) {
	if mgr == nil {
		return false, fmt.Errorf("no package manager available")
	}

	isPinned, err := mgr.IsPinned(ctx, pkgName)
	if err != nil {
		return false, fmt.Errorf("check pin status: %w", err)
	}
	if !isPinned {
		return false, nil
	}

	if _, err = mgr.Unpin(ctx, pkgName); err != nil {
		return false, fmt.Errorf("unpin package: %w", err)
	}
	return true, nil
}

func (e *Executor) ensurePackagePinned(ctx context.Context, mgr pkg.Manager, pkgName string) (bool, error) {

	isPinned, err := e.isPackagePinned(ctx, mgr, pkgName)
	if err != nil {
		return false, fmt.Errorf("check pin state for %s: %w", pkgName, err)
	}
	if isPinned {
		return false, nil
	}

	if !e.repairFilesystem(ctx) {
		return false, errReadOnlyFS
	}

	return e.pinPackage(ctx, mgr, pkgName)
}

func (e *Executor) ensurePackageUnpinned(ctx context.Context, mgr pkg.Manager, pkgName string) (bool, error) {
	return e.unpinPackage(ctx, mgr, pkgName)
}
