package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manchtools/cadestro/sdk/pkg"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/exec/exectest"
)

func TestEnsureFlatpakPinned(t *testing.T) {
	const app = "org.example.App"
	ctx := context.Background()

	t.Run("already pinned", func(t *testing.T) {
		runner := exectest.New(sysexec.Direct)
		mgr, err := pkg.NewFlatpak(runner)
		require.NoError(t, err)
		runner.Push(sysexec.Result{Stdout: app + "\n"}, nil)
		changed, err := ensureFlatpakPinned(ctx, mgr, app)
		require.NoError(t, err)
		assert.False(t, changed)
		assert.Len(t, runner.Calls(), 1)
	})

	t.Run("not pinned", func(t *testing.T) {
		runner := exectest.New(sysexec.Direct)
		mgr, err := pkg.NewFlatpak(runner)
		require.NoError(t, err)
		runner.Push(sysexec.Result{}, nil)
		changed, err := ensureFlatpakPinned(ctx, mgr, app)
		require.NoError(t, err)
		assert.True(t, changed)
		calls := runner.Calls()
		require.Len(t, calls, 2)
		assert.Equal(t, []string{"mask", app, "--system"}, calls[1].Args)
	})

	t.Run("pin failure", func(t *testing.T) {
		runner := exectest.New(sysexec.Direct)
		mgr, err := pkg.NewFlatpak(runner)
		require.NoError(t, err)
		runner.Push(sysexec.Result{}, nil)
		runner.Push(sysexec.Result{}, errors.New("permission denied"))
		_, err = ensureFlatpakPinned(ctx, mgr, app)
		require.Error(t, err)
	})

	t.Run("probe failure", func(t *testing.T) {
		runner := exectest.New(sysexec.Direct)
		mgr, err := pkg.NewFlatpak(runner)
		require.NoError(t, err)
		runner.Push(sysexec.Result{}, errors.New("flatpak unavailable"))
		_, err = ensureFlatpakPinned(ctx, mgr, app)
		require.Error(t, err)
		assert.Len(t, runner.Calls(), 1)
	})
}
