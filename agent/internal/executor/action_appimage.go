package executor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	sdk "github.com/manchtools/cadestro/contract"
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysfs "github.com/manchtools/cadestro/sdk/sys/fs"
)

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (e *Executor) executeAppImage(ctx context.Context, params *pb.AppInstallParams, state pb.DesiredState) (*pb.CommandOutput, bool, error) {
	e.ensureDeps()
	if params == nil {
		return nil, false, fmt.Errorf("app params required")
	}

	installPath := params.InstallPath
	if installPath == "" {
		installPath = "/opt/appimages"
	}

	trimmedURL := strings.TrimSpace(params.Url)
	if err := sdk.ValidateHTTPSURL(trimmedURL); err != nil {
		return nil, false, fmt.Errorf("invalid appimage URL: %w", err)
	}
	parsedURL, err := url.Parse(trimmedURL)
	if err != nil {
		return nil, false, fmt.Errorf("invalid appimage URL %q: %w", params.Url, err)
	}
	filename := filepath.Base(parsedURL.Path)

	if filename == "." || filename == ".." || filename == "/" || filename == "" || strings.ContainsAny(filename, `/\`) {
		return nil, false, fmt.Errorf("appimage URL %q does not yield a usable filename", params.Url)
	}

	if state == pb.DesiredState_DESIRED_STATE_PRESENT {
		if err := requireVerifiedArtifact(params.Url, params.ChecksumSha256); err != nil {
			return nil, false, err
		}
	}

	fullPath := filepath.Join(installPath, filename)

	resolvedPath, err := sysfs.ResolveAndValidatePath(fullPath)
	if err != nil {
		return nil, false, fmt.Errorf("invalid path: %w", err)
	}

	switch state {
	case pb.DesiredState_DESIRED_STATE_PRESENT:

		if params.ChecksumSha256 != "" {
			if actualHash, hashErr := sha256File(resolvedPath); hashErr == nil {

				if strings.EqualFold(actualHash, strings.TrimSpace(params.ChecksumSha256)) {
					return &pb.CommandOutput{
						ExitCode: 0,
						Stdout:   fmt.Sprintf("appimage %s already installed with correct checksum", filename),
					}, false, nil
				}
			}
		} else if ok, _ := e.deps.fs.Exists(ctx, resolvedPath); ok {

			return &pb.CommandOutput{
				ExitCode: 0,
				Stdout:   fmt.Sprintf("appimage %s already installed", filename),
			}, false, nil
		}

		if out, err := e.requireWritableFS(ctx); err != nil {
			return out, false, err
		}

		if err := e.createDirectory(ctx, filepath.Dir(resolvedPath), true); err != nil {
			return nil, false, fmt.Errorf("create directory: %w", err)
		}
		if err := fetchArtifact(ctx, params.Url, resolvedPath, params.ChecksumSha256, "0755", redirectForArtifact(params.ChecksumSha256)); err != nil {
			return nil, false, fmt.Errorf("download: %w", err)
		}

		return &pb.CommandOutput{
			ExitCode: 0,
			Stdout:   fmt.Sprintf("installed %s to %s", filename, resolvedPath),
		}, true, nil

	case pb.DesiredState_DESIRED_STATE_ABSENT:

		if ok, existErr := e.deps.fs.Exists(ctx, resolvedPath); existErr == nil && !ok {
			return &pb.CommandOutput{
				ExitCode: 0,
				Stdout:   fmt.Sprintf("appimage %s already not present", filename),
			}, false, nil
		}

		if out, err := e.requireWritableFS(ctx); err != nil {
			return out, false, err
		}

		if err := e.removeFileStrict(ctx, resolvedPath); err != nil {
			return nil, false, fmt.Errorf("remove: %w", err)
		}
		return &pb.CommandOutput{
			ExitCode: 0,
			Stdout:   fmt.Sprintf("removed %s", resolvedPath),
		}, true, nil
	}

	return nil, false, fmt.Errorf("unknown desired state: %v", state)
}
