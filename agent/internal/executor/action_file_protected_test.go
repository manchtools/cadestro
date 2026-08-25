package executor

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func TestIsCriticalFile_DenylistExhaustive(t *testing.T) {
	require.NotEmpty(t, criticalFiles, "criticalFiles must not be empty")

	for _, f := range criticalFiles {
		assert.Truef(t, isCriticalFile(f), "listed critical file %q must be recognised", f)
	}

	mustDeny := []string{
		"/etc/passwd", "/etc/shadow", "/etc/group", "/etc/gshadow",
		"/etc/sudoers", "/etc/fstab", "/etc/ssh/sshd_config",
	}
	for _, f := range mustDeny {
		assert.Truef(t, isCriticalFile(f), "intent-critical file %q must be denied", f)
	}

	assert.False(t, isCriticalFile("/etc/foo.d/bar.conf"))
	assert.False(t, isCriticalFile("/opt/myapp/app.conf"))

	assert.True(t, isCriticalFile("/etc/resolv.conf"))
}

func TestIsProtectedPath_DirsAndTopLevelChildren(t *testing.T) {

	for _, p := range []string{"/", "/etc", "/usr", "/var", "/home", "/root", "/boot", "/opt", "/tmp", "/snap"} {
		assert.Truef(t, isProtectedPath(p), "system directory %q must be protected", p)
	}

	assert.True(t, isProtectedPath("/lost+found"), "immediate children of / are protected")
	assert.True(t, isProtectedPath("/etc/passwd"), "critical files are protected")

	assert.False(t, isProtectedPath("/etc/foo.d/bar.conf"), "managed config under a drop-in is not protected")
	assert.False(t, isProtectedPath("/opt/myapp/data"))
}

func TestExecuteFile_ABSENT_RefusesCriticalFile(t *testing.T) {
	e := &Executor{logger: slog.Default(), now: time.Now}

	for _, p := range []string{"/etc/passwd", "/etc/shadow", "/etc/sudoers"} {
		_, changed, err := e.executeFile(context.Background(),
			&pb.FileParams{Path: p}, pb.DesiredState_DESIRED_STATE_ABSENT)
		require.Errorf(t, err, "ABSENT delete of critical file %q must be refused", p)
		assert.Contains(t, err.Error(), "protected")
		assert.False(t, changed)
	}
}

func TestExecuteFile_PRESENT_RefusesOverwriteOfSudoers(t *testing.T) {
	e := &Executor{logger: slog.Default(), now: time.Now}

	_, changed, err := e.executeFile(context.Background(),
		&pb.FileParams{Path: "/etc/sudoers", Content: "pwned ALL=(ALL) NOPASSWD: ALL\n", Mode: "0440"},
		pb.DesiredState_DESIRED_STATE_PRESENT)
	require.Error(t, err, "PRESENT overwrite of /etc/sudoers must be refused")
	assert.Contains(t, err.Error(), "critical")
	assert.False(t, changed)
}
