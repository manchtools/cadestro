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
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Executor struct {
	runner     sysexec.Runner
	pkgManager pkg.Manager
	now        func() time.Time
}

const defaultShellTimeout = time.Hour
const defaultPackageTimeout = 30 * time.Minute

func actionTimeout(action *pb.Action) time.Duration {
	if requested := action.GetTimeoutSeconds(); requested > 0 {
		return time.Duration(requested) * time.Second
	}
	switch action.GetParams().(type) {
	case *pb.Action_Shell:
		return defaultShellTimeout
	case *pb.Action_Package, *pb.Action_Update:
		return defaultPackageTimeout
	default:
		return 0
	}
}

func NewExecutor(runner sysexec.Runner) (*Executor, error) {
	if runner == nil {
		return nil, errors.New("executor: runner is required")
	}
	executor := &Executor{runner: runner, now: time.Now}
	backends := pkg.Detect()
	if len(backends) > 0 {
		manager, err := pkg.New(backends[0], runner)
		if err != nil {
			return nil, fmt.Errorf("executor: initialize package manager: %w", err)
		}
		executor.pkgManager = manager
	}
	return executor, nil
}

func (e *Executor) ExecuteAction(ctx context.Context, action *pb.Action) *pb.ActionResult {
	result := &pb.ActionResult{Status: pb.ExecutionStatus_EXECUTION_STATUS_FAILED}
	if action == nil || action.GetId() == nil {
		result.Output = &pb.CommandOutput{Stderr: "action is required"}
		return e.finish(result)
	}
	result.ActionId = action.GetId()
	if timeout := actionTimeout(action); timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	var output *pb.CommandOutput
	var err error
	switch params := action.GetParams().(type) {
	case *pb.Action_Package:
		output, err = e.executePackage(ctx, params.Package, action.GetDesiredState())
	case *pb.Action_Update:
		output, err = e.executeUpdate(ctx, params.Update)
	case *pb.Action_Shell:
		output, result.DetectionOutput, err = e.executeShell(ctx, params.Shell)
	default:
		err = errors.New("unsupported action parameters")
	}
	result.Output = output
	if err == nil {
		result.Status = pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS
	} else if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		result.Status = pb.ExecutionStatus_EXECUTION_STATUS_TIMEOUT
		ensureResultError(result, "action timed out")
	} else {
		ensureResultError(result, err.Error())
	}
	return e.finish(result)
}

func ensureResultError(result *pb.ActionResult, message string) {
	if result.GetOutput().GetStderr() != "" || result.GetDetectionOutput().GetStderr() != "" {
		return
	}
	output := result.Output
	if output == nil {
		output = result.DetectionOutput
	}
	if output == nil {
		output = &pb.CommandOutput{}
		result.Output = output
	}
	output.Stderr = message
}

func (e *Executor) finish(result *pb.ActionResult) *pb.ActionResult {
	completed := e.now()
	result.CompletedAt = timestamppb.New(completed)
	return result
}

func (e *Executor) executeShell(ctx context.Context, params *pb.ShellActionParams) (*pb.CommandOutput, *pb.CommandOutput, error) {
	if params == nil {
		return nil, nil, errors.New("shell params required")
	}
	if params.GetIsCompliance() && params.GetDetectionScript() == "" {
		return nil, nil, errors.New("compliance shell action requires a detection script")
	}
	var detection *pb.CommandOutput
	if params.GetDetectionScript() != "" {
		var err error
		detection, err = e.runShell(ctx, params, params.GetDetectionScript())
		if err != nil {
			detection = outputWithError(detection, err)
			return nil, detection, err
		}
		if detection.GetExitCode() == 0 {
			return nil, detection, nil
		}
		if params.GetIsCompliance() {
			return nil, detection, nil
		}
	}
	if params.GetScript() == "" {
		return nil, detection, errors.New("shell action requires a script")
	}
	output, err := e.runShell(ctx, params, params.GetScript())
	if err != nil || output.GetExitCode() != 0 {
		if err == nil {
			err = fmt.Errorf("shell exited with status %d", output.GetExitCode())
		}
		output = outputWithError(output, err)
		return output, detection, err
	}
	if params.GetDetectionScript() == "" {
		return output, nil, nil
	}
	verified, err := e.runShell(ctx, params, params.GetDetectionScript())
	if err != nil {
		verified = outputWithError(verified, err)
		return output, verified, err
	}
	if verified.GetExitCode() != 0 {
		err := fmt.Errorf("shell remediation verification exited with status %d", verified.GetExitCode())
		verified = outputWithError(verified, err)
		return output, verified, err
	}
	return output, verified, nil
}

func outputWithError(output *pb.CommandOutput, err error) *pb.CommandOutput {
	if output == nil {
		output = &pb.CommandOutput{}
	}
	if output.GetStderr() == "" {
		output.Stderr = err.Error()
	}
	return output
}

func (e *Executor) runShell(ctx context.Context, params *pb.ShellActionParams, script string) (*pb.CommandOutput, error) {
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
