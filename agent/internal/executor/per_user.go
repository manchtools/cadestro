package executor

import (
	"context"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/sdk/sys/desktop"
	sysenc "github.com/manchtools/cadestro/sdk/sys/encryption"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
	sysfs "github.com/manchtools/cadestro/sdk/sys/fs"
	"github.com/manchtools/cadestro/sdk/sys/network"
	sysservice "github.com/manchtools/cadestro/sdk/sys/service"
	sysuser "github.com/manchtools/cadestro/sdk/sys/user"
)

func mustDesktopManager(r sysexec.Runner) desktop.Manager {
	m, err := desktop.New(r)
	if err != nil {
		panic("executor: desktop manager must construct: " + err.Error())
	}
	return m
}

func mustServiceManager(r sysexec.Runner) sysservice.Manager {
	m, err := sysservice.New(sysservice.Systemd, r)
	if err != nil {
		panic("executor: service manager must construct: " + err.Error())
	}
	return m
}

func mustNetworkManager(r sysexec.Runner) network.Manager {
	m, err := network.New(network.NetworkManager, r)
	if err != nil {
		panic("executor: network manager must construct: " + err.Error())
	}
	return m
}

func mustUserManager(r sysexec.Runner) sysuser.Manager {
	m, err := sysuser.New(sysuser.ShadowUtils, r)
	if err != nil {
		panic("executor: user manager must construct: " + err.Error())
	}
	return m
}

func mustFSManager(r sysexec.Runner) sysfs.Manager {
	m, err := sysfs.New(r)
	if err != nil {
		panic("executor: fs manager must construct: " + err.Error())
	}
	return m
}

func mustEncManager(r sysexec.Runner) sysenc.Manager {
	m, err := sysenc.New(sysenc.LUKS, r)
	if err != nil {
		panic("executor: encryption manager must construct: " + err.Error())
	}
	return m
}

func (e *Executor) runAsUser(ctx context.Context, s desktop.Session, extraEnv []string, dir string, name string, args []string) (*pb.CommandOutput, error) {
	if name == "" {
		return nil, errEmptyName
	}
	if s.Username == "" {
		return nil, errEmptyUsername
	}
	if dir == "" {
		dir = s.Home
	}

	ru, err := desktop.RunAsRunner(e.runnerOrDirect(), s)
	if err != nil {
		return nil, err
	}
	r, err := ru.Run(ctx, sysexec.Command{Name: name, Args: args, Env: extraEnv, Dir: dir})
	return toOutput(&r), err
}

var (
	errEmptyName     = errPerUser("name is required")
	errEmptyUsername = errPerUser("session has empty Username")
)

type errPerUser string

func (e errPerUser) Error() string { return "executor.runAsUser: " + string(e) }
