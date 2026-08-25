package terminal

import (
	"context"
	"errors"
	"io"
	"os"
	"os/user"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func isSandboxStartErr(err error) bool {
	return errors.Is(err, syscall.EPERM) ||
		errors.Is(err, syscall.EACCES) ||
		errors.Is(err, syscall.ENOSYS) ||
		errors.Is(err, os.ErrPermission)
}

func openSession(t *testing.T, cfg SessionConfig) (*Session, error) {
	t.Helper()
	m, err := New()
	if err != nil {
		t.Fatalf("terminal.New: %v", err)
	}
	return m.Open(context.Background(), cfg)
}

func startSessionOrSkip(t *testing.T, cfg SessionConfig) *Session {
	t.Helper()
	s, err := openSession(t, cfg)
	if err != nil {
		if isSandboxStartErr(err) {
			t.Skipf("start session: sandbox restriction: %v", err)
		}
		t.Fatalf("start session: %v", err)
	}
	return s
}

func requireLinuxCurrentUser(t *testing.T) *user.User {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("PTY tests are linux-only")
	}
	cur, err := user.Current()
	if err != nil {
		t.Skipf("user.Current() failed: %v", err)
	}
	return cur
}

func TestOpen_EmptyUser(t *testing.T) {
	_, err := openSession(t, SessionConfig{})
	if err == nil {
		t.Fatal("expected error for empty user")
	}
	if !strings.Contains(err.Error(), "user is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOpen_UnknownUser(t *testing.T) {
	_, err := openSession(t, SessionConfig{User: "this-user-definitely-does-not-exist-cadestro-12345"})
	if err == nil {
		t.Fatal("expected error for unknown user")
	}
	var unknownErr user.UnknownUserError
	if !errors.As(err, &unknownErr) {
		t.Errorf("expected user.UnknownUserError in chain, got: %v", err)
	}
}

func TestOpen_MissingShell(t *testing.T) {
	cur, err := user.Current()
	if err != nil {
		t.Skipf("user.Current() failed: %v", err)
	}
	_, err = openSession(t, SessionConfig{
		User:  cur.Username,
		Shell: "/no/such/shell/binary",
	})
	if err == nil {
		t.Fatal("expected error for missing shell")
	}
	if !strings.Contains(err.Error(), "stat shell") {
		t.Errorf("expected stat shell error, got: %v", err)
	}
}

func TestOpen_ShellIsDirectory(t *testing.T) {
	cur, err := user.Current()
	if err != nil {
		t.Skipf("user.Current() failed: %v", err)
	}
	_, err = openSession(t, SessionConfig{User: cur.Username, Shell: "/tmp"})
	if err == nil {
		t.Fatal("expected error when shell is a directory")
	}
	if !strings.Contains(err.Error(), "directory") {
		t.Errorf("expected directory error, got: %v", err)
	}
}

func TestOpen_ShellNotAbsolute(t *testing.T) {
	cur, err := user.Current()
	if err != nil {
		t.Skipf("user.Current() failed: %v", err)
	}
	_, err = openSession(t, SessionConfig{User: cur.Username, Shell: "bash"})
	if err == nil {
		t.Fatal("expected error for non-absolute shell path")
	}
	if !strings.Contains(err.Error(), "absolute path") {
		t.Errorf("expected absolute-path error, got: %v", err)
	}
}

func TestOpen_ShellNotExecutable(t *testing.T) {
	cur, err := user.Current()
	if err != nil {
		t.Skipf("user.Current() failed: %v", err)
	}

	dir := t.TempDir()
	notExec := dir + "/not-a-shell"
	if err := os.WriteFile(notExec, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = openSession(t, SessionConfig{User: cur.Username, Shell: notExec})
	if err == nil {
		t.Fatal("expected error for non-executable shell")
	}
	if !strings.Contains(err.Error(), "not executable") {
		t.Errorf("expected not-executable error, got: %v", err)
	}
}

func TestNew(t *testing.T) {
	m, err := New()
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if m == nil {
		t.Fatal("New() returned a nil Manager")
	}
}

func TestOpen_ContextCancelled(t *testing.T) {
	m, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = m.Open(ctx, SessionConfig{User: "this-user-definitely-does-not-exist-cadestro-12345"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Open(cancelled) error = %v, want context.Canceled", err)
	}
}

func TestValidateDims(t *testing.T) {
	cases := []struct {
		name       string
		cols, rows uint16
		wantErr    bool
	}{
		{"both valid", 80, 24, false},
		{"max valid", 65535, 65535, false},
		{"min valid", 1, 1, false},
		{"zero cols", 0, 24, true},
		{"zero rows", 80, 0, true},
		{"both zero", 0, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateDims(tc.cols, tc.rows)
			if tc.wantErr && err == nil {
				t.Errorf("validateDims(%d,%d) = nil, want error", tc.cols, tc.rows)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("validateDims(%d,%d) = %v, want nil", tc.cols, tc.rows, err)
			}
		})
	}
}

func TestSession_ResizeRejectsZeroDims(t *testing.T) {
	cur := requireLinuxCurrentUser(t)

	s := startSessionOrSkip(t, SessionConfig{User: cur.Username, Shell: pickShell(t)})
	defer s.Close()

	if err := s.Resize(0, 24); err == nil {
		t.Error("Resize(0,24) accepted a zero dimension")
	}
	if err := s.Resize(80, 0); err == nil {
		t.Error("Resize(80,0) accepted a zero dimension")
	}
	if err := s.Resize(100, 30); err != nil {
		t.Errorf("Resize(100,30) after a rejected resize failed: %v", err)
	}
}

func TestOpen_NonNumericUID(t *testing.T) {
	restore := lookupUser
	defer func() { lookupUser = restore }()
	lookupUser = func(string) (*user.User, error) {
		return &user.User{Username: "x", Uid: "not-a-number", Gid: "1000", HomeDir: "/tmp"}, nil
	}
	_, err := openSession(t, SessionConfig{User: "x"})
	if err == nil || !strings.Contains(err.Error(), "parse uid") {
		t.Errorf("Open with non-numeric uid: err = %v, want a parse-uid error", err)
	}
}

func TestOpen_NonNumericGID(t *testing.T) {
	restore := lookupUser
	defer func() { lookupUser = restore }()
	lookupUser = func(string) (*user.User, error) {
		return &user.User{Username: "x", Uid: "1000", Gid: "not-a-number", HomeDir: "/tmp"}, nil
	}
	_, err := openSession(t, SessionConfig{User: "x"})
	if err == nil || !strings.Contains(err.Error(), "parse gid") {
		t.Errorf("Open with non-numeric gid: err = %v, want a parse-gid error", err)
	}
}

func TestOpen_DefaultShell(t *testing.T) {
	cur := requireLinuxCurrentUser(t)
	info, err := os.Stat(DefaultShell)
	if err != nil || info.IsDir() || info.Mode().Perm()&0o111 == 0 {
		t.Skipf("default shell %s not usable on this host", DefaultShell)
	}
	s := startSessionOrSkip(t, SessionConfig{User: cur.Username})
	defer s.Close()
	if _, err := io.WriteString(s, "exit\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	drain(t, s, 5*time.Second)
}

func TestOpen_SwitchesCredentialWhenUIDDiffers(t *testing.T) {
	cur := requireLinuxCurrentUser(t)
	restore := getgid
	defer func() { getgid = restore }()
	getgid = func() int { return os.Getgid() + 1 }

	s := startSessionOrSkip(t, SessionConfig{User: cur.Username, Shell: pickShell(t)})
	defer s.Close()
	if _, err := io.WriteString(s, "exit\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	drain(t, s, 5*time.Second)
}

func TestSession_ResizeAfterCloseFails(t *testing.T) {
	cur := requireLinuxCurrentUser(t)
	s := startSessionOrSkip(t, SessionConfig{User: cur.Username, Shell: pickShell(t)})
	_ = s.Close()
	<-s.Done()
	if err := s.Resize(80, 24); err == nil {
		t.Error("Resize after Close should fail (PTY closed)")
	} else if !strings.Contains(err.Error(), "resize") {
		t.Errorf("Resize after Close: err = %v, want a resize error", err)
	}
}

func TestSession_CloseSurfacesPTYError(t *testing.T) {
	cur := requireLinuxCurrentUser(t)
	s := startSessionOrSkip(t, SessionConfig{User: cur.Username, Shell: pickShell(t)})

	restore := ptyClose
	defer func() { ptyClose = restore }()
	sentinel := errors.New("synthetic pty close failure")
	ptyClose = func(f *os.File) error {
		_ = f.Close()
		return sentinel
	}

	if err := s.Close(); !errors.Is(err, sentinel) {
		t.Errorf("Close() = %v, want the surfaced pty-close error %v", err, sentinel)
	}
}

func pickShell(t *testing.T) string {
	t.Helper()
	for _, candidate := range []string{"/bin/bash", "/bin/sh", "/usr/bin/bash"} {
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		if info.Mode().Perm()&0o111 == 0 {
			continue
		}
		return candidate
	}
	t.Skip("no usable shell found in fixed locations (/bin/bash, /bin/sh, /usr/bin/bash)")
	return ""
}

func TestSession_EchoAndExit(t *testing.T) {
	cur := requireLinuxCurrentUser(t)

	s := startSessionOrSkip(t, SessionConfig{
		User:  cur.Username,
		Shell: pickShell(t),
		Cols:  80,
		Rows:  24,
	})
	defer s.Close()

	if _, err := io.WriteString(s, "echo CADESTRO-OK\nexit\n"); err != nil {
		t.Fatalf("write: %v", err)
	}

	output := drain(t, s, 5*time.Second)
	if !strings.Contains(output, "CADESTRO-OK") {
		t.Errorf("expected output to contain CADESTRO-OK, got: %q", output)
	}

	code, err := s.Wait()
	if err != nil {
		t.Fatalf("wait failed: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestSession_ResizeBeforeExit(t *testing.T) {
	cur := requireLinuxCurrentUser(t)

	s := startSessionOrSkip(t, SessionConfig{User: cur.Username, Shell: pickShell(t)})
	defer s.Close()

	if err := s.Resize(120, 40); err != nil {
		t.Errorf("resize: %v", err)
	}

	if _, err := io.WriteString(s, "exit\n"); err != nil {
		t.Errorf("write after resize: %v", err)
	}
	drain(t, s, 5*time.Second)
}

func TestSession_ConcurrentResizeAndClose(t *testing.T) {
	cur := requireLinuxCurrentUser(t)
	s := startSessionOrSkip(t, SessionConfig{User: cur.Username, Shell: pickShell(t)})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_ = s.Resize(80, 24)
		}
	}()
	_ = s.Close()
	wg.Wait()
}

func TestSession_CloseUnblocksWait(t *testing.T) {
	cur := requireLinuxCurrentUser(t)

	s := startSessionOrSkip(t, SessionConfig{User: cur.Username, Shell: pickShell(t)})

	done := make(chan struct{})
	go func() {
		_, _ = s.Wait()
		close(done)
	}()

	if err := s.Close(); err != nil {
		t.Errorf("close: %v", err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return within 5s after Close")
	}
}

func TestSession_CloseIsIdempotent(t *testing.T) {
	cur := requireLinuxCurrentUser(t)

	s := startSessionOrSkip(t, SessionConfig{User: cur.Username, Shell: pickShell(t)})

	if err := s.Close(); err != nil {
		t.Errorf("first close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("second close should be no-op, got: %v", err)
	}
}

func TestSession_ReapClosesPTYWithoutExplicitClose(t *testing.T) {
	cur := requireLinuxCurrentUser(t)

	s := startSessionOrSkip(t, SessionConfig{User: cur.Username, Shell: pickShell(t)})

	if _, err := io.WriteString(s, "exit\n"); err != nil {
		t.Fatalf("write exit: %v", err)
	}
	if _, err := s.Wait(); err != nil {

		t.Logf("wait returned: %v", err)
	}

	deadline := time.After(2 * time.Second)
	read := make(chan error, 1)
	go func() {
		_, err := s.Read(make([]byte, 64))
		read <- err
	}()
	select {
	case err := <-read:
		if err == nil {
			t.Error("expected Read to fail after reap closed the PTY")
		}
	case <-deadline:
		t.Error("Read blocked forever after process exit — reap did not close PTY")
	}
}

func TestSession_DoneChannel(t *testing.T) {
	cur := requireLinuxCurrentUser(t)

	s := startSessionOrSkip(t, SessionConfig{User: cur.Username, Shell: pickShell(t)})

	select {
	case <-s.Done():
		t.Fatal("Done() closed before session ended")
	default:
	}

	if _, err := io.WriteString(s, "exit\n"); err != nil {
		t.Fatalf("write exit: %v", err)
	}

	select {
	case <-s.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("Done() did not close within 5s after exit")
	}

	_ = s.Close()
}

func TestBuildEnv_DefaultsAndOverrides(t *testing.T) {
	u := &user.User{
		Username: "alice",
		HomeDir:  "/home/alice",
	}
	env := buildEnv([]string{"FOO=bar", "TERM=screen"}, u, "/bin/bash")
	m := envToMap(env)

	if m["FOO"] != "bar" {
		t.Errorf("FOO = %q, want bar", m["FOO"])
	}

	if m["TERM"] != "screen" {
		t.Errorf("TERM = %q, want screen (caller override)", m["TERM"])
	}

	if m["HOME"] != "/home/alice" {
		t.Errorf("HOME = %q, want /home/alice", m["HOME"])
	}
	if m["USER"] != "alice" {
		t.Errorf("USER = %q, want alice", m["USER"])
	}
	if m["LOGNAME"] != "alice" {
		t.Errorf("LOGNAME = %q, want alice", m["LOGNAME"])
	}
	if m["SHELL"] != "/bin/bash" {
		t.Errorf("SHELL = %q, want /bin/bash", m["SHELL"])
	}
	if m["PATH"] == "" {
		t.Error("PATH should be populated")
	}
}

func TestBuildEnv_NoEmptyDefaults(t *testing.T) {

	u := &user.User{Username: "nobody", HomeDir: ""}
	env := buildEnv(nil, u, "/bin/sh")
	m := envToMap(env)
	if _, ok := m["HOME"]; ok {
		t.Errorf("HOME should not be set when HomeDir is empty, got %q", m["HOME"])
	}
}

func TestDefaultWorkDir_FallsBackToTmp(t *testing.T) {
	u := &user.User{Username: "ghost", HomeDir: "/no/such/home"}
	if got := defaultWorkDir(u); got != "/tmp" {
		t.Errorf("defaultWorkDir(missing home) = %q, want /tmp", got)
	}
}

func TestDefaultWorkDir_PrefersExistingHome(t *testing.T) {
	dir := t.TempDir()
	u := &user.User{Username: "test", HomeDir: dir}
	if got := defaultWorkDir(u); got != dir {
		t.Errorf("defaultWorkDir(existing home) = %q, want %q", got, dir)
	}
}

func drain(t *testing.T, s *Session, timeout time.Duration) string {
	t.Helper()
	type chunk struct {
		buf []byte
		err error
	}
	out := make(chan chunk, 1)
	go func() {
		var collected []byte
		buf := make([]byte, 4096)
		for {
			n, err := s.Read(buf)
			if n > 0 {
				collected = append(collected, buf[:n]...)
			}
			if err != nil {
				out <- chunk{collected, err}
				return
			}
		}
	}()
	select {
	case c := <-out:
		return string(c.buf)
	case <-time.After(timeout):
		_ = s.Close()
		t.Fatalf("drain: read did not finish within %v", timeout)
		return ""
	}
}

func envToMap(env []string) map[string]string {
	m := map[string]string{}
	for _, e := range env {
		if i := strings.IndexByte(e, '='); i > 0 {
			m[e[:i]] = e[i+1:]
		}
	}
	return m
}
