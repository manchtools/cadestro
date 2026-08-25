//go:build integration

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

func TestIntegration_DirectoryCreateAndRemove(t *testing.T) {
	e := newTestExecutor()
	root := t.TempDir()

	target := filepath.Join(root, "managed")
	_, changed, err := e.executeDirectory(context.Background(),
		&pb.DirectoryParams{Path: target, Mode: "0750"},
		pb.DesiredState_DESIRED_STATE_PRESENT)
	require.NoError(t, err)
	assert.True(t, changed)

	info, statErr := os.Stat(target)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
	assert.Equal(t, os.FileMode(0o750), info.Mode().Perm())

	_, changed, err = e.executeDirectory(context.Background(),
		&pb.DirectoryParams{Path: target, Mode: "0750"},
		pb.DesiredState_DESIRED_STATE_PRESENT)
	require.NoError(t, err)
	assert.False(t, changed)

	_, changed, err = e.executeDirectory(context.Background(),
		&pb.DirectoryParams{Path: target},
		pb.DesiredState_DESIRED_STATE_ABSENT)
	require.NoError(t, err)
	assert.True(t, changed)

	_, statErr = os.Stat(target)
	assert.True(t, os.IsNotExist(statErr))

	_, changed, err = e.executeDirectory(context.Background(),
		&pb.DirectoryParams{Path: target},
		pb.DesiredState_DESIRED_STATE_ABSENT)
	require.NoError(t, err)
	assert.False(t, changed)
}

func TestIntegration_DirectoryRefusesSymlinkOnCreate(t *testing.T) {
	_ = newTestExecutor()
	root := t.TempDir()

	victim := filepath.Join(root, "victim")
	require.NoError(t, os.Mkdir(victim, 0o700))
	link := filepath.Join(root, "managed")
	require.NoError(t, os.Symlink(victim, link))

	err := createDirectoryWithPermissions(context.Background(), link, "0777", "", "", false)
	require.Error(t, err)

	info, statErr := os.Stat(victim)
	require.NoError(t, statErr)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(),
		"symlink target's mode must be unchanged")
}

func TestIntegration_DirectoryRefusesSymlinkOnRemove(t *testing.T) {
	_ = newTestExecutor()
	root := t.TempDir()

	victim := filepath.Join(root, "victim")
	require.NoError(t, os.MkdirAll(victim, 0o755))
	link := filepath.Join(root, "managed")
	require.NoError(t, os.Symlink(victim, link))

	err := removeDirectory(context.Background(), link)
	require.Error(t, err)
	_, statErr := os.Stat(victim)
	assert.NoError(t, statErr, "symlink target must not be removed")
}

func TestIntegration_FileCreateAndRemove(t *testing.T) {
	e := newTestExecutor()
	root := t.TempDir()

	target := filepath.Join(root, "test.txt")
	content := "hello world"

	_, changed, err := e.executeFile(context.Background(),
		&pb.FileParams{Path: target, Content: content, Mode: "0644"},
		pb.DesiredState_DESIRED_STATE_PRESENT)
	require.NoError(t, err)
	assert.True(t, changed)

	data, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	assert.Equal(t, content, string(data))

	_, changed, err = e.executeFile(context.Background(),
		&pb.FileParams{Path: target, Content: content, Mode: "0644"},
		pb.DesiredState_DESIRED_STATE_PRESENT)
	require.NoError(t, err)
	assert.False(t, changed)

	_, changed, err = e.executeFile(context.Background(),
		&pb.FileParams{Path: target},
		pb.DesiredState_DESIRED_STATE_ABSENT)
	require.NoError(t, err)
	assert.True(t, changed)
	_, statErr := os.Stat(target)
	assert.True(t, os.IsNotExist(statErr))
}

func TestIntegration_FileManagedBlock(t *testing.T) {
	e := newTestExecutor()
	root := t.TempDir()
	target := filepath.Join(root, "config.txt")

	initial := "line1\nline2\n"
	require.NoError(t, os.WriteFile(target, []byte(initial), 0644))

	block := "# managed block\nmanaged-setting=true\n"
	params := &pb.FileParams{Path: target, Content: block, ManagedBlock: true, Mode: "0644"}
	_, changed, err := e.executeFile(context.Background(), params, pb.DesiredState_DESIRED_STATE_PRESENT)
	require.NoError(t, err)
	assert.True(t, changed)

	data, _ := os.ReadFile(target)
	assert.Contains(t, string(data), initial)
	assert.Contains(t, string(data), block)

	_, changed, err = e.executeFile(context.Background(), params, pb.DesiredState_DESIRED_STATE_ABSENT)
	require.NoError(t, err)
	assert.True(t, changed)

	data, _ = os.ReadFile(target)
	assert.Contains(t, string(data), initial)
	assert.NotContains(t, string(data), block)
}

func TestIntegration_ServiceManagerWriteUnitDelegates(t *testing.T) {
	e := newTestExecutor()

	params := &pb.ServiceParams{UnitName: "../../etc/cron.d/evil.service", UnitContent: "[Service]\nExecStart=/bin/true\n"}
	_, _, err := e.executeService(context.Background(), params)
	if err == nil {
		t.Fatal("expected error for path-escaping unit name, got nil")
	}
}

func TestIntegration_ShellScriptRunsThroughRealRunner(t *testing.T) {
	e := newTestExecutor()

	out, err := e.runShellScript(context.Background(),
		&pb.ShellParams{RunAsRoot: true}, "true")
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, int32(0), out.ExitCode)

	out, err = e.runShellScript(context.Background(),
		&pb.ShellParams{RunAsRoot: true}, "exit 42")
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, int32(42), out.ExitCode)
}
