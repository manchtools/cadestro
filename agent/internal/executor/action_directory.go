package executor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysfs "github.com/manchtools/cadestro/sdk/sys/fs"
)

func (e *Executor) executeDirectory(ctx context.Context, params *pb.DirectoryParams, state pb.DesiredState) (*pb.CommandOutput, bool, error) {
	e.ensureDeps()
	if params == nil {
		return nil, false, fmt.Errorf("directory params required")
	}

	if params.Path == "" {
		return nil, false, fmt.Errorf("directory path is required")
	}

	cleanPath, err := sysfs.ResolveAndValidatePath(params.Path)
	if err != nil {
		return nil, false, fmt.Errorf("invalid path: %w", err)
	}

	switch state {
	case pb.DesiredState_DESIRED_STATE_PRESENT:

		if isProtectedPath(cleanPath) || isProtectedPath(filepath.Clean(params.Path)) ||
			sysfs.IsUnderProtectedPrefix(cleanPath) || sysfs.IsUnderProtectedPrefix(filepath.Clean(params.Path)) {
			return nil, false, fmt.Errorf("refusing to manage protected system path: %s (resolved from %s)", cleanPath, params.Path)
		}

		if e.directoryMatchesDesired(ctx, cleanPath, params) {
			return &pb.CommandOutput{
				ExitCode: 0,
				Stdout:   fmt.Sprintf("directory %s is already in desired state", cleanPath),
			}, false, nil
		}

		if out, err := e.requireWritableFS(ctx); err != nil {
			return out, false, err
		}

		if err := e.createDirectoryWithPermissions(ctx, cleanPath, params.Mode, params.Owner, params.Group, params.Recursive); err != nil {
			return nil, false, err
		}

		return &pb.CommandOutput{
			ExitCode: 0,
			Stdout:   fmt.Sprintf("created directory %s", cleanPath),
		}, true, nil

	case pb.DesiredState_DESIRED_STATE_ABSENT:

		if isProtectedPath(cleanPath) || isProtectedPath(filepath.Clean(params.Path)) ||
			sysfs.IsUnderProtectedPrefix(cleanPath) || sysfs.IsUnderProtectedPrefix(filepath.Clean(params.Path)) {
			return nil, false, fmt.Errorf("refusing to delete protected system path: %s (resolved from %s)", cleanPath, params.Path)
		}

		if !e.fileExistsWithSudo(ctx, cleanPath) {
			return &pb.CommandOutput{
				ExitCode: 0,
				Stdout:   fmt.Sprintf("directory %s does not exist, nothing to remove", cleanPath),
			}, false, nil
		}

		if out, err := e.requireWritableFS(ctx); err != nil {
			return out, false, err
		}

		if err := e.removeDirectory(ctx, cleanPath); err != nil {
			return nil, false, fmt.Errorf("remove directory: %w", err)
		}
		return &pb.CommandOutput{
			ExitCode: 0,
			Stdout:   fmt.Sprintf("removed directory %s", cleanPath),
		}, true, nil
	}

	return nil, false, fmt.Errorf("unknown desired state: %v", state)
}

func (e *Executor) directoryMatchesDesired(ctx context.Context, path string, params *pb.DirectoryParams) bool {

	mode, err := statFile(ctx, path)
	if err != nil {
		return false
	}

	if !mode.IsDir() {
		return false
	}

	if params.Mode != "" {
		desiredMode, err := strconv.ParseUint(params.Mode, 8, 32)
		if err != nil {
			return false
		}
		if mode.Perm() != os.FileMode(desiredMode).Perm() {
			return false
		}
	}

	if params.Owner != "" || params.Group != "" {
		currentOwner, currentGroup := getFileOwnership(path)
		if currentOwner == "" && currentGroup == "" {
			return false
		}
		if params.Owner != "" && currentOwner != params.Owner {
			return false
		}
		if params.Group != "" && currentGroup != params.Group {
			return false
		}
	}

	return true
}
