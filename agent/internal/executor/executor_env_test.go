package executor

import (
	"context"
	"errors"
	"strings"
	"testing"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

func TestRunShellScript_RejectsBlocklistedEnvVar(t *testing.T) {
	e := NewExecutor(nil)
	ctx := context.Background()

	const bogusInterp = "/nonexistent/cadestro-ws17a-interp"

	for _, name := range []string{"LD_PRELOAD", "PATH", "LD_LIBRARY_PATH"} {
		t.Run("rejects "+name, func(t *testing.T) {
			params := &pb.ShellParams{
				Interpreter: bogusInterp,
				RunAsRoot:   true,
				Environment: map[string]string{name: "/tmp/evil"},
			}
			out, err := e.runShellScript(ctx, params, "echo hi")
			if err == nil {
				t.Fatalf("runShellScript with %s = nil error, want rejection before exec", name)
			}
			if !strings.Contains(err.Error(), "is not allowed") {
				t.Errorf("error = %q, want the env allow-list rejection (%q) — a different error means exec ran first", err, name)
			}
			if out != nil {
				t.Errorf("output must be nil on a rejected env var, got %v", out)
			}
		})
	}

	t.Run("allows MYAPP_FLAG past the gate", func(t *testing.T) {
		params := &pb.ShellParams{
			Interpreter: bogusInterp,
			RunAsRoot:   true,
			Environment: map[string]string{"MYAPP_FLAG": "1"},
		}
		_, err := e.runShellScript(ctx, params, "echo hi")
		if err != nil && strings.Contains(err.Error(), "is not allowed") {
			t.Errorf("MYAPP_FLAG was rejected by the env gate (%v); an application variable must pass", err)
		}
	})
}

func TestRunShellScript_DoesNotInjectReservedLocaleVar(t *testing.T) {
	e := NewExecutor(nil)
	ctx := context.Background()

	const bogusInterp = "/nonexistent/cadestro-reserved-locale-interp"

	for _, name := range []string{"LC_ALL", "LANG", "NO_COLOR"} {
		t.Run("rejects "+name, func(t *testing.T) {
			params := &pb.ShellParams{
				Interpreter: bogusInterp,
				RunAsRoot:   true,
				Environment: map[string]string{name: "C.UTF-8"},
			}
			_, err := e.runShellScript(ctx, params, "echo hi")
			if err == nil {
				t.Fatalf("runShellScript setting reserved %s = nil error, want rejection before exec", name)
			}
			if !errors.Is(err, sysexec.ErrReservedEnvVar) {
				t.Errorf("error = %v, want ErrReservedEnvVar (the Runner forces LC_ALL=C/LANG=C/NO_COLOR=1)", err)
			}
		})
	}

	t.Run("allows MYAPP_FLAG past the reserved gate", func(t *testing.T) {
		params := &pb.ShellParams{
			Interpreter: bogusInterp,
			RunAsRoot:   true,
			Environment: map[string]string{"MYAPP_FLAG": "1"},
		}
		_, err := e.runShellScript(ctx, params, "echo hi")
		if errors.Is(err, sysexec.ErrReservedEnvVar) {
			t.Errorf("MYAPP_FLAG must not be treated as a reserved var, got %v", err)
		}
	})
}
