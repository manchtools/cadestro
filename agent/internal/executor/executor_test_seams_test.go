package executor

// These compatibility seams are test-only. Production code routes through
// Executor.deps; the legacy names remain for integration helpers that exercise
// SDK capabilities directly rather than an Executor action.

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
	notifyAll      = func(ctx context.Context, title, body string) { _ = testNotify().NotifyAll(ctx, title, body) }
	notifyUsers    = func(ctx context.Context, users []string, title, body string) {
		_ = testNotify().NotifyUsers(ctx, users, title, body)
	}
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
func atomicWriteFile(ctx context.Context, path, content, mode, owner, group string) error {
	return testExecutor().atomicWriteFile(ctx, path, content, mode, owner, group)
}
func readFileWithSudo(ctx context.Context, path string) (string, error) {
	return testExecutor().readFileWithSudo(ctx, path)
}
func fileExistsWithSudo(ctx context.Context, path string) bool {
	return testExecutor().fileExistsWithSudo(ctx, path)
}
func removeFileStrict(ctx context.Context, path string) error {
	return testExecutor().removeFileStrict(ctx, path)
}
func createDirectory(ctx context.Context, path string, recursive bool) error {
	return testExecutor().createDirectory(ctx, path, recursive)
}
func createDirectoryWithPermissions(ctx context.Context, path, mode, owner, group string, recursive bool) error {
	return testExecutor().createDirectoryWithPermissions(ctx, path, mode, owner, group, recursive)
}
func removeDirectory(ctx context.Context, path string) error {
	return testExecutor().removeDirectory(ctx, path)
}
func userExists(ctx context.Context, username string) (bool, error) {
	return testExecutor().userExists(ctx, username)
}
func groupExists(ctx context.Context, groupName string) (bool, error) {
	return testExecutor().groupExists(ctx, groupName)
}
func userInGroup(ctx context.Context, username, groupName string) bool {
	return testExecutor().userInGroup(ctx, username, groupName)
}
func getBinaryVersion(path string) (string, error) { return testExecutor().getBinaryVersion(path) }
