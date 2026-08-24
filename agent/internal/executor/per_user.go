// per_user.go — agent-side wrappers around sdk/go/sys/desktop's
// per-user fan-out. The package-local helpers convert between the
// SDK's *exec.Cmd shape and the agent's *pb.CommandOutput shape so
// per-user execution paths match the existing command-helper ergonomics
// instead of forcing every caller to learn a parallel API.
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

// mustDesktopManager constructs the desktop capability used by one executor.
func mustDesktopManager(r sysexec.Runner) desktop.Manager {
	m, err := desktop.New(r)
	if err != nil {
		panic("executor: desktop manager must construct: " + err.Error())
	}
	return m
}

// mustServiceManager constructs the service capability used by one executor.
func mustServiceManager(r sysexec.Runner) sysservice.Manager {
	m, err := sysservice.New(sysservice.Systemd, r)
	if err != nil {
		panic("executor: service manager must construct: " + err.Error())
	}
	return m
}

// mustNetworkManager constructs the network capability used by one executor.
func mustNetworkManager(r sysexec.Runner) network.Manager {
	m, err := network.New(network.NetworkManager, r)
	if err != nil {
		panic("executor: network manager must construct: " + err.Error())
	}
	return m
}

// mustUserManager constructs the user capability used by one executor.
func mustUserManager(r sysexec.Runner) sysuser.Manager {
	m, err := sysuser.New(sysuser.ShadowUtils, r)
	if err != nil {
		panic("executor: user manager must construct: " + err.Error())
	}
	return m
}

// mustFSManager constructs the filesystem capability used by one executor.
func mustFSManager(r sysexec.Runner) sysfs.Manager {
	m, err := sysfs.New(r)
	if err != nil {
		panic("executor: fs manager must construct: " + err.Error())
	}
	return m
}

// mustEncManager constructs the encryption capability used by one executor.
func mustEncManager(r sysexec.Runner) sysenc.Manager {
	m, err := sysenc.New(sysenc.LUKS, r)
	if err != nil {
		panic("executor: encryption manager must construct: " + err.Error())
	}
	return m
}

// runAsUser runs `name args...` as the given session's user. The wrapper
// builds `runuser -u <user> -- <name> <args...>` and hands the
// resulting args + env (desktop defaults plus extraEnv) to the SDK runner.
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
	// desktop.RunAsRunner wraps the command to run AS the session user: it builds
	// `runuser -u <user> -- env <session-env> PATH=<curated UserPath> <name>
	// <args>` and runs it in Command.Dir. So the per-user env (HOME/USER/
	// XDG_RUNTIME_DIR/…), the curated per-user PATH (not root's), and the working
	// directory are all owned by the SDK now — no hand-built runuser/env splicing
	// here. extraEnv is screened + merged by RunAsRunner.
	ru, err := desktop.RunAsRunner(e.runnerOrDirect(), s)
	if err != nil {
		return nil, err
	}
	r, err := ru.Run(ctx, sysexec.Command{Name: name, Args: args, Env: extraEnv, Dir: dir})
	return toOutput(&r), err
}

// errEmptyName / errEmptyUsername are sentinel errors so the
// callers can distinguish "caller bug" from "runuser execution
// failure." Pinned as vars rather than fmt.Errorf'd inline so a
// test can errors.Is() against them without string matching.
var (
	errEmptyName     = errPerUser("name is required")
	errEmptyUsername = errPerUser("session has empty Username")
)

type errPerUser string

func (e errPerUser) Error() string { return "executor.runAsUser: " + string(e) }
