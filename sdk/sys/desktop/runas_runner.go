package desktop

import (
	"context"
	"fmt"
	"strings"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

// RunAsRunner wraps a base [exec.Runner] so every command it runs executes AS the
// session's user via runuser, with that user's desktop session environment (HOME,
// USER, XDG_RUNTIME_DIR, the session bus address) and the curated per-user PATH.
//
// It lets a capability built on the SDK Runner operate on behalf of a specific
// logged-in user without that capability knowing anything about runuser — most
// usefully a per-user Flatpak Manager:
//
//	r, _ := exec.NewRunner(exec.Direct) // the agent runs as root
//	ru, _ := desktop.RunAsRunner(r, session)
//	fp, _ := pkg.NewUserFlatpak(ru)
//	fp.Install(ctx, "flathub", "org.x.App") // installs for `session`
//
// The base Runner MUST run as root: runuser performs the privilege DROP to the
// target user, so the wrapped command is never escalated again. The caller's
// command env is screened by the same hijack blocklist the Runner enforces.
func RunAsRunner(base sysexec.Runner, s Session) (sysexec.Runner, error) {
	if base == nil {
		return nil, fmt.Errorf("desktop.RunAsRunner: %w", sysexec.ErrRunnerRequired)
	}
	if s.Username == "" {
		return nil, fmt.Errorf("desktop.RunAsRunner: session has empty Username")
	}
	return &runAsRunner{base: base, s: s}, nil
}

type runAsRunner struct {
	base sysexec.Runner
	s    Session
}

func (ra *runAsRunner) Backend() sysexec.PrivilegeBackend { return ra.base.Backend() }

func (ra *runAsRunner) Run(ctx context.Context, c sysexec.Command) (sysexec.Result, error) {
	wrapped, err := ra.wrap(c)
	if err != nil {
		return sysexec.Result{}, err
	}
	return ra.base.Run(ctx, wrapped)
}

func (ra *runAsRunner) Stream(ctx context.Context, c sysexec.Command, onLine sysexec.OutputCallback) (sysexec.Result, error) {
	wrapped, err := ra.wrap(c)
	if err != nil {
		return sysexec.Result{}, err
	}
	return ra.base.Stream(ctx, wrapped, onLine)
}

// wrap rewrites c into `runuser -u <user> -- env <session-env> PATH=<curated>
// <name> <args...>`, running it as the session user with that user's desktop
// environment. PATH is forced last (a caller-supplied PATH is dropped);
// the rest of the caller's env is screened through the hijack blocklist because
// it is spliced into the inner env wrapper, which the base Runner does not screen.
func (ra *runAsRunner) wrap(c sysexec.Command) (sysexec.Command, error) {
	if c.Name == "" {
		return sysexec.Command{}, fmt.Errorf("desktop.RunAsRunner: command name is required")
	}
	if err := validateExtraEnv(c.Env); err != nil {
		return sysexec.Command{}, err
	}
	env := EnvFor(ra.s)
	for _, e := range c.Env {
		if key, _, ok := strings.Cut(e, "="); ok && key == "PATH" {
			continue // PATH is always forced to the curated UserPath below
		}
		env = append(env, e)
	}
	env = append(env, "PATH="+UserPath(ra.s))

	args := append([]string{"-u", ra.s.Username, "--", envPath}, env...)
	args = append(args, c.Name)
	args = append(args, c.Args...)
	return sysexec.Command{
		Name:     runuserPath,
		Args:     args,
		Dir:      c.Dir, // run in the caller's working dir (default: the user's home)
		Stdin:    c.Stdin,
		Escalate: false, // runuser from root IS the privilege drop
	}, nil
}
