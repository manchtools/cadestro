package executor

import (
	"context"
	"errors"
	"reflect"
	"strings"
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

func executorWithBackend(t *testing.T, backend pkg.Backend, runner *fakeRunner) *Executor {
	t.Helper()
	manager, err := pkg.New(backend, runner)
	if err != nil {
		t.Fatal(err)
	}
	executor := mustExecutor(t, runner)
	executor.pkgManager = manager
	return executor
}

func commandLines(commands []sysexec.Command) []string {
	lines := make([]string, len(commands))
	for i, command := range commands {
		lines[i] = strings.TrimSpace(command.Name + " " + strings.Join(command.Args, " "))
	}
	return lines
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
	installCalls int
	removeCalls  int
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

func TestPackageInstallUsesPacmanWithoutPartialUpgrade(t *testing.T) {
	runner := &fakeRunner{results: []sysexec.Result{{ExitCode: 1}, {}}}
	result := executorWithBackend(t, pkg.Pacman, runner).ExecuteAction(context.Background(), &pb.Action{
		Id:           &pb.ActionId{Value: "01J0000000000000000000000A"},
		DesiredState: pb.DesiredState_DESIRED_STATE_PRESENT,
		Params:       &pb.Action_Package{Package: &pb.PackageActionParams{Name: "example"}},
	})
	if result.GetStatus() != pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS {
		t.Fatalf("status = %s, want success", result.GetStatus())
	}
	want := []string{"pacman -Q example", "pacman -S --noconfirm --needed example"}
	if got := commandLines(runner.commands); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %q, want %q", got, want)
	}
}

func TestUpdateUsesBackendUpgradeSequence(t *testing.T) {
	tests := []struct {
		name    string
		backend pkg.Backend
		want    []string
	}{
		{"apt", pkg.Apt, []string{"apt-get update", "apt-get upgrade -y -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold"}},
		{"dnf", pkg.Dnf, []string{"dnf check-update", "dnf upgrade -y"}},
		{"dnf5", pkg.Dnf5, []string{"dnf5 check-update", "dnf5 upgrade -y"}},
		{"pacman", pkg.Pacman, []string{"pacman -Syu --noconfirm"}},
		{"zypper", pkg.Zypper, []string{"zypper --non-interactive refresh", "zypper --non-interactive update"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &fakeRunner{}
			result := executorWithBackend(t, test.backend, runner).ExecuteAction(context.Background(), &pb.Action{
				Id:           &pb.ActionId{Value: "01J0000000000000000000000A"},
				DesiredState: pb.DesiredState_DESIRED_STATE_PRESENT,
				Params:       &pb.Action_Update{Update: &pb.UpdateActionParams{}},
			})
			if result.GetStatus() != pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS {
				t.Fatalf("status = %s, want success", result.GetStatus())
			}
			if got := commandLines(runner.commands); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("commands = %q, want %q", got, test.want)
			}
		})
	}
}

func TestPackageInstallPreservesRefreshFailure(t *testing.T) {
	runner := &fakeRunner{results: []sysexec.Result{{ExitCode: 1}, {ExitCode: 2}}}
	result := executorWithBackend(t, pkg.Apt, runner).ExecuteAction(context.Background(), &pb.Action{
		Id:           &pb.ActionId{Value: "01J0000000000000000000000A"},
		DesiredState: pb.DesiredState_DESIRED_STATE_PRESENT,
		Params:       &pb.Action_Package{Package: &pb.PackageActionParams{Name: "example"}},
	})
	if result.GetStatus() != pb.ExecutionStatus_EXECUTION_STATUS_FAILED {
		t.Fatalf("status = %s, want failed", result.GetStatus())
	}
	want := []string{"dpkg -s example", "apt-get update"}
	if got := commandLines(runner.commands); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %q, want %q", got, want)
	}
}

func TestUpdatePreservesRefreshFailure(t *testing.T) {
	runner := &fakeRunner{results: []sysexec.Result{{ExitCode: 2}}}
	result := executorWithBackend(t, pkg.Dnf, runner).ExecuteAction(context.Background(), &pb.Action{
		Id:           &pb.ActionId{Value: "01J0000000000000000000000A"},
		DesiredState: pb.DesiredState_DESIRED_STATE_PRESENT,
		Params:       &pb.Action_Update{Update: &pb.UpdateActionParams{}},
	})
	if result.GetStatus() != pb.ExecutionStatus_EXECUTION_STATUS_FAILED {
		t.Fatalf("status = %s, want failed", result.GetStatus())
	}
	want := []string{"dnf check-update"}
	if got := commandLines(runner.commands); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %q, want %q", got, want)
	}
}
