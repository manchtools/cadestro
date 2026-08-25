package executor

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/sdk/pkg"
)

type probeErrPkgManager struct{ pkg.Manager }

func (probeErrPkgManager) IsInstalled(context.Context, string) (bool, error) {
	return false, errors.New("backend probe failed")
}

func TestExecutePackage_FailsClosedOnProbeError(t *testing.T) {
	e := &Executor{logger: slog.Default(), now: time.Now, pkgBackend: pkg.Apt, pkgManager: probeErrPkgManager{}}
	for _, state := range []pb.DesiredState{
		pb.DesiredState_DESIRED_STATE_PRESENT,
		pb.DesiredState_DESIRED_STATE_ABSENT,
	} {
		if _, _, err := e.executePackage(context.Background(), &pb.PackageParams{Name: "anything"}, state); err == nil {
			t.Errorf("state %v: a probe error must fail closed, not proceed to a privileged mutation", state)
		}
	}
}

func TestDefaultTimeoutForAction(t *testing.T) {
	cases := []struct {
		name      string
		actType   pb.ActionType
		requested int32
		want      int32
	}{
		{"explicit timeout always wins", pb.ActionType_ACTION_TYPE_PACKAGE, 42, 42},
		{"shell default", pb.ActionType_ACTION_TYPE_SHELL, 0, defaultScriptTimeout},
		{"script default", pb.ActionType_ACTION_TYPE_SCRIPT_RUN, 0, defaultScriptTimeout},
		{"package default (was unbounded)", pb.ActionType_ACTION_TYPE_PACKAGE, 0, defaultPackageTimeout},
		{"update default (was unbounded)", pb.ActionType_ACTION_TYPE_UPDATE, 0, defaultPackageTimeout},
		{"other action: no timeout", pb.ActionType_ACTION_TYPE_FILE, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := defaultTimeoutForAction(tc.actType, tc.requested); got != tc.want {
				t.Errorf("defaultTimeoutForAction(%v, %d) = %d, want %d", tc.actType, tc.requested, got, tc.want)
			}
		})
	}
}

type fakePkgManager struct {
	pkg.Manager
	captured chan context.Context
}

func (f fakePkgManager) IsInstalled(ctx context.Context, _ string) (bool, error) {
	if f.captured != nil {
		select {
		case f.captured <- ctx:
		default:
		}
	}
	return true, nil
}

func TestExecutePackage_PassesActionContextToManager(t *testing.T) {
	capturedCh := make(chan context.Context, 1)
	e := &Executor{
		logger:     slog.Default(),
		now:        time.Now,
		pkgBackend: pkg.Apt,
		pkgManager: fakePkgManager{captured: capturedCh},
	}

	actionCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {

		_, _, _ = e.executePackage(actionCtx, &pb.PackageParams{Name: "anything"}, pb.DesiredState_DESIRED_STATE_PRESENT)
	}()

	var captured context.Context
	select {
	case captured = <-capturedCh:
	case <-time.After(2 * time.Second):
		t.Fatal("executePackage never called the package manager with the action context (WS16 #3)")
	}

	if captured == context.Background() {
		t.Fatal("manager was called with context.Background, not the action context (WS16 #3)")
	}

	cancel()
	select {
	case <-captured.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("captured manager context did not observe the action-context cancel")
	}
}

func TestPkgManagerForCtx_CancelledCtx_FailsClosed(t *testing.T) {
	mgr := fakePkgManager{}
	e := &Executor{pkgManager: mgr}

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := e.pkgManagerForCtx(cancelledCtx); got != nil {
		t.Error("a cancelled action ctx must fail closed (nil), not return a usable manager")
	}

	if got := e.pkgManagerForCtx(context.Background()); got != mgr {
		t.Error("with a live ctx, pkgManagerForCtx must return the configured manager")
	}
}
