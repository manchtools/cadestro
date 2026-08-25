package executor

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysfs "github.com/manchtools/cadestro/sdk/sys/fs"
)

func (e *Executor) executeFile(ctx context.Context, params *pb.FileParams, state pb.DesiredState) (*pb.CommandOutput, bool, error) {
	e.ensureDeps()
	if params == nil {
		return nil, false, fmt.Errorf("file params required")
	}
	if len(params.Content) > maxFileContentSize {
		return nil, false, fmt.Errorf("file content exceeds maximum size (%d bytes)", maxFileContentSize)
	}

	resolvedPath, err := sysfs.ResolveAndValidatePath(params.Path)
	if err != nil {
		return nil, false, fmt.Errorf("invalid path: %w", err)
	}

	switch state {
	case pb.DesiredState_DESIRED_STATE_PRESENT:

		if e.fileMatchesDesired(ctx, resolvedPath, params) {
			return &pb.CommandOutput{
				ExitCode: 0,
				Stdout:   fmt.Sprintf("file %s is already in desired state", resolvedPath),
			}, false, nil
		}

		if isCriticalFile(resolvedPath) || isCriticalFile(filepath.Clean(params.Path)) {
			return nil, false, fmt.Errorf("refusing to overwrite critical system file: %s (resolved from %s)", resolvedPath, params.Path)
		}

		if out, err := e.requireWritableFS(ctx); err != nil {
			return out, false, err
		}

		parentDir := filepath.Dir(resolvedPath)
		if err := e.createDirectory(ctx, parentDir, true); err != nil {
			return nil, false, fmt.Errorf("create directory %s: %w", parentDir, err)
		}

		var finalContent string
		actionVerb := "created"
		if params.ManagedBlock {

			existing, err := e.readFileWithSudo(ctx, resolvedPath)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, false, fmt.Errorf("read existing file: %w", err)
			}

			if len(existing) > 0 && !strings.HasSuffix(existing, "\n") {
				existing += "\n"
			}
			finalContent = existing + params.Content
			actionVerb = "added block to"
		} else {
			finalContent = params.Content
		}

		if err := e.atomicWriteFile(ctx, resolvedPath, finalContent, params.Mode, params.Owner, params.Group); err != nil {
			return nil, false, err
		}

		return &pb.CommandOutput{
			ExitCode: 0,
			Stdout:   fmt.Sprintf("%s %s", actionVerb, resolvedPath),
		}, true, nil

	case pb.DesiredState_DESIRED_STATE_ABSENT:

		if !e.fileExistsWithSudo(ctx, resolvedPath) {
			return &pb.CommandOutput{
				ExitCode: 0,
				Stdout:   fmt.Sprintf("file %s does not exist, nothing to remove", resolvedPath),
			}, false, nil
		}

		if isProtectedPath(resolvedPath) || isProtectedPath(filepath.Clean(params.Path)) {
			return nil, false, fmt.Errorf("refusing to remove protected system path: %s (resolved from %s)", resolvedPath, params.Path)
		}

		if out, err := e.requireWritableFS(ctx); err != nil {
			return out, false, err
		}

		if params.ManagedBlock {

			existingContent, err := e.readFileWithSudo(ctx, resolvedPath)
			if err != nil {
				return nil, false, fmt.Errorf("read file: %w", err)
			}

			if !strings.Contains(existingContent, params.Content) {
				return &pb.CommandOutput{
					ExitCode: 0,
					Stdout:   fmt.Sprintf("content not found in %s, nothing to remove", resolvedPath),
				}, false, nil
			}

			newContent := strings.Replace(existingContent, params.Content, "", 1)

			for strings.Contains(newContent, "\n\n\n") {
				newContent = strings.ReplaceAll(newContent, "\n\n\n", "\n\n")
			}

			if err := e.atomicWriteFile(ctx, resolvedPath, newContent, params.Mode, params.Owner, params.Group); err != nil {
				return nil, false, err
			}

			return &pb.CommandOutput{
				ExitCode: 0,
				Stdout:   fmt.Sprintf("removed content block from %s", resolvedPath),
			}, true, nil
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

func (e *Executor) fileMatchesDesired(ctx context.Context, path string, params *pb.FileParams) bool {

	mode, err := statFile(ctx, path)
	if err != nil {
		return false
	}

	if !mode.IsRegular() {
		return false
	}

	content, err := e.readFileWithSudo(ctx, path)
	if err != nil {
		return false
	}

	if params.ManagedBlock {

		if !strings.Contains(content, params.Content) {
			return false
		}
	} else {

		currentHash := sha256.Sum256([]byte(content))
		desiredHash := sha256.Sum256([]byte(params.Content))
		if currentHash != desiredHash {
			return false
		}
	}

	if params.Mode != "" {

		var desiredMode uint64
		if _, err := fmt.Sscanf(params.Mode, "%o", &desiredMode); err == nil {
			currentMode := mode.Perm()
			if uint32(currentMode) != uint32(desiredMode) {
				return false
			}
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

var criticalFiles = []string{
	"/etc/passwd",
	"/etc/shadow",
	"/etc/group",
	"/etc/gshadow",
	"/etc/sudoers",
	"/etc/fstab",
	"/etc/hosts",
	"/etc/hostname",
	"/etc/resolv.conf",
	"/etc/nsswitch.conf",
	"/etc/ssh/sshd_config",
	"/etc/pam.conf",
	"/etc/machine-id",
}

func isProtectedPath(path string) bool {
	cleanPath := filepath.Clean(path)

	if sysfs.IsProtectedPath(cleanPath) {
		return true
	}

	if isCriticalFile(cleanPath) {
		return true
	}

	parts := strings.Split(strings.TrimPrefix(cleanPath, "/"), "/")
	if len(parts) == 1 && parts[0] != "" {
		return true
	}

	return false
}

func isCriticalFile(path string) bool {
	cleanPath := filepath.Clean(path)
	for _, critical := range criticalFiles {
		if cleanPath == critical {
			return true
		}
	}
	return false
}
