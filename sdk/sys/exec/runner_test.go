package exec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewRunner_RejectsZeroAndUnknown(t *testing.T) {
	for _, b := range []PrivilegeBackend{0, PrivilegeBackend(-1), PrivilegeBackend(99)} {
		r, err := NewRunner(b)
		if !errors.Is(err, ErrUnknownBackend) {
			t.Errorf("NewRunner(%d) err = %v, want ErrUnknownBackend", b, err)
		}
		if r != nil {
			t.Errorf("NewRunner(%d) returned a non-nil runner on error", b)
		}
	}
}

func TestNewRunner_AcceptsImplementedBackends(t *testing.T) {
	for _, b := range []PrivilegeBackend{Sudo, Doas, Direct} {
		r, err := NewRunner(b)
		if err != nil {
			t.Fatalf("NewRunner(%d) err = %v, want nil", b, err)
		}
		if r.Backend() != b {
			t.Errorf("Backend() = %d, want %d", r.Backend(), b)
		}
	}
}

func TestWrapEscalation(t *testing.T) {
	const abs = "/usr/sbin/useradd"
	args := []string{"-m", "deploy"}

	tests := []struct {
		backend  PrivilegeBackend
		escalate bool
		wantName string
		wantArgv []string
	}{
		{Direct, true, abs, []string{"-m", "deploy"}},
		{Direct, false, abs, []string{"-m", "deploy"}},
		{Sudo, true, "sudo", []string{"-n", abs, "-m", "deploy"}},
		{Doas, true, "doas", []string{"-n", abs, "-m", "deploy"}},
		{Sudo, false, abs, []string{"-m", "deploy"}},
		{Doas, false, abs, []string{"-m", "deploy"}},
	}
	for _, tc := range tests {
		name, argv := wrapEscalation(tc.backend, tc.escalate, abs, args)
		if name != tc.wantName {
			t.Errorf("backend=%d escalate=%v: name=%q, want %q", tc.backend, tc.escalate, name, tc.wantName)
		}
		if strings.Join(argv, " ") != strings.Join(tc.wantArgv, " ") {
			t.Errorf("backend=%d escalate=%v: argv=%v, want %v", tc.backend, tc.escalate, argv, tc.wantArgv)
		}
	}
}

func TestWrapEscalation_DoesNotMutateCallerArgs(t *testing.T) {
	args := []string{"-m", "deploy"}
	_, _ = wrapEscalation(Sudo, true, "/usr/sbin/useradd", args)
	if args[0] != "-m" || args[1] != "deploy" || len(args) != 2 {
		t.Errorf("caller args mutated: %v", args)
	}
}

func directRunner(t *testing.T) Runner {
	t.Helper()
	r, err := NewRunner(Direct)
	if err != nil {
		t.Fatalf("NewRunner(Direct): %v", err)
	}
	return r
}

func TestRunner_NonZeroExitIsNotAnError(t *testing.T) {
	res, err := directRunner(t).Run(context.Background(), Command{Name: "sh", Args: []string{"-c", "exit 3"}})
	if err != nil {
		t.Fatalf("Run err = %v, want nil for a clean non-zero exit", err)
	}
	if res.ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3", res.ExitCode)
	}
}

func TestRunner_CapturesStdout(t *testing.T) {
	res, err := directRunner(t).Run(context.Background(), Command{Name: "sh", Args: []string{"-c", "printf hello"}})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", res.ExitCode)
	}
	if !strings.Contains(res.Stdout, "hello") {
		t.Errorf("Stdout = %q, want it to contain hello", res.Stdout)
	}
}

func TestRunner_RejectsEmptyNameAndUnknownBinary(t *testing.T) {
	if _, err := directRunner(t).Run(context.Background(), Command{Name: ""}); err == nil {
		t.Error("Run with empty Name returned nil error, want a failure-to-execute error")
	}
	if _, err := directRunner(t).Run(context.Background(), Command{Name: "definitely-not-a-real-binary-xyz"}); !errors.Is(err, ErrBackendUnavailable) {
		t.Errorf("Run with a nonexistent binary err = %v, want ErrBackendUnavailable (errors.Is-matchable)", err)
	}
}

