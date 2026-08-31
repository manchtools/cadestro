package executor

import (
	"context"
	"errors"
	"testing"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/sdk/pkg"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

type fakeRunner struct {
	commands []sysexec.Command
	results  []sysexec.Result
	errors   []error
}

func mustExecutor(t *testing.T, runner sysexec.Runner) *Executor {
	t.Helper()
	executor, err := NewExecutor(runner)
	if err != nil {
		t.Fatal(err)
	}
	return executor
}

func TestNewExecutorRejectsNilRunner(t *testing.T) {
	if _, err := NewExecutor(nil); err == nil {
		t.Fatal("NewExecutor(nil) returned nil error")
	}
}

func (f *fakeRunner) Run(_ context.Context, command sysexec.Command) (sysexec.Result, error) {
	f.commands = append(f.commands, command)
	index := len(f.commands) - 1
	var result sysexec.Result
	if index < len(f.results) {
		result = f.results[index]
	}
	if index < len(f.errors) {
		return result, f.errors[index]
	}
	return result, nil
}

func (f *fakeRunner) Stream(context.Context, sysexec.Command, sysexec.OutputCallback) (sysexec.Result, error) {
	return sysexec.Result{}, errors.New("unexpected stream")
}

func (f *fakeRunner) Backend() sysexec.PrivilegeBackend { return sysexec.Direct }

type fakePackageManager struct {
	pkg.Manager
	installed    bool
	version      string
	updates      bool
	installCalls int
	removeCalls  int
	updateCalls  int
	upgradeCalls int
	operationErr error
}

func (f *fakePackageManager) IsInstalled(context.Context, string) (bool, error) {
	return f.installed, f.operationErr
}

func (f *fakePackageManager) InstalledVersion(context.Context, string) (string, error) {
	return f.version, f.operationErr
}

func (f *fakePackageManager) Install(context.Context, pkg.InstallOptions, ...pkg.InstallSpec) (sysexec.Result, error) {
	f.installCalls++
	return sysexec.Result{}, f.operationErr
}

func (f *fakePackageManager) Remove(context.Context, pkg.RemoveOptions, ...string) (sysexec.Result, error) {
	f.removeCalls++
	return sysexec.Result{}, f.operationErr
}

func (f *fakePackageManager) Update(context.Context) (sysexec.Result, error) {
	f.updateCalls++
	return sysexec.Result{}, f.operationErr
}

func (f *fakePackageManager) HasUpdates(context.Context) (bool, error) {
	return f.updates, f.operationErr
}

func (f *fakePackageManager) UpgradeAll(context.Context) (sysexec.Result, error) {
	f.upgradeCalls++
	return sysexec.Result{}, f.operationErr
}

func TestComplianceShellOnlyDetects(t *testing.T) {
	runner := &fakeRunner{results: []sysexec.Result{{ExitCode: 1}}}
	executor := mustExecutor(t, runner)
	result := executor.ExecuteAction(context.Background(), &pb.Action{
		Id:           &pb.ActionId{Value: "01J0000000000000000000000A"},
		DesiredState: pb.DesiredState_DESIRED_STATE_PRESENT,
		Params: &pb.Action_Shell{Shell: &pb.ShellActionParams{
			DetectionScript: "test -f /etc/example", IsCompliance: true,
		}},
	})
	if result.GetStatus() != pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS {
		t.Fatalf("result = %s, want success", result.GetStatus())
	}
	if len(runner.commands) != 1 {
		t.Fatalf("commands = %d, want one detection command", len(runner.commands))
	}
}

func TestShellErrorsUseRelevantOutput(t *testing.T) {
	t.Run("detection", func(t *testing.T) {
		runner := &fakeRunner{errors: []error{errors.New("detection failed")}}
		result := mustExecutor(t, runner).ExecuteAction(context.Background(), &pb.Action{
			Id:     &pb.ActionId{Value: "01J0000000000000000000000A"},
			Params: &pb.Action_Shell{Shell: &pb.ShellActionParams{DetectionScript: "check", Script: "fix"}},
		})
		if result.GetDetectionOutput().GetStderr() != "run shell: detection failed" || result.GetOutput() != nil {
			t.Fatalf("result outputs = %#v, want detection stderr only", result)
		}
	})
	t.Run("verification", func(t *testing.T) {
		runner := &fakeRunner{results: []sysexec.Result{{ExitCode: 1}, {}, {}}, errors: []error{nil, nil, errors.New("verification failed")}}
		result := mustExecutor(t, runner).ExecuteAction(context.Background(), &pb.Action{
			Id:     &pb.ActionId{Value: "01J0000000000000000000000A"},
			Params: &pb.Action_Shell{Shell: &pb.ShellActionParams{DetectionScript: "check", Script: "fix"}},
		})
		if result.GetDetectionOutput().GetStderr() != "run shell: verification failed" || result.GetOutput().GetStderr() != "" {
			t.Fatalf("result outputs = %#v, want verification stderr only", result)
		}
	})
	t.Run("ordinary", func(t *testing.T) {
		runner := &fakeRunner{results: []sysexec.Result{{Stderr: "command stderr"}}, errors: []error{errors.New("command failed")}}
		result := mustExecutor(t, runner).ExecuteAction(context.Background(), &pb.Action{
			Id:     &pb.ActionId{Value: "01J0000000000000000000000A"},
			Params: &pb.Action_Shell{Shell: &pb.ShellActionParams{Script: "fix"}},
		})
		if result.GetOutput().GetStderr() != "command stderr" || result.GetDetectionOutput() != nil {
			t.Fatalf("result outputs = %#v, want ordinary output stderr", result)
		}
	})
}

func TestShellRemediatesAndVerifies(t *testing.T) {
	runner := &fakeRunner{results: []sysexec.Result{{ExitCode: 1}, {}, {}}}
	executor := mustExecutor(t, runner)
	result := executor.ExecuteAction(context.Background(), &pb.Action{
		Id:           &pb.ActionId{Value: "01J0000000000000000000000A"},
		DesiredState: pb.DesiredState_DESIRED_STATE_PRESENT,
		Params: &pb.Action_Shell{Shell: &pb.ShellActionParams{
			DetectionScript: "test -f /etc/example", Script: "touch /etc/example",
		}},
	})
	if result.GetStatus() != pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS {
		t.Fatalf("result = %s", result.GetStatus())
	}
	if len(runner.commands) != 3 {
		t.Fatalf("commands = %d, want detect, remediate, verify", len(runner.commands))
	}
}

func TestShellRejectsHijackEnvironment(t *testing.T) {
	runner := &fakeRunner{}
	executor := mustExecutor(t, runner)
	result := executor.ExecuteAction(context.Background(), &pb.Action{
		Id:           &pb.ActionId{Value: "01J0000000000000000000000A"},
		DesiredState: pb.DesiredState_DESIRED_STATE_PRESENT,
		Params: &pb.Action_Shell{Shell: &pb.ShellActionParams{
			Script: "true", Environment: map[string]string{"PATH": "/tmp"},
		}},
	})
	if result.GetStatus() != pb.ExecutionStatus_EXECUTION_STATUS_FAILED {
		t.Fatalf("status = %s, want failed", result.GetStatus())
	}
}

func TestPackageSkipsInstalledVersion(t *testing.T) {
	manager := &fakePackageManager{installed: true, version: "1.0"}
	executor := mustExecutor(t, &fakeRunner{})
	executor.pkgManager = manager
	result := executor.ExecuteAction(context.Background(), &pb.Action{
		Id:           &pb.ActionId{Value: "01J0000000000000000000000A"},
		DesiredState: pb.DesiredState_DESIRED_STATE_PRESENT,
		Params:       &pb.Action_Package{Package: &pb.PackageActionParams{Name: "example", Version: "1.0"}},
	})
	if result.GetStatus() != pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS || manager.installCalls != 0 {
		t.Fatalf("status=%s installs=%d", result.GetStatus(), manager.installCalls)
	}
}

func TestUpdateSkipsCurrentSystem(t *testing.T) {
	manager := &fakePackageManager{}
	executor := mustExecutor(t, &fakeRunner{})
	executor.pkgManager = manager
	result := executor.ExecuteAction(context.Background(), &pb.Action{
		Id:           &pb.ActionId{Value: "01J0000000000000000000000A"},
		DesiredState: pb.DesiredState_DESIRED_STATE_PRESENT,
		Params:       &pb.Action_Update{Update: &pb.UpdateActionParams{}},
	})
	if result.GetStatus() != pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS || manager.upgradeCalls != 0 {
		t.Fatalf("status=%s upgrades=%d", result.GetStatus(), manager.upgradeCalls)
	}
}
