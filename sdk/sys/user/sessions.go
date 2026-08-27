package user

import (
	"bufio"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/manchtools/cadestro/sdk/sys/exec"
)

// KillSessions terminates all of a user's sessions. It prefers systemd's
// loginctl terminate-user and falls back to pkill -KILL -u. A pkill exit of 1
// (no matching processes) is treated as success.
func (u *shadowUtils) KillSessions(ctx context.Context, name string) error {
	if err := validateUsername(name); err != nil {
		return err
	}

	if res, err := u.exec(ctx, exec.Command{Name: "loginctl", Args: []string{"terminate-user", name}, Escalate: true}); err == nil && res.ExitCode == 0 {
		return nil
	}
	res, err := u.exec(ctx, exec.Command{Name: "pkill", Args: []string{"-KILL", "-u", name}, Escalate: true})
	if err != nil {
		return fmt.Errorf("kill sessions for %s: %w", name, err)
	}

	if res.ExitCode != 0 && res.ExitCode != 1 {
		return &exec.CommandError{Name: "pkill", ExitCode: res.ExitCode, Stderr: res.Stderr}
	}
	return nil
}

const lastTimeLayout = "Mon Jan _2 15:04:05 2006"

// LastLogin returns the most recent login time for the named user. It shells
// `last -1 -F <name>` (an unprivileged read; the Runner forces the C locale so
// the timestamp is always the stable English form) and parses the single record.
//
// A user that has never logged in has no record: `last` then prints only its
// "wtmp begins …" footer (or nothing), and LastLogin returns the zero time.Time
// with a nil error — never logging in is a legitimate state, not a failure. A
// genuine execution failure (the `last` binary missing, a cancelled context)
// propagates so it is never mistaken for "never logged in".
func (u *shadowUtils) LastLogin(ctx context.Context, name string) (time.Time, error) {
	if err := validateUsername(name); err != nil {
		return time.Time{}, err
	}
	res, err := u.exec(ctx, exec.Command{Name: "last", Args: []string{"-1", "-F", name}})
	if err != nil {
		return time.Time{}, fmt.Errorf("last login for %s: %w", name, err)
	}
	if res.ExitCode != 0 {
		return time.Time{}, &exec.CommandError{Name: "last", ExitCode: res.ExitCode, Stderr: res.Stderr}
	}
	return parseLastLogin(res.Stdout), nil
}

func parseLastLogin(out string) time.Time {
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "wtmp begins") {
			continue
		}
		if t, ok := extractLastTimestamp(line); ok {
			return t
		}
	}
	return time.Time{}
}

func extractLastTimestamp(line string) (time.Time, bool) {
	fields := strings.Fields(line)
	for i := 0; i+5 <= len(fields); i++ {
		candidate := strings.Join(fields[i:i+5], " ")
		if t, err := time.ParseInLocation(lastTimeLayout, candidate, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