func TestBuildChildEnv_DefaultBranchDropsHijackVars(t *testing.T) {
	t.Setenv("LD_PRELOAD", "/tmp/evil.so")
	t.Setenv("BASH_ENV", "/tmp/evil.sh")
	t.Setenv("CADESTRO_BENIGN_TEST_VAR", "keep-me")
	t.Setenv("LANG", "de_DE.UTF-8")

	env, err := buildChildEnv(Command{})
	if err != nil {
		t.Fatalf("buildChildEnv: %v", err)
	}
	has := func(prefix string) bool {
		for _, e := range env {
			if strings.HasPrefix(e, prefix) {
				return true
			}
		}
		return false
	}
	if has("LD_PRELOAD=") {
		t.Error("LD_PRELOAD leaked from the parent env into the child")
	}
	if has("BASH_ENV=") {
		t.Error("BASH_ENV leaked from the parent env into the child")
	}
	if !has("CADESTRO_BENIGN_TEST_VAR=keep-me") {
		t.Error("a benign inherited var was dropped; the default branch must still inherit safe vars")
	}

	if got := env[len(env)-3:]; got[0] != "LC_ALL=C" || got[1] != "LANG=C" || got[2] != "NO_COLOR=1" {
		t.Errorf("tail = %v, want forcedEnv last (LC_ALL=C, LANG=C, NO_COLOR=1)", got)
	}
}

func TestBuildChildEnv_DefaultBranchKeepsPATH(t *testing.T) {
	const parentPath = "/usr/local/bin:/usr/bin:/bin"
	t.Setenv("PATH", parentPath)

	env, err := buildChildEnv(Command{})
	if err != nil {
		t.Fatalf("buildChildEnv: %v", err)
	}
	var paths []string
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, "PATH="); ok {
			paths = append(paths, v)
		}
	}
	if len(paths) != 1 {
		t.Fatalf("child env carries %d PATH entries %q, want exactly 1; env = %v", len(paths), paths, env)
	}
	if paths[0] != parentPath {
		t.Errorf("child PATH = %q, want the parent's %q", paths[0], parentPath)
	}
}

func TestResolveAbsolute_BareNameIsAbsolute(t *testing.T) {
	abs, err := resolveAbsolute("sh")
	if err != nil {
		t.Fatalf("resolveAbsolute(sh) err = %v", err)
	}
	if !filepath.IsAbs(abs) {
		t.Errorf("resolveAbsolute(sh) = %q, want an absolute path", abs)
	}
}

func TestResolveAbsolute_RelativeSlashNameResolvedToAbsolute(t *testing.T) {
	dir := t.TempDir()
	tool := filepath.Join(dir, "tool")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	abs, err := resolveAbsolute("./tool")
	if err != nil {
		t.Fatalf("resolveAbsolute(./tool) err = %v", err)
	}
	if !filepath.IsAbs(abs) {
		t.Errorf("resolveAbsolute(./tool) = %q, want an absolute path (LookPath leaves it relative)", abs)
	}
}

func TestRunner_RespectsAlreadyCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := directRunner(t).Run(ctx, Command{Name: "sh", Args: []string{"-c", "exit 0"}}); !errors.Is(err, context.Canceled) {
		t.Errorf("Run with a pre-cancelled ctx err = %v, want context.Canceled", err)
	}
}

func TestRunner_DeliversStdin(t *testing.T) {
	res, err := directRunner(t).Run(context.Background(), Command{
		Name:  "cat",
		Stdin: strings.NewReader("piped-secret"),
	})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if !strings.Contains(res.Stdout, "piped-secret") {
		t.Errorf("Stdout = %q, want the piped stdin echoed back", res.Stdout)
	}
}

