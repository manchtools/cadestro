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
	executor := NewExecutor(runner)
	result := executor.ExecuteAction(context.Background(), &pb.Action{
		Id:           &pb.ActionId{Value: "01J0000000000000000000000A"},
		DesiredState: pb.DesiredState_DESIRED_STATE_PRESENT,
		Params: &pb.Action_Shell{Shell: &pb.ShellActionParams{
			DetectionScript: "test -f /etc/example", IsCompliance: true,
		}},
	})
	if result.GetStatus() != pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS || result.GetCompliant() {
		t.Fatalf("result = %s compliant=%v, want success and non-compliant", result.GetStatus(), result.GetCompliant())
	}
	if len(runner.commands) != 1 {
		t.Fatalf("commands = %d, want one detection command", len(runner.commands))
	}
}

func TestShellRemediatesAndVerifies(t *testing.T) {
	runner := &fakeRunner{results: []sysexec.Result{{ExitCode: 1}, {}, {}}}
	executor := NewExecutor(runner)
	result := executor.ExecuteAction(context.Background(), &pb.Action{
		Id:           &pb.ActionId{Value: "01J0000000000000000000000A"},
		DesiredState: pb.DesiredState_DESIRED_STATE_PRESENT,
		Params: &pb.Action_Shell{Shell: &pb.ShellActionParams{
			DetectionScript: "test -f /etc/example", Script: "touch /etc/example",
		}},
	})
	if result.GetStatus() != pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS || !result.GetChanged() || !result.GetCompliant() {
		t.Fatalf("result = %s changed=%v compliant=%v", result.GetStatus(), result.GetChanged(), result.GetCompliant())
	}
	if len(runner.commands) != 3 {
		t.Fatalf("commands = %d, want detect, remediate, verify", len(runner.commands))
	}
}

func TestShellRejectsHijackEnvironment(t *testing.T) {
	runner := &fakeRunner{}
	executor := NewExecutor(runner)
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
	executor := NewExecutor(&fakeRunner{})
	executor.pkgManager = manager
	result := executor.ExecuteAction(context.Background(), &pb.Action{
		Id:           &pb.ActionId{Value: "01J0000000000000000000000A"},
		DesiredState: pb.DesiredState_DESIRED_STATE_PRESENT,
		Params:       &pb.Action_Package{Package: &pb.PackageActionParams{Name: "example", Version: "1.0"}},
	})
	if result.GetStatus() != pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS || result.GetChanged() || manager.installCalls != 0 {
		t.Fatalf("status=%s changed=%v installs=%d", result.GetStatus(), result.GetChanged(), manager.installCalls)
	}
}

func TestUpdateSkipsCurrentSystem(t *testing.T) {
	manager := &fakePackageManager{}
	executor := NewExecutor(&fakeRunner{})
	executor.pkgManager = manager
	result := executor.ExecuteAction(context.Background(), &pb.Action{
		Id:           &pb.ActionId{Value: "01J0000000000000000000000A"},
		DesiredState: pb.DesiredState_DESIRED_STATE_PRESENT,
		Params:       &pb.Action_Update{Update: &pb.UpdateActionParams{}},
	})
	if result.GetStatus() != pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS || result.GetChanged() || manager.upgradeCalls != 0 {
		t.Fatalf("status=%s changed=%v upgrades=%d", result.GetStatus(), result.GetChanged(), manager.upgradeCalls)
	}
}
