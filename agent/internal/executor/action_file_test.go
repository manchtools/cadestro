package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysfs "github.com/manchtools/cadestro/sdk/sys/fs"
)

func TestExecuteFile_RejectsNilParams(t *testing.T) {
	e := NewExecutor(nil)
	_, changed, err := e.executeFile(context.Background(), nil, pb.DesiredState_DESIRED_STATE_PRESENT)
	if err == nil {
		t.Fatal("expected error for nil params, got nil")
	}
	if changed {
		t.Error("changed must be false when params are nil")
	}
}

func TestExecuteFile_RejectsContentExceedingMaxSize(t *testing.T) {
	e := NewExecutor(nil)
	oversized := strings.Repeat("x", maxFileContentSize+1)
	params := &pb.FileParams{Path: "/tmp/test.txt", Content: oversized}
	_, changed, err := e.executeFile(context.Background(), params, pb.DesiredState_DESIRED_STATE_PRESENT)
	if err == nil {
		t.Fatal("expected error for oversized content, got nil")
	}
	if changed {
		t.Error("changed must be false when content exceeds max size")
	}
	if !strings.Contains(err.Error(), "exceeds maximum size") {
		t.Errorf("error should mention size limit, got %q", err)
	}
}

func TestExecuteFile_ContentAtMaxSizeAccepted(t *testing.T) {
	e := NewExecutor(nil)
	atLimit := strings.Repeat("x", maxFileContentSize)
	params := &pb.FileParams{Path: "/tmp/nonexistent/test.txt", Content: atLimit}
	_, _, err := e.executeFile(context.Background(), params, pb.DesiredState_DESIRED_STATE_PRESENT)

	if err != nil && strings.Contains(err.Error(), "exceeds maximum size") {
		t.Errorf("content at exactly maxFileContentSize must not be rejected as oversized: %v", err)
	}
}

func TestExecuteFile_RejectsUnknownDesiredState(t *testing.T) {
	e := NewExecutor(nil)
	params := &pb.FileParams{Path: "/tmp/test.txt", Content: "hello"}
	_, changed, err := e.executeFile(context.Background(), params, pb.DesiredState(999))
	if err == nil {
		t.Fatal("expected error for unknown desired state, got nil")
	}
	if changed {
		t.Error("changed must be false for unknown state")
	}
}

func TestFileMatchesDesired_ReturnsFalseForMissingFile(t *testing.T) {
	e := NewExecutor(nil)
	if e.fileMatchesDesired(context.Background(), "/nonexistent/path/that/does/not/exist.txt", &pb.FileParams{
		Content: "hello",
	}) {
		t.Error("fileMatchesDesired must return false for a non-existent file")
	}
}

func TestFileMatchesDesired_ReturnsFalseWhenPathIsDirectory(t *testing.T) {
	e := NewExecutor(nil)
	if e.fileMatchesDesired(context.Background(), "/tmp", &pb.FileParams{Content: "hello"}) {
		t.Error("fileMatchesDesired must return false for a directory")
	}
}

func TestFileMatchesDesired_OwnerOnlyCheck(t *testing.T) {
	e := NewExecutor(nil)
	ctx := context.Background()

	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	const content = "hello world"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	owner, group := getFileOwnership(path)
	if owner == "" || group == "" {
		t.Skip("ownership lookup unavailable on this platform")
	}

	if !e.fileMatchesDesired(ctx, path, &pb.FileParams{Content: content, Group: group}) {
		t.Error("group-only request with the file's own group must match (empty-Owner regression)")
	}

	if !e.fileMatchesDesired(ctx, path, &pb.FileParams{Content: content, Owner: owner}) {
		t.Error("owner-only request with the file's own owner must match")
	}

	if e.fileMatchesDesired(ctx, path, &pb.FileParams{Content: content, Owner: owner + "-nope"}) {
		t.Error("owner-only request with a non-matching owner must not match")
	}

	if e.fileMatchesDesired(ctx, "/nonexistent/test.txt", &pb.FileParams{Group: "wheel"}) {
		t.Error("must return false for a non-existent file")
	}
}

func TestDirectoryMatchesDesired_ReturnsFalseForMissingDir(t *testing.T) {
	e := NewExecutor(nil)
	if e.directoryMatchesDesired(context.Background(), "/nonexistent/dir", &pb.DirectoryParams{}) {
		t.Error("directoryMatchesDesired must return false for a non-existent directory")
	}
}

func TestDirectoryMatchesDesired_ReturnsFalseForRegularFile(t *testing.T) {
	e := NewExecutor(nil)

	if e.directoryMatchesDesired(context.Background(), "/etc/hostname", &pb.DirectoryParams{}) {
		t.Error("directoryMatchesDesired must return false for a regular file")
	}
}

func TestIsProtectedPath_CriticalFiles(t *testing.T) {
	for _, path := range criticalFiles {
		t.Run(path, func(t *testing.T) {
			if !isCriticalFile(path) {
				t.Errorf("isCriticalFile(%q) = false, want true", path)
			}
		})
	}

	if isCriticalFile("/etc/hosts.allow") {
		t.Error("/etc/hosts.allow is not in the denylist, must not be critical")
	}
	if isCriticalFile("/etc/ssh/sshd_config.d/01-cadestro-test.conf") {
		t.Error("drop-in config file must not be flagged as critical")
	}
}

func TestIsProtectedPath_TopLevelChildren(t *testing.T) {
	if !isProtectedPath("/lost+found") {
		t.Error("immediate child of / (lost+found) must be protected")
	}
	if !isProtectedPath("/opt") {
		t.Error("immediate child of / (opt) must be protected")
	}
}

func TestIsProtectedPath_DeniesEtcSubdirs(t *testing.T) {
	protectedSubdirs := []string{
		"/etc/sudoers.d",
		"/etc/systemd/system",
		"/etc/ssh",
		"/etc/pam.d",
	}
	for _, path := range protectedSubdirs {
		t.Run(path, func(t *testing.T) {

			if !isProtectedPath(path) && !sysfs.IsUnderProtectedPrefix(path) {
				t.Errorf("%s must be refused by the protection guard (isProtectedPath || IsUnderProtectedPrefix)", path)
			}
		})
	}
}

func TestExecuteFile_PRESENT_RejectsBeforeRemount(t *testing.T) {
	var remountCalled bool
	e := NewExecutor(nil)
	e.repairFS = func(ctx context.Context) bool {
		remountCalled = true
		return true
	}

	params := &pb.FileParams{Path: "/etc/sudoers", Content: "# evil config"}
	_, _, err := e.executeFile(context.Background(), params, pb.DesiredState_DESIRED_STATE_PRESENT)
	if err == nil {
		t.Fatal("expected error for PRESENT overwrite of sudoers, got nil")
	}
	if remountCalled {
		t.Error("requireWritableFS must NOT be called for a critical-file PRESENT rejection")
	}
}
