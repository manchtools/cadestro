package executor

import (
	"context"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/sdk/sys/desktop"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
	sysnotify "github.com/manchtools/cadestro/sdk/sys/notify"
)

var (
	executorRunner = must("Direct runner", func() (sysexec.Runner, error) {
		return sysexec.NewRunner(sysexec.Direct)
	})
	executorDefaults = newExecutorDeps(executorRunner)
	desktopMgr       = executorDefaults.desktop
	serviceMgr       = executorDefaults.service
	networkMgr       = executorDefaults.network
	userMgr          = executorDefaults.user
	fsMgr            = executorDefaults.fs
	encMgr           = executorDefaults.encrypt
)

func testNotify() sysnotify.Manager { return executorDefaults.notify }

func testExecutor() *Executor {
	e := NewExecutor(nil)
	e.runner = executorRunner
	e.deps = executorDeps{desktop: desktopMgr, service: serviceMgr, network: networkMgr, user: userMgr, fs: fsMgr, encrypt: encMgr, notify: testNotify()}
	return e
}

func runAsUser(ctx context.Context, s desktop.Session, extraEnv []string, dir, name string, args []string) (*pb.CommandOutput, error) {
	return testExecutor().runAsUser(ctx, s, extraEnv, dir, name, args)
}
func getBinaryVersion(ctx context.Context, path string) (string, error) {
	return testExecutor().getBinaryVersion(ctx, path)
}
