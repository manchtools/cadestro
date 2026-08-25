package executor

import (
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

func mustDirectRunner() sysexec.Runner {
	r, err := sysexec.NewRunner(sysexec.Direct)
	if err != nil {
		panic("executor: Direct runner must construct: " + err.Error())
	}
	return r
}

func toOutput(r *sysexec.Result) *pb.CommandOutput {
	if r == nil {
		return nil
	}
	return &pb.CommandOutput{
		ExitCode: int32(r.ExitCode),
		Stdout:   r.Stdout,
		Stderr:   r.Stderr,
	}
}

func asCmdError(name string, r sysexec.Result, err error) error {
	if err != nil {
		return err
	}
	if r.ExitCode != 0 {
		return &sysexec.CommandError{Name: name, ExitCode: r.ExitCode, Stderr: r.Stderr}
	}
	return nil
}
