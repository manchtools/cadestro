package executor

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manchtools/cadestro/sdk/sys/desktop"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

type recordingBaseRunner struct{ cmds []sysexec.Command }

func (r *recordingBaseRunner) Run(_ context.Context, c sysexec.Command) (sysexec.Result, error) {
	r.cmds = append(r.cmds, c)
	return sysexec.Result{}, nil
}

func (r *recordingBaseRunner) Stream(_ context.Context, c sysexec.Command, _ sysexec.OutputCallback) (sysexec.Result, error) {
	r.cmds = append(r.cmds, c)
	return sysexec.Result{}, nil
}

func (r *recordingBaseRunner) Backend() sysexec.PrivilegeBackend { return sysexec.Direct }

func TestRunAsUser_WorkingDirAndPerUserEnv(t *testing.T) {
	prev := executorRunner
	t.Cleanup(func() { executorRunner = prev })
	rec := &recordingBaseRunner{}
	executorRunner = rec

	s := desktop.Session{Username: "alice", UID: 1000, Home: "/home/alice"}

	_, err := runAsUser(context.Background(), s, nil, "/work/dir", "/bin/echo", []string{"hi"})
	require.NoError(t, err)
	require.Len(t, rec.cmds, 1)
	cmd := rec.cmds[0]
	assert.Equal(t, "/work/dir", cmd.Dir,
		"WorkingDirectory must reach the wrapped runuser command (RunAsRunner honors Command.Dir)")
	joined := strings.Join(cmd.Args, " ")
	assert.Contains(t, joined, "HOME=/home/alice", "per-user HOME set via RunAsRunner")
	assert.Contains(t, joined, "USER=alice", "per-user USER set via RunAsRunner")
	assert.Contains(t, joined, "PATH="+desktop.UserPath(s), "curated per-user PATH (not the agent root's)")
	assert.Contains(t, joined, "alice", "command runs as the session user")

	rec.cmds = nil
	_, err = runAsUser(context.Background(), s, nil, "", "/bin/echo", []string{"hi"})
	require.NoError(t, err)
	require.Len(t, rec.cmds, 1)
	assert.Equal(t, "/home/alice", rec.cmds[0].Dir, "empty WorkingDirectory defaults to the user's home")
}
