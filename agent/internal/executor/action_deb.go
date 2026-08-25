package executor

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/sdk/pkg"
)

var validDebPkgName = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]*$`)

func (e *Executor) executeDeb(ctx context.Context, params *pb.AppInstallParams, state pb.DesiredState) (*pb.CommandOutput, bool, error) {
	e.ensureDeps()
	if params == nil {
		return nil, false, fmt.Errorf("app params required")
	}

	if state == pb.DesiredState_DESIRED_STATE_PRESENT {
		if err := requireVerifiedArtifact(params.Url, params.ChecksumSha256); err != nil {
			return nil, false, err
		}
	}

	if !slices.Contains(pkg.Detect(), pkg.Apt) {
		return nil, false, notApplicable("no supported .deb package manager available on this system")
	}

	mgr := e.pkgManagerForCtx(ctx)
	if mgr == nil {
		return nil, false, fmt.Errorf("no usable package manager for .deb (context expired or none detected)")
	}

	switch state {
	case pb.DesiredState_DESIRED_STATE_PRESENT:

		if out, err := e.requireWritableFS(ctx); err != nil {
			return out, false, err
		}

		tmpFile, err := os.CreateTemp("", "*.deb")
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
				Stdout:   fmt.Sprintf("deb package %s is already installed", pkgName),
			}, false, nil
		}

		return packageResult(mgr.InstallLocal(ctx, tmpFile.Name(), pkg.InstallLocalOptions{}))

	case pb.DesiredState_DESIRED_STATE_ABSENT:

		pkgName, err := e.debAbsentPackageName(ctx, mgr, params)
		if err != nil {
			return nil, false, err
		}
		if installed, err := mgr.IsInstalled(ctx, pkgName); err != nil {
			return nil, false, fmt.Errorf("check %s installed: %w", pkgName, err)
		} else if !installed {
			return &pb.CommandOutput{
				ExitCode: 0,
				Stdout:   fmt.Sprintf("deb package %s is already not installed", pkgName),
			}, false, nil
		}
		if out, err := e.requireWritableFS(ctx); err != nil {
			return out, false, err
		}

		return packageResult(mgr.Remove(ctx, pkg.RemoveOptions{}, pkgName))
	}

	return nil, false, fmt.Errorf("unknown desired state: %v", state)
}

func (e *Executor) debAbsentPackageName(ctx context.Context, mgr pkg.Manager, params *pb.AppInstallParams) (string, error) {

	if requireVerifiedArtifact(params.Url, params.ChecksumSha256) != nil {
		return debPackageNameFromURL(params.Url)
	}

	tmpFile, err := os.CreateTemp("", "*.deb")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	_ = tmpFile.Close()

	if dlErr := fetchArtifact(ctx, params.Url, tmpFile.Name(), params.ChecksumSha256, "", redirectForArtifact(params.ChecksumSha256)); dlErr == nil {

		info, nameErr := mgr.LocalPackageInfo(ctx, tmpFile.Name())
		if nameErr != nil {
			return "", fmt.Errorf("download succeeded but could not read the package name: %w", nameErr)
		}
		return info.Name, nil
	}

	return debPackageNameFromURL(params.Url)
}

func debPackageNameFromURL(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid deb url %q: %w", rawURL, err)
	}
	base := path.Base(parsed.Path)
	if base == "" || base == "/" || base == "." {
		return "", fmt.Errorf("deb url %q has no filename segment", rawURL)
	}
	if !strings.HasSuffix(base, ".deb") {
		return "", fmt.Errorf("deb url filename %q does not end in .deb", base)
	}

	stem := strings.TrimSuffix(base, ".deb")
	name, _, ok := strings.Cut(stem, "_")
	if !ok || name == "" {
		return "", fmt.Errorf("deb url filename %q is not in name_version_arch.deb form", base)
	}
	if !validDebPkgName.MatchString(name) {
		return "", fmt.Errorf("invalid debian package name %q derived from %s", name, rawURL)
	}
	return name, nil
}