func TestRunner_EnvBlocklistRejectedBeforeExec(t *testing.T) {
	r := directRunner(t)
	if _, err := r.Run(context.Background(), Command{Name: "sh", Args: []string{"-c", "true"}, Env: []string{"LD_PRELOAD=/evil.so"}}); !errors.Is(err, ErrBlockedEnvVar) {
		t.Errorf("blocked env err = %v, want ErrBlockedEnvVar", err)
	}
	if _, err := r.Run(context.Background(), Command{Name: "sh", Args: []string{"-c", "true"}, Env: []string{"PATH=/evil"}}); !errors.Is(err, ErrBlockedEnvVar) {
		t.Errorf("PATH-via-Env err = %v, want ErrBlockedEnvVar (use ChildPath for a curated PATH)", err)
	}
	if _, err := r.Run(context.Background(), Command{Name: "sh", Args: []string{"-c", "true"}, Env: []string{"not-key-value"}}); !errors.Is(err, ErrInvalidEnvVar) {
		t.Errorf("malformed env err = %v, want ErrInvalidEnvVar", err)
	}
}

func TestRunner_AllowedEnvReachesChild(t *testing.T) {
	res, err := directRunner(t).Run(context.Background(), Command{
		Name: "sh", Args: []string{"-c", "printf %s \"$CADESTRO_TEST_VAR\""},
		Env: []string{"CADESTRO_TEST_VAR=visible"},
	})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if !strings.Contains(res.Stdout, "visible") {
		t.Errorf("Stdout = %q, want the allowed env var visible to the child", res.Stdout)
	}
}

func TestRunner_ForcesDeterministicEnv(t *testing.T) {
	res, err := directRunner(t).Run(context.Background(), Command{
		Name: "sh", Args: []string{"-c", `printf '%s|%s|%s' "$LC_ALL" "$LANG" "$NO_COLOR"`},
	})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if got := strings.TrimSpace(res.Stdout); got != "C|C|1" {
		t.Errorf("forced LC_ALL|LANG|NO_COLOR = %q, want \"C|C|1\"", got)
	}
}

func TestRunner_RejectsReservedEnv(t *testing.T) {
	for _, e := range []string{"LANG=ja_JP.UTF-8", "LC_ALL=ja_JP.UTF-8", "LC_NUMERIC=de_DE.UTF-8", "LANGUAGE=ja", "NO_COLOR="} {
		_, err := directRunner(t).Run(context.Background(), Command{Name: "true", Env: []string{e}})
		if !errors.Is(err, ErrReservedEnvVar) {
			t.Errorf("Run with Env %q err = %v, want ErrReservedEnvVar", e, err)
		}
	}
}

func TestRunner_InheritsParentEnvWhilePinningLocale(t *testing.T) {
	t.Setenv("CADESTRO_TEST_INHERIT", "visible")
	res, err := directRunner(t).Run(context.Background(), Command{
		Name: "sh", Args: []string{"-c", `printf '%s|%s' "$CADESTRO_TEST_INHERIT" "$LC_ALL"`},
	})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if got := strings.TrimSpace(res.Stdout); got != "visible|C" {
		t.Errorf("got %q, want \"visible|C\" (parent var inherited + locale pinned)", got)
	}
}

func TestRunner_ChildPathIsAuthoritative(t *testing.T) {
	res, err := directRunner(t).Run(context.Background(), Command{
		Name: "sh", Args: []string{"-c", "printf %s \"$PATH\""},
		ChildPath: "/curated/bin:/curated/sbin",
	})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if strings.TrimSpace(res.Stdout) != "/curated/bin:/curated/sbin" {
		t.Errorf("child PATH = %q, want the curated path", res.Stdout)
	}
}

func TestDetectEscalationDenied(t *testing.T) {
	tests := []struct {
		name    string
		backend PrivilegeBackend
		res     Result
		want    bool
	}{
		{"sudo password required", Sudo, Result{ExitCode: 1, Stderr: "sudo: a password is required"}, true},
		{"sudo terminal required", Sudo, Result{ExitCode: 1, Stderr: "sudo: a terminal is required to read the password"}, true},
		{"doas auth failure", Doas, Result{ExitCode: 1, Stderr: "doas: Authorization required"}, true},
		{"direct never denied", Direct, Result{ExitCode: 1, Stderr: "anything"}, false},
		{"clean exit not denied", Sudo, Result{ExitCode: 0}, false},
		{"genuine command failure not denied", Sudo, Result{ExitCode: 1, Stderr: "useradd: user already exists"}, false},
	}
	for _, tc := range tests {
		err := detectEscalationDenied(tc.backend, tc.res)
		if got := errors.Is(err, ErrEscalationDenied); got != tc.want {
			t.Errorf("%s: denied=%v, want %v (err=%v)", tc.name, got, tc.want, err)
		}
	}
}

