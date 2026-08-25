package executor

import (
	"context"
	"errors"
	"strings"
	"testing"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/sdk/pkg"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

func TestExecutePackage_RejectsNilParams(t *testing.T) {
	e := NewExecutor(nil)
	_, changed, err := e.executePackage(context.Background(), nil, pb.DesiredState_DESIRED_STATE_PRESENT)
	if err == nil {
		t.Fatal("expected error for nil params, got nil")
	}
	if changed {
		t.Error("changed must be false when params are nil")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("error should mention 'required', got %q", err)
	}
}

func TestExecutePackage_FailsWhenNoPackageManager(t *testing.T) {
	e := NewExecutor(nil)
	params := &pb.PackageParams{Name: "curl"}
	_, changed, err := e.executePackage(context.Background(), params, pb.DesiredState_DESIRED_STATE_PRESENT)
	if err == nil {
		t.Fatal("expected error when no package manager is available, got nil")
	}
	if changed {
		t.Error("changed must be false when no package manager exists")
	}
}

func TestExecutePackage_RejectsUnknownDesiredState(t *testing.T) {

	r, err := sysexec.NewRunner(sysexec.Direct)
	if err != nil {
		t.Fatalf("build direct runner: %v", err)
	}
	mgr, err := pkg.New(pkg.Apt, r)
	if err != nil {
		t.Fatalf("build apt manager: %v", err)
	}
	e := &Executor{pkgManager: mgr, pkgBackend: pkg.Apt}
	params := &pb.PackageParams{Name: "curl"}
	_, changed, err := e.executePackage(context.Background(), params, pb.DesiredState(999))
	if err == nil {
		t.Fatal("expected error for unknown desired state, got nil")
	}
	if changed {
		t.Error("changed must be false for unknown state")
	}
}

func TestExecutePackage_ContextCancelledBeforeDispatch(t *testing.T) {
	r, err := sysexec.NewRunner(sysexec.Direct)
	if err != nil {
		t.Fatalf("build direct runner: %v", err)
	}
	mgr, err := pkg.New(pkg.Apt, r)
	if err != nil {
		t.Fatalf("build apt manager: %v", err)
	}
	e := &Executor{pkgManager: mgr, pkgBackend: pkg.Apt}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	params := &pb.PackageParams{Name: "curl"}
	_, changed, err := e.executePackage(ctx, params, pb.DesiredState_DESIRED_STATE_PRESENT)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
	if changed {
		t.Error("changed must be false when context is cancelled")
	}
}

func TestGetPackageNameForManager_FallbackToName(t *testing.T) {
	tests := []struct {
		name     string
		backend  pkg.Backend
		params   *pb.PackageParams
		expected string
	}{
		{
			name:     "apt with apt-specific name",
			backend:  pkg.Apt,
			params:   &pb.PackageParams{Name: "curl", AptName: "libcurl4"},
			expected: "libcurl4",
		},
		{
			name:     "apt without apt-specific name falls back to Name",
			backend:  pkg.Apt,
			params:   &pb.PackageParams{Name: "curl", DnfName: "libcurl"},
			expected: "curl",
		},
		{
			name:     "dnf with dnf-specific name",
			backend:  pkg.Dnf,
			params:   &pb.PackageParams{Name: "curl", DnfName: "libcurl"},
			expected: "libcurl",
		},
		{
			name:     "dnf without dnf-specific falls back to Name",
			backend:  pkg.Dnf,
			params:   &pb.PackageParams{Name: "curl", AptName: "libcurl4"},
			expected: "curl",
		},
		{
			name:     "dnf5 uses dnf-specific name",
			backend:  pkg.Dnf5,
			params:   &pb.PackageParams{Name: "curl", DnfName: "libcurl"},
			expected: "libcurl",
		},
		{
			name:     "pacman falls back to Name when no pacman-specific",
			backend:  pkg.Pacman,
			params:   &pb.PackageParams{Name: "curl", AptName: "libcurl4"},
			expected: "curl",
		},
		{
			name:     "zypper falls back to Name when no zypper-specific",
			backend:  pkg.Zypper,
			params:   &pb.PackageParams{Name: "curl", AptName: "libcurl4"},
			expected: "curl",
		},
		{
			name:     "empty name returns empty (caller handles skip)",
			backend:  pkg.Apt,
			params:   &pb.PackageParams{Name: ""},
			expected: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Executor{pkgBackend: tt.backend}
			got := e.getPackageNameForManager(tt.params)
			if got != tt.expected {
				t.Errorf("getPackageNameForManager() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestIsPackagePinned_NilManagerFailsClosed(t *testing.T) {
	e := &Executor{}
	_, err := e.isPackagePinned(context.Background(), nil, "curl")
	if err == nil {
		t.Fatal("expected error for nil manager, got nil")
	}
}

func TestPinPackage_NilManagerFailsClosed(t *testing.T) {
	e := &Executor{}
	_, err := e.pinPackage(context.Background(), nil, "curl")
	if err == nil {
		t.Fatal("expected error for nil manager, got nil")
	}
}

func TestUnpinPackage_NilManagerFailsClosed(t *testing.T) {
	e := &Executor{}
	_, err := e.unpinPackage(context.Background(), nil, "curl")
	if err == nil {
		t.Fatal("expected error for nil manager, got nil")
	}
}

func TestPackageResult_CommandNeverRan(t *testing.T) {
	result := sysexec.Result{ExitCode: 0, Stdout: "", Stderr: ""}
	runnerErr := errors.New("exec: fork/exec: no such file or directory")
	out, changed, err := packageResult(result, runnerErr)
	if err == nil {
		t.Fatal("expected error from packageResult when runner fails, got nil")
	}
	if changed {
		t.Error("changed must be false when the command never ran")
	}
	if out.ExitCode != 1 {
		t.Errorf("exit code should be synthesised to 1 (runner failure), got %d", out.ExitCode)
	}
	if out.Stderr == "" {
		t.Error("stderr should carry the runner error message when the command never ran")
	}
}

func TestPackageResult_NonZeroExitIsError(t *testing.T) {
	result := sysexec.Result{ExitCode: 100, Stdout: "stdout", Stderr: "command not found"}
	out, changed, err := packageResult(result, errors.New("command failed"))
	if err == nil {
		t.Fatal("expected error for non-zero exit, got nil")
	}
	if changed {
		t.Error("changed must be false when the command fails")
	}
	if out.ExitCode != 100 {
		t.Errorf("exit code must be the command's real exit code 100, got %d", out.ExitCode)
	}
}

func TestPackageResult_Success(t *testing.T) {
	result := sysexec.Result{ExitCode: 0, Stdout: "installed ok\n", Stderr: ""}
	out, changed, err := packageResult(result, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !changed {
		t.Error("changed must be true on success")
	}
	if out.ExitCode != 0 {
		t.Errorf("exit code must be 0, got %d", out.ExitCode)
	}
}
