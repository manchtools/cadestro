package executor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/manchtools/cadestro/agent/internal/store"
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/sdk/pkg"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

const maxScriptSize = 1 << 20

const maxFileContentSize = 10 << 20

const defaultScriptTimeout int32 = 3600

const defaultPackageTimeout int32 = 1800

func defaultTimeoutForAction(actionType pb.ActionType, requested int32) int32 {
	if requested > 0 {
		return requested
	}
	switch actionType {
	case pb.ActionType_ACTION_TYPE_SHELL, pb.ActionType_ACTION_TYPE_SCRIPT_RUN:
		return defaultScriptTimeout
	case pb.ActionType_ACTION_TYPE_PACKAGE, pb.ActionType_ACTION_TYPE_UPDATE:
		return defaultPackageTimeout
	default:
		return 0
	}
}

type Executor struct {
	httpClient *http.Client
	pkgManager pkg.Manager
	pkgBackend pkg.Backend

	runner       sysexec.Runner
	deps         executorDeps
	depsOnce     sync.Once
	logger       *slog.Logger
	mu           sync.RWMutex
	luksKeyStore LuksKeyStore
	lpsStore     LpsPasswordStore
	store        *store.Store
	actionStore  ActionStore
	updateCfg    *AgentUpdateConfig

	agentUpdateExecutedMu sync.Mutex
	agentUpdateExecuted   bool

	luksTimestampFailMu    sync.Mutex
	luksTimestampFailCount map[string]int

	now func() time.Time

	repairFS func(ctx context.Context) bool
}

func (e *Executor) pkgManagerForCtx(ctx context.Context) pkg.Manager {
	if ctx.Err() != nil {
		return nil
	}
	return e.pkgManager
}

func NewExecutor(runner sysexec.Runner) *Executor {
	logger := slog.Default()
	var (
		mgr     pkg.Manager
		backend pkg.Backend
	)

	deps := newExecutorDeps(runner)
	switch {
	case runner == nil:
		logger.Warn("no privilege runner provided; package actions will fail")
	default:

		if backends := pkg.Detect(); len(backends) == 0 {
			logger.Warn("no supported package manager detected; package actions will fail")
		} else {
			backend = backends[0]
			m, err := pkg.New(backend, runner)
			if err != nil {
				logger.Warn("failed to build package manager; package actions will fail",
					"backend", backend.String(), "error", err)
			} else {
				mgr = m
				logger.Info("package manager detected", "manager", backend.String())
			}
		}
	}
	e := &Executor{
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
		pkgManager: mgr,
		pkgBackend: backend,
		runner:     runner,
		deps:       deps,
		logger:     logger,
		now:        time.Now,
	}
	return e
}

func (e *Executor) SetLuksKeyStore(ks LuksKeyStore) {
	e.mu.Lock()
	e.luksKeyStore = ks
	e.mu.Unlock()
}

func (e *Executor) SetLpsPasswordStore(ps LpsPasswordStore) {
	e.mu.Lock()
	e.lpsStore = ps
	e.mu.Unlock()
}

func (e *Executor) getLpsPasswordStore() LpsPasswordStore {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lpsStore
}

func (e *Executor) SetStore(s *store.Store) {
	e.mu.Lock()
	e.store = s
	e.mu.Unlock()
}

func (e *Executor) SetUpdateConfig(cfg *AgentUpdateConfig) {
	e.mu.Lock()
	e.updateCfg = cfg
	e.mu.Unlock()
}

func (e *Executor) SetActionStore(as ActionStore) {
	e.mu.Lock()
	e.actionStore = as
	e.mu.Unlock()
}

func (e *Executor) getLuksKeyStore() LuksKeyStore {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.luksKeyStore
}

func (e *Executor) getStore() *store.Store {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.store
}

func (e *Executor) getActionStore() ActionStore {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.actionStore
}

