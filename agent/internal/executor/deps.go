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
		managerRunner = must("Direct runner", func() (sysexec.Runner, error) {
			return sysexec.NewRunner(sysexec.Direct)
		})
	}
	return executorDeps{
		desktop: must("desktop manager", func() (desktop.Manager, error) { return desktop.New(managerRunner) }),
		service: must("service manager", func() (sysservice.Manager, error) { return sysservice.New(sysservice.Systemd, managerRunner) }),
		network: must("network manager", func() (network.Manager, error) { return network.New(network.NetworkManager, managerRunner) }),
		user:    must("user manager", func() (sysuser.Manager, error) { return sysuser.New(sysuser.ShadowUtils, managerRunner) }),
		fs:      must("fs manager", func() (sysfs.Manager, error) { return sysfs.New(managerRunner) }),
		encrypt: must("encryption manager", func() (sysenc.Manager, error) { return sysenc.New(sysenc.LUKS, managerRunner) }),
		notify:  must("notify manager", func() (sysnotify.Manager, error) { return sysnotify.New(managerRunner) }),
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

func (e *Executor) runSudo(ctx context.Context, name string, args ...string) (*pb.CommandOutput, error) {
	r, err := e.runnerOrDirect().Run(ctx, sysexec.Command{Name: name, Args: args, Escalate: true})
	return toOutput(&r), asCmdError(name, r, err)
}

func (e *Executor) runnerOrDirect() sysexec.Runner {
	if e.runner != nil {
		return e.runner
	}
	return must("Direct runner", func() (sysexec.Runner, error) {
		return sysexec.NewRunner(sysexec.Direct)
	})
}
