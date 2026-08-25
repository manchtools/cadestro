package exec_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

func runnerForEnvTest(t *testing.T) sysexec.Runner {
	t.Helper()
	r, err := sysexec.NewRunner(sysexec.Direct)
	if err != nil {
		t.Fatalf("NewRunner(Direct): %v", err)
	}
	return r
}

func TestRunner_RejectsBlockedEnvVars(t *testing.T) {
	blocked := []string{
		"LD_PRELOAD=/tmp/evil.so",
		"PATH=/tmp/attacker",
		"BASH_ENV=/tmp/evil.sh",
		"GCONV_PATH=/tmp/evil",
		"LD_LIBRARY_PATH=/tmp/evil",
	}
	r := runnerForEnvTest(t)
	for _, e := range blocked {
		_, err := r.Run(context.Background(), sysexec.Command{Name: "true", Env: []string{e}})
		if !errors.Is(err, sysexec.ErrBlockedEnvVar) {
			t.Errorf("Run with Env %q err = %v, want ErrBlockedEnvVar", e, err)
		}
	}
}

func TestRunner_RejectsMalformedEnvEntry(t *testing.T) {
	r := runnerForEnvTest(t)
	_, err := r.Run(context.Background(), sysexec.Command{Name: "true", Env: []string{"NOTKEY_EQUALS_VALUE"}})
	if !errors.Is(err, sysexec.ErrInvalidEnvVar) {
		t.Fatalf("err = %v, want ErrInvalidEnvVar", err)
	}
}

func TestRunner_AcceptsSafeEnvVar(t *testing.T) {
	r := runnerForEnvTest(t)
	res, err := r.Run(context.Background(), sysexec.Command{
		Name: "sh", Args: []string{"-c", "printf %s \"$CADESTRO_AUDIT_TEST_MARKER\""},
		Env: []string{"CADESTRO_AUDIT_TEST_MARKER=ok"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := strings.TrimSpace(res.Stdout); got != "ok" {
		t.Fatalf("child did not see the safe env var: stdout=%q (trimmed=%q)", res.Stdout, got)
	}
}
