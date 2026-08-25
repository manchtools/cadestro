package executor

import (
	"context"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/sdk/sys/desktop"
	sysenc "github.com/manchtools/cadestro/sdk/sys/encryption"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
	sysfs "github.com/manchtools/cadestro/sdk/sys/fs"
	"github.com/manchtools/cadestro/sdk/sys/network"
	sysnotify "github.com/manchtools/cadestro/sdk/sys/notify"
	sysservice "github.com/manchtools/cadestro/sdk/sys/service"
	sysuser "github.com/manchtools/cadestro/sdk/sys/user"
)

type executorDeps struct {
	desktop desktop.Manager
	service sysservice.Manager
	network network.Manager
	user    sysuser.Manager
	fs      sysfs.Manager
	encrypt sysenc.Manager
	notify  sysnotify.Manager
}

func newExecutorDeps(runner sysexec.Runner) executorDeps {
	managerRunner := runner
	if managerRunner == nil {
		managerRunner = mustDirectRunner()
	}
	return executorDeps{
		desktop: mustDesktopManager(managerRunner),
		service: mustServiceManager(managerRunner),
		network: mustNetworkManager(managerRunner),
		user:    mustUserManager(managerRunner),
		fs:      mustFSManager(managerRunner),
		encrypt: mustEncManager(managerRunner),
		notify:  mustNotifyManager(managerRunner),
	}
}

func (e *Executor) ensureDeps() {
	if e.deps.desktop != nil && e.deps.service != nil && e.deps.network != nil && e.deps.user != nil && e.deps.fs != nil && e.deps.encrypt != nil && e.deps.notify != nil {
		return
	}
	e.depsOnce.Do(func() {
		defaults := newExecutorDeps(e.runner)
		if e.deps.desktop == nil {
			e.deps.desktop = defaults.desktop
		}
		if e.deps.service == nil {
			e.deps.service = defaults.service
		}
		if e.deps.network == nil {
			e.deps.network = defaults.network
		}
		if e.deps.user == nil {
			e.deps.user = defaults.user
		}
		if e.deps.fs == nil {
			e.deps.fs = defaults.fs
		}
		if e.deps.encrypt == nil {
			e.deps.encrypt = defaults.encrypt
		}
		if e.deps.notify == nil {
			e.deps.notify = defaults.notify
		}
	})
}

func mustNotifyManager(r sysexec.Runner) sysnotify.Manager {
	m, err := sysnotify.New(r)
	if err != nil {
		panic("executor: notify manager must construct: " + err.Error())
	}
	return m
}

func (e *Executor) runSudo(ctx context.Context, name string, args ...string) (*pb.CommandOutput, error) {
	r, err := e.runnerOrDirect().Run(ctx, sysexec.Command{Name: name, Args: args, Escalate: true})
	return toOutput(&r), asCmdError(name, r, err)
}

func (e *Executor) runnerOrDirect() sysexec.Runner {
	if e.runner != nil {
		return e.runner
	}
	return mustDirectRunner()
}