func (e *Executor) ExecuteAction(ctx context.Context, action *pb.Action) *pb.ActionResult {
	env := action
	start := e.now()

	result := &pb.ActionResult{
		ActionId: env.GetId(),
		Status:   pb.ExecutionStatus_EXECUTION_STATUS_RUNNING,
		Changed:  true,
	}

	parentCtx := ctx
	timeout := defaultTimeoutForAction(env.Type, env.GetTimeoutSeconds())
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
	}

	var execErr error
	var output *pb.CommandOutput

	switch env.Type {
	case pb.ActionType_ACTION_TYPE_PACKAGE:
		var changed bool
		output, changed, execErr = e.executePackage(ctx, env.GetPackage(), env.DesiredState)
		result.Changed = changed
	case pb.ActionType_ACTION_TYPE_UPDATE:
		var changed bool
		output, changed, execErr = e.executeUpdate(ctx, env.GetUpdate())
		result.Changed = changed
	case pb.ActionType_ACTION_TYPE_APP_IMAGE:
		var changed bool
		output, changed, execErr = e.executeAppImage(ctx, env.GetApp(), env.DesiredState)
		result.Changed = changed
	case pb.ActionType_ACTION_TYPE_FLATPAK:
		var changed bool
		output, changed, execErr = e.executeFlatpak(ctx, env.GetFlatpak(), env.DesiredState)
		result.Changed = changed
	case pb.ActionType_ACTION_TYPE_DEB:
		var changed bool
		output, changed, execErr = e.executeDeb(ctx, env.GetApp(), env.DesiredState)
		result.Changed = changed
	case pb.ActionType_ACTION_TYPE_RPM:
		var changed bool
		output, changed, execErr = e.executeRpm(ctx, env.GetApp(), env.DesiredState)
		result.Changed = changed
	case pb.ActionType_ACTION_TYPE_SHELL, pb.ActionType_ACTION_TYPE_SCRIPT_RUN:
		var detectionOutput *pb.CommandOutput
		var changed bool
		output, detectionOutput, changed, execErr = e.executeShell(ctx, env.GetShell())
		result.Changed = changed
		result.DetectionOutput = detectionOutput
		if env.GetShell().GetIsCompliance() {
			result.Compliant = detectionOutput != nil && detectionOutput.ExitCode == 0 && execErr == nil
		}
	case pb.ActionType_ACTION_TYPE_SERVICE:
		var changed bool
		output, changed, execErr = e.executeService(ctx, env.GetService())
		result.Changed = changed
	case pb.ActionType_ACTION_TYPE_FILE:
		var changed bool
		output, changed, execErr = e.executeFile(ctx, env.GetFile(), env.DesiredState)
		result.Changed = changed
	case pb.ActionType_ACTION_TYPE_DIRECTORY:
		var changed bool
		output, changed, execErr = e.executeDirectory(ctx, env.GetDirectory(), env.DesiredState)
		result.Changed = changed
	case pb.ActionType_ACTION_TYPE_REPOSITORY:
		var changed bool
		output, changed, execErr = e.executeRepository(ctx, env.GetRepository(), env.DesiredState)
		result.Changed = changed
	case pb.ActionType_ACTION_TYPE_USER:
		var changed bool
		var metadata map[string]string
		output, changed, metadata, execErr = e.executeUser(ctx, env.GetUser(), env.DesiredState, envActionID(env))
		result.Changed = changed
		if len(metadata) > 0 {
			result.Metadata = metadata
		}
	case pb.ActionType_ACTION_TYPE_GROUP:
		var changed bool
		output, changed, execErr = e.executeGroup(ctx, env.GetGroup(), env.DesiredState)
		result.Changed = changed
	case pb.ActionType_ACTION_TYPE_SSH:
		var changed bool
		output, changed, execErr = e.executeSsh(ctx, env.GetSsh(), env.DesiredState, envActionID(env))
		result.Changed = changed
	case pb.ActionType_ACTION_TYPE_SSHD:
		var changed bool
		output, changed, execErr = e.executeSshd(ctx, env.GetSshd(), env.DesiredState, envActionID(env))
		result.Changed = changed
	case pb.ActionType_ACTION_TYPE_ADMIN_POLICY:
		var changed bool
		output, changed, execErr = e.executeSudo(ctx, env.GetAdminPolicy(), env.DesiredState, envActionID(env))
		result.Changed = changed
	case pb.ActionType_ACTION_TYPE_LPS:
		var changed bool
		var metadata map[string]string
		output, changed, metadata, execErr = e.executeLps(ctx, env.GetLps(), env.DesiredState, envActionID(env))
		result.Changed = changed
		if len(metadata) > 0 {
			result.Metadata = metadata
		}
	case pb.ActionType_ACTION_TYPE_ENCRYPTION:
		var changed bool

		output, changed, _, execErr = e.executeLuksAction(ctx, env.GetEncryption(), env.DesiredState, envActionID(env))
		result.Changed = changed
	case pb.ActionType_ACTION_TYPE_WIFI:
		var changed bool
		output, changed, execErr = e.executeWifiAction(ctx, env.GetWifi(), env.DesiredState, envActionID(env))
		result.Changed = changed
	case pb.ActionType_ACTION_TYPE_AGENT_UPDATE:
		var changed bool
		output, changed, execErr = e.executeAgentUpdate(ctx, env.GetAgentUpdate())
		result.Changed = changed
	default:
		execErr = fmt.Errorf("unsupported action type: %v", env.Type)
	}

	result.Output = output
	completed := e.now()
	result.CompletedAt = timestamppb.New(completed)
	result.Duration = durationpb.New(completed.Sub(start))

	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		result.Status = pb.ExecutionStatus_EXECUTION_STATUS_TIMEOUT

		if errors.Is(parentCtx.Err(), context.DeadlineExceeded) {
			result.Error = "action deadline exceeded (parent context)"
		} else {
			result.Error = fmt.Sprintf("action timed out after %d seconds", timeout)
		}
	case errors.Is(ctx.Err(), context.Canceled):
		result.Status = pb.ExecutionStatus_EXECUTION_STATUS_FAILED
		result.Error = "action cancelled"
	case errors.Is(execErr, errNotApplicable):

		result.Status = pb.ExecutionStatus_EXECUTION_STATUS_NOT_APPLICABLE
		result.Error = execErr.Error()
	case execErr != nil:
		result.Status = pb.ExecutionStatus_EXECUTION_STATUS_FAILED
		result.Error = execErr.Error()
	default:
		result.Status = pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS
	}

	if result.Status == pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS {
		if env.Type == pb.ActionType_ACTION_TYPE_SHELL || env.Type == pb.ActionType_ACTION_TYPE_SCRIPT_RUN {
			if result.DetectionOutput != nil && result.DetectionOutput.ExitCode != 0 {
				result.Status = pb.ExecutionStatus_EXECUTION_STATUS_FAILED
				result.Error = fmt.Sprintf("script exited with code %d", result.DetectionOutput.ExitCode)
			} else if result.Output != nil && result.Output.ExitCode != 0 {
				result.Status = pb.ExecutionStatus_EXECUTION_STATUS_FAILED
				result.Error = fmt.Sprintf("script exited with code %d", result.Output.ExitCode)
			}
		}
	}

	return result
}

