package exec

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PrivilegeBackend selects how a Runner escalates privilege for a Command whose
// Escalate flag is set. The zero value is INVALID (fail-closed): NewRunner
// rejects it with ErrUnknownBackend. Valid values start at 1 (Decision 5/6).
type PrivilegeBackend int

const (
	// Sudo escalates via `sudo -n` (non-interactive; never prompts).
	Sudo PrivilegeBackend = iota + 1
	// Doas escalates via `doas -n`.
	Doas
	// Direct runs with no wrapper — the process is already root. Rolling our
	// own no-op pass-through avoids sudo's distro-varying "root must be in
	// sudoers" check (opensuse rejects it by default) and the cost of forking
	// sudo just to re-exec the same binary.
	Direct
)

// Command describes one execution. The zero value is invalid — Name is
// required. The capability layer fills this in and sets Escalate per operation;
// it is escalation-method-agnostic. The Runner alone turns Escalate into the
// concrete sudo/doas/bare invocation.
//
// There is no locale knob: the Runner ALWAYS forces a deterministic environment
// (LC_ALL=C, LANG=C, NO_COLOR=1) on every command so the SDK's parsing of tool
// output is locale/format-stable by construction. It is not overridable — those
// names are rejected if passed via Env. (TZ is deliberately left to the device.)
type Command struct {
	Name      string    // resolved to an absolute path before escalation
	Args      []string  // operands; the caller pre-applies SeparatePositionals
	Dir       string    // "" = inherit cwd
	Env       []string  // extra KEY=VALUE; screened by the env hijack blocklist
	Stdin     io.Reader // "" = no stdin
	ChildPath string    // explicit, isolating child PATH; "" = inherit/sanitized
	Escalate  bool      // run through the privilege backend
}

// Runner abstracts command execution + privilege escalation. It is injected
// into every capability constructor (Decision 2) so the SDK keeps no global
// escalation state and the whole capability layer is unit-testable with a fake
// (see exectest.FakeRunner) — no host, no sudo, no container.
type Runner interface {
	// Run executes c and returns its captured output. A non-zero exit is
	// reported in Result.ExitCode, NOT as an error; a non-nil error means the
	// command could not be executed (binary not found, blocked env var, ctx
	// cancelled) or escalation failed (ErrEscalation*).
	Run(ctx context.Context, c Command) (Result, error)
	// Stream is Run with real-time line delivery via onLine.
	Stream(ctx context.Context, c Command, onLine OutputCallback) (Result, error)
	// Backend reports the privilege backend, so a capability (e.g. fs) can pick
	// its fd-safe vs sudo code path.
	Backend() PrivilegeBackend
}

type runner struct{ backend PrivilegeBackend }

// NewRunner builds a Runner for the named backend. It is PURE: it validates the
// backend is known and does NOT probe the host (use Detect for that). The zero
// value and any unimplemented backend are rejected with ErrUnknownBackend.
func NewRunner(b PrivilegeBackend) (Runner, error) {
	switch b {
	case Sudo, Doas, Direct:
		return &runner{backend: b}, nil
	default:
		return nil, fmt.Errorf("%w: %d", ErrUnknownBackend, int(b))
	}
}

func (r *runner) Backend() PrivilegeBackend { return r.backend }

func (r *runner) Run(ctx context.Context, c Command) (Result, error) {
	return r.exec(ctx, c, nil)
}

func (r *runner) Stream(ctx context.Context, c Command, onLine OutputCallback) (Result, error) {
	return r.exec(ctx, c, onLine)
}

func (r *runner) exec(ctx context.Context, c Command, onLine OutputCallback) (Result, error) {

	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if c.Name == "" {
		return Result{}, fmt.Errorf("exec: command name is required")
	}

	env, err := buildChildEnv(c)
	if err != nil {
		return Result{}, err
	}

	absPath, err := resolveAbsolute(c.Name)
	if err != nil {
		return Result{}, fmt.Errorf("%w: command not found: %s", ErrBackendUnavailable, c.Name)
	}

	if c.Escalate {
		if tool := escalationTool(r.backend); tool != "" {
			if _, err := exec.LookPath(tool); err != nil {
				return Result{}, fmt.Errorf("%w: %s", ErrEscalationUnavailable, tool)
			}
		}
	}
	name, argv := wrapEscalation(r.backend, c.Escalate, absPath, c.Args)

	res, runErr := runStreamingWithStdin(ctx, name, argv, c.Stdin, env, c.Dir, onLine)
	result := Result{}
	if res != nil {
		result = *res
	}
	if runErr != nil {
		return result, runErr
	}

	if c.Escalate {
		if denied := detectEscalationDenied(r.backend, result); denied != nil {
			return result, denied
		}
	}
	return result, nil
}

func resolveAbsolute(name string) (string, error) {
	p, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	return filepath.Abs(p)
}

func escalationTool(b PrivilegeBackend) string {
	switch b {
	case Sudo:
		return "sudo"
	case Doas:
		return "doas"
	default:
		return ""
	}
}

func wrapEscalation(b PrivilegeBackend, escalate bool, absPath string, args []string) (string, []string) {
	tool := escalationTool(b)
	if !escalate || tool == "" {
		return absPath, append([]string(nil), args...)
	}
	argv := make([]string, 0, len(args)+2)
	argv = append(argv, "-n", absPath)
	argv = append(argv, args...)
	return tool, argv
}

func detectEscalationDenied(b PrivilegeBackend, res Result) error {
	if res.ExitCode == 0 {
		return nil
	}
	s := res.Stderr
	switch b {
	case Sudo:
		if strings.Contains(s, "a password is required") ||
			strings.Contains(s, "a terminal is required") ||
			strings.Contains(s, "no askpass program") {
			return fmt.Errorf("%w: %s", ErrEscalationDenied, strings.TrimSpace(s))
		}
	case Doas:
		if strings.Contains(s, "Authorization required") ||
			strings.Contains(s, "Authentication failed") {
			return fmt.Errorf("%w: %s", ErrEscalationDenied, strings.TrimSpace(s))
		}
	}
	return nil
}

var forcedEnv = []string{"LC_ALL=C", "LANG=C", "NO_COLOR=1"}

func buildChildEnv(c Command) ([]string, error) {

	if err := ValidateCommandEnv(c.Env); err != nil {
		return nil, err
	}
	switch {
	case c.ChildPath != "":

		return append(composeEnv(c.ChildPath, c.Env), forcedEnv...), nil
	case len(c.Env) > 0:

		return append(composeEnv(os.Getenv("PATH"), c.Env), forcedEnv...), nil
	default:

		var env []string
		for _, e := range os.Environ() {
			if key, _, ok := strings.Cut(e, "="); !ok || !IsAllowedEnvVar(key) {
				continue
			}
			env = append(env, e)
		}

		return append(composeEnv(os.Getenv("PATH"), env), forcedEnv...), nil
	}
}
