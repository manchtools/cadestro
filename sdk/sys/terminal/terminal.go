// Package terminal provides PTY-based shell session management for remote
// terminal access. It allocates a pseudo-terminal, spawns a shell as a
// configured Linux user, and exposes a small API for stdin/stdout I/O plus
// out-of-band controls (resize, close, wait).
//
// This package is the SDK foundation for the remote terminal feature; the
// agent is responsible for wiring it to the bidirectional control stream
// and enforcing authentication, audit, and session limits.
//
//	m, _ := terminal.New()
//	sess, err := m.Open(ctx, terminal.SessionConfig{User: "alice"})
//	if err != nil { ... }
//	defer sess.Close()
//
// terminal is a single-implementation capability (design §3.8): it exposes the
// Manager interface for shape-uniformity with the rest of the SDK. Unlike the
// other capabilities it takes NO exec.Runner — a PTY session is a long-lived,
// bidirectional stream, not a captured one-shot command, so the Runner
// abstraction (which returns a completed Result) cannot model it. The privilege
// to switch UID comes from the agent already running as root, applied via the
// child's syscall.Credential.
package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/creack/pty"
)

// Manager constructs PTY shell sessions. See the package doc for why it carries
// no Runner.
type Manager interface {
	// Open allocates a PTY and spawns a login shell as cfg.User. ctx governs the
	// allocation only: the returned Session OUTLIVES ctx, so cancelling ctx
	// after Open returns does NOT stop the session — terminate it with Close.
	Open(ctx context.Context, cfg SessionConfig) (*Session, error)
}

var (
	lookupUser = user.Lookup
	getuid     = os.Getuid
	getgid     = os.Getgid
	ptyClose   = func(f *os.File) error { return f.Close() }
)

type manager struct{}

// New returns a terminal Manager. It takes no arguments — no Runner (see the
// package doc), no backend — and does not fail today; the error return matches
// the SDK's uniform New(...) (T, error) shape so a future fallible option is
// additive rather than a breaking signature change.
func New() (Manager, error) {
	return &manager{}, nil
}

// Defaults applied when SessionConfig fields are zero.
const (
	DefaultShell = "/bin/bash"
	DefaultCols  = 80
	DefaultRows  = 24
)

// SessionConfig configures a new PTY session.
type SessionConfig struct {
	// User is the Linux username to run the shell as. Required.
	User string

	// Shell is the absolute path of the shell binary. Defaults to
	// DefaultShell ("/bin/bash"). The binary must exist and be executable.
	Shell string

	// Cols and Rows are the initial terminal window size. Zero values are
	// replaced with DefaultCols and DefaultRows.
	Cols uint16
	Rows uint16

	// Env is the environment variables passed to the shell. HOME, USER,
	// LOGNAME, SHELL, TERM, and PATH are populated automatically if absent.
	Env []string

	// WorkDir is the working directory for the shell. Defaults to the
	// user's home directory if it exists, otherwise /tmp.
	WorkDir string
}

// Session represents a running PTY shell session. The zero value is not
// usable; obtain a Session via Manager.Open.
//
// Read, Write, and Resize are safe for concurrent use from independent
// goroutines (the typical reader+writer pattern). Close, Wait, and Done
// may be called from any goroutine and at any time.
type Session struct {
	cmd *exec.Cmd
	pty *os.File

	fdMu sync.Mutex

	closeOnce sync.Once
	closeErr  error

	waitOnce sync.Once
	waitErr  error
	exitCode int

	done chan struct{}
}

// Open allocates a PTY, spawns a shell as cfg.User, and returns a Session.
// The caller must call Close (or Wait followed by reaping done) to release
// resources. Open returns an error if the context is already cancelled, the
// user cannot be looked up, the shell binary is missing, or the PTY cannot be
// allocated.
//
// ctx governs allocation only. The returned Session is detached from ctx and
// outlives it; cancelling ctx after Open returns does not terminate the
// session — use Close.
func (m *manager) Open(ctx context.Context, cfg SessionConfig) (*Session, error) {

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cfg.User == "" {
		return nil, errors.New("terminal: user is required")
	}
	u, err := lookupUser(cfg.User)
	if err != nil {
		return nil, fmt.Errorf("terminal: lookup user %q: %w", cfg.User, err)
	}
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("terminal: parse uid %q: %w", u.Uid, err)
	}
	gid, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("terminal: parse gid %q: %w", u.Gid, err)
	}

	shell := cfg.Shell
	if shell == "" {
		shell = DefaultShell
	}
	if !filepath.IsAbs(shell) {
		return nil, fmt.Errorf("terminal: shell %q must be an absolute path", shell)
	}
	info, err := os.Stat(shell)
	if err != nil {
		return nil, fmt.Errorf("terminal: stat shell %q: %w", shell, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("terminal: shell %q is a directory", shell)
	}
	if info.Mode()&0o111 == 0 {
		return nil, fmt.Errorf("terminal: shell %q is not executable", shell)
	}

	cols := cfg.Cols
	if cols == 0 {
		cols = DefaultCols
	}
	rows := cfg.Rows
	if rows == 0 {
		rows = DefaultRows
	}

	workDir := cfg.WorkDir
	if workDir == "" {
		workDir = defaultWorkDir(u)
	}

	cmd := exec.Command(shell, "-l")
	cmd.Dir = workDir
	cmd.Env = buildEnv(cfg.Env, u, shell)

	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if uint32(uid) != uint32(getuid()) || uint32(gid) != uint32(getgid()) {
		cmd.SysProcAttr.Credential = &syscall.Credential{
			Uid: uint32(uid),
			Gid: uint32(gid),
		}
	}

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		return nil, fmt.Errorf("terminal: start pty: %w", err)
	}

	s := &Session{
		cmd:  cmd,
		pty:  ptmx,
		done: make(chan struct{}),
	}
	go s.reap()
	return s, nil
}

