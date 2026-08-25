package executor

import (
	"context"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/sdk/sys/desktop"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

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