func (e *Executor) runShellScript(ctx context.Context, params *pb.ShellParams, script string) (*pb.CommandOutput, error) {
	interpreter := params.Interpreter
	if interpreter == "" {
		interpreter = "/bin/sh"
	}

	envVars := []string{
		"HOME=" + os.Getenv("HOME"),
		"USER=" + os.Getenv("USER"),
	}
	for k, v := range params.Environment {
		if !sysexec.IsAllowedEnvVar(k) {
			return nil, fmt.Errorf("environment variable %q is not allowed", k)
		}
		envVars = append(envVars, fmt.Sprintf("%s=%s", k, v))
	}

	args := []string{"-c", script}
	if params.RunAsRoot {
		r, err := e.runnerOrDirect().Run(ctx, sysexec.Command{
			Name:     interpreter,
			Args:     args,
			Env:      envVars,
			Dir:      params.WorkingDirectory,
			Escalate: true,
		})
		return toOutput(&r), err
	}

	return e.runShellScriptPerUser(ctx, params, interpreter, args, envVars)
}

func (e *Executor) runShellScriptPerUser(ctx context.Context, params *pb.ShellParams, interpreter string, args []string, envVars []string) (*pb.CommandOutput, error) {
	sessions, err := e.deps.desktop.ActiveSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("enumerate active desktop sessions: %w", err)
	}
	if len(sessions) == 0 {
		e.logger.Warn("shell RunAsRoot=false: no active desktop sessions; per-user run deferred until a user signs in")
		return &pb.CommandOutput{
			ExitCode: 0,
			Stdout:   "skipped: no signed-in desktop users; will run again on next reconciliation",
		}, nil
	}

	extraEnv := stripHomeAndUser(envVars)

	merged := &pb.CommandOutput{}
	var firstFailure error
	for _, s := range sessions {
		userPrefix := "[user=" + s.Username + "] "
		out, runErr := e.runAsUser(ctx, s, extraEnv, params.WorkingDirectory, interpreter, args)
		if out != nil {
			if out.Stdout != "" {
				merged.Stdout += userPrefix + out.Stdout
				if !strings.HasSuffix(out.Stdout, "\n") {
					merged.Stdout += "\n"
				}
			}
			if out.Stderr != "" {
				merged.Stderr += userPrefix + out.Stderr
				if !strings.HasSuffix(out.Stderr, "\n") {
					merged.Stderr += "\n"
				}
			}
			if out.ExitCode != 0 && merged.ExitCode == 0 {
				merged.ExitCode = out.ExitCode
			}
		}
		if runErr != nil && firstFailure == nil {
			firstFailure = fmt.Errorf("user %s: %w", s.Username, runErr)
		}
	}
	return merged, firstFailure
}

