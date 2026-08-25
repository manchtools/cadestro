package executor

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	sdk "github.com/manchtools/cadestro/contract"
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/sdk/pkg"
)

func (e *Executor) executeRpm(ctx context.Context, params *pb.AppInstallParams, state pb.DesiredState) (*pb.CommandOutput, bool, error) {
	e.ensureDeps()
	if params == nil {
		return nil, false, fmt.Errorf("app params required")
	}

	if err := requireVerifiedArtifact(params.Url, params.ChecksumSha256); err != nil {
		return nil, false, err
	}

	if detected := pkg.Detect(); !slices.Contains(detected, pkg.Dnf) && !slices.Contains(detected, pkg.Dnf5) && !slices.Contains(detected, pkg.Zypper) {
		return nil, false, notApplicable("no supported .rpm package manager available on this system")
	}

	mgr := e.pkgManagerForCtx(ctx)
	if mgr == nil {
		return nil, false, fmt.Errorf("no usable package manager for .rpm (context expired or none detected)")
	}

	switch state {
	case pb.DesiredState_DESIRED_STATE_PRESENT:

		if out, err := e.requireWritableFS(ctx); err != nil {
			return out, false, err
		}

		tmpFile, err := os.CreateTemp("", "*.rpm")
		if err != nil {
			return nil, false, fmt.Errorf("create temp file: %w", err)
		}
		defer os.Remove(tmpFile.Name())
		_ = tmpFile.Close()

		if err := fetchArtifact(ctx, params.Url, tmpFile.Name(), params.ChecksumSha256, "", redirectForArtifact(params.ChecksumSha256)); err != nil {
			return nil, false, fmt.Errorf("download: %w", err)
		}

		info, err := mgr.LocalPackageInfo(ctx, tmpFile.Name())
		if err != nil {
			return nil, false, err
		}
		pkgName := info.Name

		if installed, err := mgr.IsInstalled(ctx, pkgName); err != nil {
			return nil, false, fmt.Errorf("check %s installed: %w", pkgName, err)
		} else if installed {
			return &pb.CommandOutput{
				ExitCode: 0,
				Stdout:   fmt.Sprintf("rpm package %s is already installed", pkgName),
			}, false, nil
		}

		return packageResult(mgr.InstallLocal(ctx, tmpFile.Name(), pkg.InstallLocalOptions{AllowUnsigned: true}))

	case pb.DesiredState_DESIRED_STATE_ABSENT:

		if out, err := e.requireWritableFS(ctx); err != nil {
			return out, false, err
		}
		tmpFile, err := os.CreateTemp("", "*.rpm")
		if err != nil {
			return nil, false, fmt.Errorf("create temp file: %w", err)
		}
		defer os.Remove(tmpFile.Name())
		_ = tmpFile.Close()
		if err := fetchArtifact(ctx, params.Url, tmpFile.Name(), params.ChecksumSha256, "", redirectForArtifact(params.ChecksumSha256)); err != nil {

			return nil, false, fmt.Errorf("cannot determine rpm package to remove: artifact %s is unreachable (%w); re-point the action at a reachable URL or remove the package manually", params.Url, err)
		}
		info, err := mgr.LocalPackageInfo(ctx, tmpFile.Name())
		if err != nil {
			return nil, false, err
		}
		pkgName := info.Name
		if installed, err := mgr.IsInstalled(ctx, pkgName); err != nil {
			return nil, false, fmt.Errorf("check %s installed: %w", pkgName, err)
		} else if !installed {
			return &pb.CommandOutput{
				ExitCode: 0,
				Stdout:   fmt.Sprintf("rpm package %s is already not installed", pkgName),
			}, false, nil
		}

		return packageResult(mgr.Remove(ctx, pkg.RemoveOptions{}, pkgName))
	}

	return nil, false, fmt.Errorf("unknown desired state: %v", state)
}

func requireVerifiedArtifact(rawURL, checksum string) error {
	if err := sdk.ValidateHTTPSURL(rawURL); err != nil {
		return fmt.Errorf("artifact rejected: %w", err)
	}
	checksum = strings.TrimSpace(checksum)
	if checksum == "" {
		return fmt.Errorf("artifact rejected: checksum_sha256 is required (refusing to install an unverified binary)")
	}

	if !isHex64(checksum) {
		return fmt.Errorf("artifact rejected: checksum_sha256 must be 64 hexadecimal characters")
	}
	return nil
}

func isHex64(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}
