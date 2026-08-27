package user

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/exec/exectest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecBoundsContext(t *testing.T) {
	f := exectest.New(exec.Direct)
	su, ok := mgr(t, f).(*shadowUtils)
	require.True(t, ok, "Manager is not *shadowUtils")

	_, err := su.exec(context.Background(), exec.Command{Name: "true"})
	require.NoError(t, err)

	ctxs := f.CallContexts()
	require.Len(t, ctxs, 1, "exec must run exactly one command")
	_, hasDeadline := ctxs[0].Deadline()
	assert.True(t, hasDeadline, "exec must bound a deadline-less context via ensureCtx")
}

func TestExecPassesThroughADeadline(t *testing.T) {
	f := exectest.New(exec.Direct)
	su := mgr(t, f).(*shadowUtils)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	deadlineCtx, dlCancel := context.WithTimeout(ctx, 42*time.Second)
	defer dlCancel()
	want, _ := deadlineCtx.Deadline()

	_, err := su.exec(deadlineCtx, exec.Command{Name: "true"})
	require.NoError(t, err)
	got, ok := f.CallContexts()[0].Deadline()
	require.True(t, ok)
	assert.Equal(t, want, got, "an existing deadline must be preserved, not overwritten")
}

func TestAllRunnerCallsRouteThroughExec(t *testing.T) {
	src := nonTestPackageSource(t)
	require.NotEmpty(t, src, "no non-test source read — the scan is broken")
	n := strings.Count(src, "u.r.Run(")
	assert.Equalf(t, 1, n,
		"`u.r.Run(` must appear exactly once (inside exec); found %d. A direct Runner call bypasses the ctx-bounding chokepoint — route it through u.exec(...) instead.", n)
}

func nonTestPackageSource(t *testing.T) string {
	t.Helper()
	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	var b strings.Builder
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		require.NoError(t, err)
		b.Write(data)
	}
	return b.String()
}
