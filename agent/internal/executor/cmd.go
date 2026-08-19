// Package executor provides thin wrappers around the SDK sys/exec Runner,
// converting between SDK types and protobuf CommandOutput.
package executor

import (
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

// mustDirectRunner constructs the safe default used for non-privileged
// capability setup and zero-value test executors.
func mustDirectRunner() sysexec.Runner {
	r, err := sysexec.NewRunner(sysexec.Direct)
	if err != nil {
		panic("executor: Direct runner must construct: " + err.Error())
	}
	return r
}

// OutputCallback is a type alias for the SDK OutputCallback.
type OutputCallback = sysexec.OutputCallback

// toOutput converts an SDK Result to a protobuf CommandOutput.
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

// asCmdError preserves the pre-rework contract that a non-zero exit is an error.
// The reworked Runner reports a non-zero exit in Result.ExitCode (not as err),
// but every caller of the non-streaming command helpers treats `err != nil` as
// "the command failed" (e.g. `if err != nil { return ..., err }`). Without this
// mapping a failed sudo command would look like success. (Streaming callers, by
// contrast, want the exit code in the output to report a script's status, so
// runCmdStreaming/runAsUserStreaming deliberately do NOT use this.)
func asCmdError(name string, r sysexec.Result, err error) error {
	if err != nil {
		return err
	}
	if r.ExitCode != 0 {
		return &sysexec.CommandError{Name: name, ExitCode: r.ExitCode, Stderr: r.Stderr}
	}
	return nil
}
