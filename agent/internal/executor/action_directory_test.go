package executor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func TestExecuteDirectory_RejectsNilParams(t *testing.T) {
	e := NewExecutor(nil)
	_, changed, err := e.executeDirectory(context.Background(), nil, pb.DesiredState_DESIRED_STATE_PRESENT)
	require.Error(t, err)
	assert.False(t, changed)
	assert.Contains(t, err.Error(), "required")
}

func TestExecuteDirectory_RejectsEmptyPath(t *testing.T) {
	e := NewExecutor(nil)
	_, changed, err := e.executeDirectory(context.Background(),
		&pb.DirectoryParams{Path: ""}, pb.DesiredState_DESIRED_STATE_PRESENT)
	require.Error(t, err)
	assert.False(t, changed)
}

func TestExecuteDirectory_RejectsUnknownState(t *testing.T) {
	e := NewExecutor(nil)
	_, changed, err := e.executeDirectory(context.Background(),
		&pb.DirectoryParams{Path: "/tmp/test"}, pb.DesiredState(999))
	require.Error(t, err)
	assert.False(t, changed)
}

func TestExecuteDirectory_PRESENT_RefusesProtectedPath(t *testing.T) {
	e := NewExecutor(nil)

	for _, p := range []string{
		"/etc", "/", "/usr",
		"/etc/sudoers.d", "/etc/sudoers.d/custom",
		"/var/lib/postgresql", "/home/alice", "/boot/efi", "/usr/local/bin",
	} {
		_, changed, err := e.executeDirectory(context.Background(),
			&pb.DirectoryParams{Path: p, Mode: "0777"},
			pb.DesiredState_DESIRED_STATE_PRESENT)
		require.Errorf(t, err, "PRESENT on protected %q must be refused", p)
		assert.Contains(t, err.Error(), "protected")
		assert.False(t, changed)
	}
}

func TestExecuteDirectory_ABSENT_DenyByDefault(t *testing.T) {
	e := NewExecutor(nil)

	for _, p := range []string{
		"/etc/sudoers.d/cadestro-ws6-nope",
		"/etc/cron.d/cadestro-ws6-nope",
		"/boot/efi/cadestro-ws6-nope",
		"/var/lib/cadestro-ws6-nope",
		"/home/cadestro-ws6-victim",
		"/root/.ssh",
		"/usr/lib/cadestro-ws6-nope",
	} {
		_, changed, err := e.executeDirectory(context.Background(),
			&pb.DirectoryParams{Path: p},
			pb.DesiredState_DESIRED_STATE_ABSENT)
		require.Errorf(t, err, "ABSENT on protected subtree %q must be refused", p)
		assert.Contains(t, err.Error(), "protected")
		assert.False(t, changed)
	}
}

func TestDirectoryMatchesDesired_ReturnsFalseForNonExistent(t *testing.T) {
	e := &Executor{}

	assert.False(t, e.directoryMatchesDesired(context.Background(), "/nonexistent/dir", &pb.DirectoryParams{}))

	tmpFile := filepath.Join(t.TempDir(), "regular-file")
	require.NoError(t, os.WriteFile(tmpFile, []byte("content"), 0644))
	assert.False(t, e.directoryMatchesDesired(context.Background(), tmpFile, &pb.DirectoryParams{}))
}
