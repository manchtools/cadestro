package exec_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

func runnerForChildPathTest(t *testing.T) sysexec.Runner {
	t.Helper()
	r, err := sysexec.NewRunner(sysexec.Direct)
	if err != nil {
		t.Fatalf("NewRunner(Direct): %v", err)
	}
	return r
}

func TestRunnerChildPath_AppliesCuratedPath(t *testing.T) {
	res, err := runnerForChildPathTest(t).Run(context.Background(), sysexec.Command{
		Name: "sh", Args: []string{"-c", "printf %s \"$PATH\""},
		Env:       []string{"MARKER=1"},
		ChildPath: "/curated/bin:/usr/bin",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := strings.TrimSpace(res.Stdout); got != "/curated/bin:/usr/bin" {
		t.Fatalf("child PATH = %q, want the curated value", got)
	}
}

func TestRunnerChildPath_EmptyEnvStillIsolates(t *testing.T) {
	t.Setenv("CADESTRO_PARENT_SECRET", "leaked-from-root")

	res, err := runnerForChildPathTest(t).Run(context.Background(), sysexec.Command{
		Name: "sh", Args: []string{"-c", "printf %s \"$PATH|${CADESTRO_PARENT_SECRET:-unset}\""},
		ChildPath: "/curated/bin",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := strings.TrimSpace(res.Stdout); got != "/curated/bin|unset" {
		t.Fatalf("empty-env curated run leaked the parent environment: got %q, want %q",
			got, "/curated/bin|unset")
	}
}

func TestRunnerChildPath_StillRejectsBlockedEnv(t *testing.T) {
	_, err := runnerForChildPathTest(t).Run(context.Background(), sysexec.Command{
		Name: "true", Env: []string{"LD_PRELOAD=/tmp/evil.so"}, ChildPath: "/usr/bin",
	})
	if !errors.Is(err, sysexec.ErrBlockedEnvVar) {
		t.Fatalf("err = %v, want ErrBlockedEnvVar", err)
	}
}