func stripHomeAndUser(envVars []string) []string {
	out := make([]string, 0, len(envVars))
	for _, kv := range envVars {
		if strings.HasPrefix(kv, "HOME=") || strings.HasPrefix(kv, "USER=") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func (e *Executor) executeShell(ctx context.Context, params *pb.ShellParams) (*pb.CommandOutput, *pb.CommandOutput, bool, error) {
	if params == nil {
		return nil, nil, false, fmt.Errorf("shell params required")
	}
	if len(params.Script) > maxScriptSize {
		return nil, nil, false, fmt.Errorf("script exceeds maximum size (%d bytes)", maxScriptSize)
	}
	if len(params.DetectionScript) > maxScriptSize {
		return nil, nil, false, fmt.Errorf("detection script exceeds maximum size (%d bytes)", maxScriptSize)
	}

	if params.GetIsCompliance() {
		if params.DetectionScript == "" {
			return nil, nil, false, fmt.Errorf("compliance action requires a non-empty detection script; refusing to run its execution script")
		}
		e.logger.Debug("compliance mode: running detection script only")
		detectionOutput, err := e.runShellScript(ctx, params, params.DetectionScript)
		if err != nil {
			return nil, detectionOutput, false, err
		}
		return nil, detectionOutput, false, nil
	}

	if params.DetectionScript == "" {
		if params.Script == "" {
			return nil, nil, false, fmt.Errorf("at least one of script or detection_script is required")
		}
		output, err := e.runShellScript(ctx, params, params.Script)
		return output, nil, true, err
	}

	e.logger.Debug("running detection script")
	detectionOutput, err := e.runShellScript(ctx, params, params.DetectionScript)
	if err != nil {
		return nil, detectionOutput, false, fmt.Errorf("detection script error: %w", err)
	}

	if detectionOutput.ExitCode == 0 {
		e.logger.Debug("detection script passed (exit 0), system is compliant")
		return nil, detectionOutput, false, nil
	}

	if params.Script == "" {
		e.logger.Debug("detection script failed (non-zero), no execution script — reporting non-compliant")
		return nil, detectionOutput, false, nil
	}

	e.logger.Debug("detection script failed (non-zero), running remediation script")
	execOutput, execErr := e.runShellScript(ctx, params, params.Script)
	if execErr != nil {
		return execOutput, detectionOutput, true, execErr
	}

	e.logger.Debug("re-running detection script to verify remediation")
	verifyOutput, verifyErr := e.runShellScript(ctx, params, params.DetectionScript)
	if verifyErr != nil {
		return execOutput, verifyOutput, true, fmt.Errorf("verification detection script error: %w", verifyErr)
	}

	if verifyOutput.ExitCode != 0 {
		return execOutput, verifyOutput, true, fmt.Errorf("remediation did not resolve the issue (detection still exits %d)", verifyOutput.ExitCode)
	}

	e.logger.Debug("verification passed, remediation successful")
	return execOutput, verifyOutput, true, nil
}
