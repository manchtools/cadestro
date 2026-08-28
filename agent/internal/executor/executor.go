package executor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/sdk/pkg"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Executor struct {
	runner     sysexec.Runner
	pkgManager pkg.Manager
	now        func() time.Time
}

func NewExecutor(runner sysexec.Runner) *Executor {
	if runner == nil {
		var err error
		runner, err = sysexec.NewRunner(sysexec.Direct)
		if err != nil {
			panic(err)
		}
	}
	executor := &Executor{runner: runner, now: time.Now}
	backends := pkg.Detect()
	if len(backends) > 0 {
		manager, err := pkg.New(backends[0], runner)
		if err == nil {
			executor.pkgManager = manager
		}
	}
	return executor
}

func (e *Executor) ResetUpdateCycle() {}

func (e *Executor) ExecuteAction(ctx context.Context, action *pb.Action) *pb.ActionResult {
	started := e.now()
	result := &pb.ActionResult{Status: pb.ExecutionStatus_EXECUTION_STATUS_FAILED}
	if action == nil || action.GetId() == nil {
		result.Error = "action is required"
		return e.finish(result, started)
	}
	result.ActionId = action.GetId()
	if timeout := action.GetTimeoutSeconds(); timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
	}

	var output *pb.CommandOutput
	var changed bool
	var err error
	switch action.GetType() {
	case pb.ActionType_ACTION_TYPE_PACKAGE:
		output, changed, err = e.executePackage(ctx, action.GetPackage(), action.GetDesiredState())
	case pb.ActionType_ACTION_TYPE_UPDATE:
		output, changed, err = e.executeUpdate(ctx, action.GetUpdate())
	case pb.ActionType_ACTION_TYPE_SHELL:
		output, result.DetectionOutput, result.Compliant, changed, err = e.executeShell(ctx, action.GetShell())
	default:
		err = fmt.Errorf("unsupported action type: %s", action.GetType())
	}
	result.Output = output
	result.Changed = changed
	if err == nil {
		result.Status = pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS
	} else if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		result.Status = pb.ExecutionStatus_EXECUTION_STATUS_TIMEOUT
		result.Error = "action timed out"
	} else {
		result.Error = err.Error()
	}
	return e.finish(result, started)
}

func (e *Executor) finish(result *pb.ActionResult, started time.Time) *pb.ActionResult {
	completed := e.now()
	result.CompletedAt = timestamppb.New(completed)
	result.Duration = durationpb.New(completed.Sub(started))
	return result
}

func (e *Executor) executeShell(ctx context.Context, params *pb.ShellParams) (*pb.CommandOutput, *pb.CommandOutput, bool, bool, error) {
	if params == nil {
		return nil, nil, false, false, errors.New("shell params required")
	}
	if params.GetIsCompliance() && params.GetDetectionScript() == "" {
		return nil, nil, false, false, errors.New("compliance shell action requires a detection script")
	}
	var detection *pb.CommandOutput
	if params.GetDetectionScript() != "" {
		var err error
		detection, err = e.runShell(ctx, params, params.GetDetectionScript())
		if err != nil {
			return nil, detection, false, false, err
		}
		if detection.GetExitCode() == 0 {
			return detection, detection, true, false, nil
		}
		if params.GetIsCompliance() {
			return detection, detection, false, false, nil
		}
	}
	if params.GetScript() == "" {
		return nil, detection, false, false, errors.New("shell action requires a script")
	}
	output, err := e.runShell(ctx, params, params.GetScript())
	if err != nil || output.GetExitCode() != 0 {
		if err == nil {
			err = fmt.Errorf("shell exited with status %d", output.GetExitCode())
		}
		return output, detection, false, false, err
	}
	if params.GetDetectionScript() == "" {
		return output, nil, false, true, nil
	}
	verified, err := e.runShell(ctx, params, params.GetDetectionScript())
	if err != nil {
		return output, verified, false, true, err
	}
	if verified.GetExitCode() != 0 {
		return output, verified, false, true, fmt.Errorf("shell remediation verification exited with status %d", verified.GetExitCode())
	}
	return output, verified, true, true, nil
}

func (e *Executor) runShell(ctx context.Context, params *pb.ShellParams, script string) (*pb.CommandOutput, error) {
	interpreter := params.GetInterpreter()
	if interpreter == "" {
		interpreter = "/bin/sh"
	}
	keys := make([]string, 0, len(params.GetEnvironment()))
	for key := range params.GetEnvironment() {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		environment = append(environment, key+"="+params.GetEnvironment()[key])
	}
	if err := sysexec.ValidateCommandEnv(environment); err != nil {
		return nil, fmt.Errorf("validate shell environment: %w", err)
	}
	command := sysexec.Command{
		Name: interpreter, Args: []string{"-c", script}, Dir: params.GetWorkingDirectory(),
		Env: environment, Escalate: true,
	}
	run, err := e.runner.Run(ctx, command)
	output := &pb.CommandOutput{ExitCode: int32(run.ExitCode), Stdout: run.Stdout, Stderr: run.Stderr}
	if err != nil {
		slog.Debug("shell runner failed", "error", err)
		return output, fmt.Errorf("run shell: %w", err)
	}
	return output, nil
}