func TestRunner_SIGKILLsChildThatIgnoresSIGTERM(t *testing.T) {

	restore := killGrace
	killGrace = 200 * time.Millisecond
	defer func() { killGrace = restore }()

	pidFile := t.TempDir() + "/pid"
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	type result struct{ err error }
	done := make(chan result, 1)
	start := time.Now()
	go func() {
		_, err := directRunner(t).Run(ctx, Command{Name: "sh", Args: []string{"-c", sigtermIgnoringScript(pidFile)}})
		done <- result{err}
	}()

	select {
	case r := <-done:
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Errorf("Runner.Run took %v; SIGTERM-ignoring child was not SIGKILLed after grace", elapsed)
		}
		if !errors.Is(r.err, context.DeadlineExceeded) {
			t.Errorf("err = %v, want context.DeadlineExceeded on cancel", r.err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("Runner.Run pinned on a SIGTERM-ignoring child — SIGKILL escalation not wired into the Runner")
	}

	assertProcessGroupGone(t, readChildPID(t, pidFile))
}

func TestRunner_OutputCapped(t *testing.T) {
	res, err := directRunner(t).Run(context.Background(), Command{
		Name: "sh", Args: []string{"-c", "yes aaaaaaaaaaaaaaaa | head -c 2000000"},
	})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if len(res.Stdout) > MaxOutputBytes+len("\n[output truncated]")+64 {
		t.Errorf("Stdout len = %d, want capped near MaxOutputBytes (%d)", len(res.Stdout), MaxOutputBytes)
	}
	if !strings.Contains(res.Stdout, "[output truncated]") {
		t.Error("Stdout missing the [output truncated] marker after exceeding the cap")
	}
}

func TestRunner_StreamDeliversLines(t *testing.T) {
	var got []string
	_, err := directRunner(t).Stream(context.Background(),
		Command{Name: "sh", Args: []string{"-c", "printf 'a\\nb\\nc\\n'"}},
		func(s StreamType, line string, seq int64) {
			if s == StreamStdout {
				got = append(got, strings.TrimSpace(line))
			}
		})
	if err != nil {
		t.Fatalf("Stream err = %v", err)
	}
	if strings.Join(got, ",") != "a,b,c" {
		t.Errorf("streamed lines = %v, want [a b c]", got)
	}
}

func TestRunner_RunInDir(t *testing.T) {
	dir := t.TempDir()
	res, err := directRunner(t).Run(context.Background(), Command{Name: "pwd", Dir: dir})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if got := strings.TrimSpace(res.Stdout); got != dir {
		t.Errorf("pwd in Dir=%q = %q, want %q", dir, got, dir)
	}
}

func TestRunner_StreamDeliversStderr(t *testing.T) {
	var got []string
	res, err := directRunner(t).Stream(context.Background(),
		Command{Name: "sh", Args: []string{"-c", "printf 'e1\\ne2\\n' >&2"}},
		func(s StreamType, line string, _ int64) {
			if s == StreamStderr {
				got = append(got, strings.TrimSpace(line))
			}
		})
	if err != nil {
		t.Fatalf("Stream err = %v", err)
	}
	if strings.Join(got, ",") != "e1,e2" {
		t.Errorf("streamed stderr = %v, want [e1 e2]", got)
	}
	if !strings.Contains(res.Stderr, "e1") {
		t.Errorf("captured Stderr = %q, want it to contain e1", res.Stderr)
	}
}

func TestRunner_StderrOutputCapped(t *testing.T) {
	res, err := directRunner(t).Run(context.Background(), Command{
		Name: "sh", Args: []string{"-c", "yes aaaaaaaaaaaaaaaa | head -c 2000000 1>&2"},
	})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if !strings.Contains(res.Stderr, "[output truncated]") {
		t.Error("Stderr missing the [output truncated] marker after exceeding the cap")
	}
}
