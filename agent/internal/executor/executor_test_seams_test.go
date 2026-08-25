package executor

import (
	"context"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/sdk/sys/desktop"
	sysnotify "github.com/manchtools/cadestro/sdk/sys/notify"
)

var (
	executorRunner = mustDirectRunner()
	desktopMgr     = mustDesktopManager(executorRunner)
	serviceMgr     = mustServiceManager(executorRunner)
	networkMgr     = mustNetworkManager(executorRunner)
	userMgr        = mustUserManager(executorRunner)
	fsMgr          = mustFSManager(executorRunner)
	encMgr         = mustEncManager(executorRunner)
)

func testNotify() sysnotify.Manager { return mustNotifyManager(executorRunner) }

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