func (s *Session) reap() {
	err := s.cmd.Wait()
	s.waitOnce.Do(func() {
		s.waitErr = err
		if s.cmd.ProcessState != nil {
			s.exitCode = s.cmd.ProcessState.ExitCode()
		}
	})
	s.fdMu.Lock()
	_ = s.pty.Close()
	s.fdMu.Unlock()
	close(s.done)
}

// Read reads from the PTY master (the shell's combined stdout/stderr).
// Returns io.EOF after the shell exits and Close has been called, or an
// underlying I/O error otherwise.
func (s *Session) Read(buf []byte) (int, error) {
	return s.pty.Read(buf)
}

// Write writes to the PTY master (the shell's stdin).
func (s *Session) Write(data []byte) (int, error) {
	return s.pty.Write(data)
}

// Resize changes the window size of the PTY. The shell receives SIGWINCH
// and applications using ncurses (vim, top, etc.) re-render accordingly.
//
// The dimensions are validated before the ioctl (WS15): a zero cols or rows is
// rejected rather than passed to TIOCSWINSZ — a zero-size window confuses
// curses applications and is never a legitimate request. The wire-level upper
// bound (<= 65535) is already enforced by the uint16 type at this boundary; the
// agent additionally rejects out-of-range uint32 values before narrowing, so a
// truncated value can never reach here.
func (s *Session) Resize(cols, rows uint16) error {
	if err := validateDims(cols, rows); err != nil {
		return err
	}

	s.fdMu.Lock()
	defer s.fdMu.Unlock()
	if err := pty.Setsize(s.pty, &pty.Winsize{Cols: cols, Rows: rows}); err != nil {
		return fmt.Errorf("terminal: resize: %w", err)
	}
	return nil
}

func validateDims(cols, rows uint16) error {
	if cols == 0 || rows == 0 {
		return fmt.Errorf("terminal: invalid dimensions cols=%d rows=%d (both must be > 0)", cols, rows)
	}
	return nil
}

// Close terminates the shell session. If the shell is still running it
// sends SIGTERM to the entire process group; if the shell has already
// exited (Done channel closed) the signal is skipped to avoid the small
// PID-recycling race window where the original PGID may belong to an
// unrelated process. Closing the PTY master is always performed.
// Safe to call multiple times; subsequent calls return the same error
// (or nil).
//
// After Close, Read and Write return errors. Use Wait or Done to observe
// the actual exit.
func (s *Session) Close() error {
	s.closeOnce.Do(func() {

		select {
		case <-s.done:

		default:

			if s.cmd.Process != nil {
				_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGTERM)
			}
		}

		s.fdMu.Lock()
		err := ptyClose(s.pty)
		s.fdMu.Unlock()
		if err != nil && !errors.Is(err, os.ErrClosed) {
			s.closeErr = err
		}
	})
	return s.closeErr
}

// Wait blocks until the shell exits and returns its exit code. Calling
// Wait from multiple goroutines is safe; all callers see the same result.
// Wait returns the cmd.Wait error (typically *exec.ExitError) so callers
// can distinguish a non-zero exit from a wait failure if needed.
func (s *Session) Wait() (int, error) {
	<-s.done
	return s.exitCode, s.waitErr
}

// Done returns a channel that is closed when the shell process has exited
// and its exit code is available via Wait.
func (s *Session) Done() <-chan struct{} {
	return s.done
}

func defaultWorkDir(u *user.User) string {
	if u.HomeDir != "" {
		if info, err := os.Stat(u.HomeDir); err == nil && info.IsDir() {
			return u.HomeDir
		}
	}
	return "/tmp"
}

func buildEnv(extra []string, u *user.User, shell string) []string {
	have := map[string]struct{}{}
	out := make([]string, 0, len(extra)+6)
	for _, e := range extra {
		if i := strings.IndexByte(e, '='); i > 0 {
			have[e[:i]] = struct{}{}
		}
		out = append(out, e)
	}
	add := func(k, v string) {
		if v == "" {
			return
		}
		if _, ok := have[k]; ok {
			return
		}
		out = append(out, k+"="+v)
	}
	add("HOME", u.HomeDir)
	add("USER", u.Username)
	add("LOGNAME", u.Username)
	add("SHELL", shell)
	add("TERM", "xterm-256color")
	add("PATH", "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	return out
}

var _ io.ReadWriter = (*Session)(nil)

var _ Manager = (*manager)(nil)
