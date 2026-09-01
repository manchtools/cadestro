// Package service manages init/service units through an injected exec.Runner.
//
// Build a Manager over an exec.Runner, then call its methods. Query verbs (is-enabled/is-active) run
// unprivileged; mutations escalate through the Runner.
//
//	r, _ := exec.NewRunner(exec.Direct)
//	svc, _ := service.New(r)
//	_ = svc.EnableNow(ctx, "nginx.service")
//
// Available reports whether systemd is usable on the host.
package service

import (
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"time"

	"github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/fs"
)

// UnitStatus is a unit's current state.
type UnitStatus struct {
	Enabled bool
	Active  bool
	Masked  bool
	Static  bool
}

// Manager controls systemd units through runner.
type Manager struct {
	r   exec.Runner
	fsm fsManager
}

// New builds a Manager driven by runner. A nil runner is rejected.
func New(runner exec.Runner) (*Manager, error) {
	if runner == nil {
		return nil, fmt.Errorf("service: %w", exec.ErrRunnerRequired)
	}
	fsm, err := newFS(runner)
	if err != nil {
		return nil, err
	}
	return &Manager{r: runner, fsm: fsm}, nil
}

// Available reports whether systemd is usable on this host.
func Available() bool {
	if _, err := lookPath("systemctl"); err != nil {
		return false
	}
	_, err := os.Stat(systemdRunMarker)
	return err == nil
}

const systemctlQueryTimeout = 30 * time.Second

func ensureCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, systemctlQueryTimeout)
}

type fsManager interface {
	ReadFile(ctx context.Context, path string) ([]byte, error)
	WriteFile(ctx context.Context, path string, data []byte, opts fs.WriteOptions) error
	Remove(ctx context.Context, path string) error
}

var (
	lookPath         = osexec.LookPath
	systemdRunMarker = "/run/systemd/system"
	newFS            = func(r exec.Runner) (fsManager, error) { return fs.New(r) }
)
