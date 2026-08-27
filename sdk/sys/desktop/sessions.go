package desktop

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

// Session is a single active graphical desktop session.
//
// All fields are populated from systemd-logind (loginctl) plus a
// passwd lookup for Home / GID. Callers should treat the struct as
// immutable — the SDK constructs it once per ActiveSessions call.
type Session struct {
	// ID is the systemd-logind session ID (e.g. "c1", "2"). Stable for
	// the lifetime of the session; not stable across logout/login.
	ID string
	// Username is the Linux account name (`loginctl show-session ... -p Name`).
	Username string
	// UID is the numeric user ID. Used to derive XDG_RUNTIME_DIR.
	UID int
	// GID is the user's primary group ID, looked up via os/user.
	GID int
	// Home is the user's home directory, looked up via os/user. Required
	// for Flatpak --user installs (which resolve everything against
	// $HOME) and for shell scripts that expect a sane working directory.
	Home string
	// RuntimeDir is /run/user/<UID>. Populated unconditionally — the
	// caller decides whether to verify it exists before invoking a
	// command that needs it (e.g. anything touching DBus).
	RuntimeDir string
	// Type is the session type (`x11`, `wayland`, `mir`). Always one of
	// the graphical types because non-graphical sessions are filtered
	// out before construction.
	Type string
}

// ActiveSessions returns every active local graphical session on the
// host, ready for fanning a user-scoped command out to each.
//
// Returns an empty slice (not an error) when:
//   - loginctl is missing (host without systemd-logind)
//   - no graphical sessions are active (machine is at login screen,
//     headless server, etc.)
//
// Returns an error only when loginctl is present but its output is
// malformed or the per-session detail probe fails for a reason other
// than the session disappearing mid-call.
func (m *manager) ActiveSessions(ctx context.Context) ([]Session, error) {
	if _, pathErr := lookPath(loginctlPath); pathErr != nil {

		return []Session{}, nil
	}

	ids, err := m.listSessionIDs(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]Session, 0, len(ids))
	for _, id := range ids {
		s, ok, err := m.loadSession(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("loginctl show-session %q: %w", id, err)
		}
		if !ok {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

func (m *manager) listSessionIDs(ctx context.Context) ([]string, error) {
	res, err := m.r.Run(ctx, sysexec.Command{Name: loginctlPath, Args: []string{"list-sessions", "--no-legend"}})
	if err != nil {
		return nil, fmt.Errorf("loginctl list-sessions: %w", err)
	}
	if res.ExitCode != 0 {
		if isLoginctlNoLogindStderr(res.Stderr) {
			return nil, nil
		}
		return nil, fmt.Errorf("loginctl list-sessions failed (exit %d): %s", res.ExitCode, strings.TrimSpace(res.Stderr))
	}
	var ids []string
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		ids = append(ids, strings.Fields(line)[0])
	}
	return ids, nil
}

func (m *manager) loadSession(ctx context.Context, id string) (Session, bool, error) {
	res, err := m.r.Run(ctx, sysexec.Command{Name: loginctlPath, Args: []string{
		"show-session", id,
		"--property=Name",
		"--property=User",
		"--property=Type",
		"--property=Active",
		"--property=Remote",
	}})
	if err != nil {
		return Session{}, false, fmt.Errorf("loginctl show-session: %w", err)
	}
	if res.ExitCode != 0 {

		if strings.Contains(res.Stderr, "No session") {
			return Session{}, false, nil
		}
		return Session{}, false, fmt.Errorf("exit %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr))
	}

	props := parseLoginctlProperties(res.Stdout)
	if props["Remote"] != "no" {
		return Session{}, false, nil
	}
	if props["Active"] != "yes" {
		return Session{}, false, nil
	}
	if !isGraphicalType(props["Type"]) {
		return Session{}, false, nil
	}

	uidStr := props["User"]
	uid, err := strconv.Atoi(uidStr)
	if err != nil {
		return Session{}, false, fmt.Errorf("loginctl returned non-numeric User=%q for session %q", uidStr, id)
	}

	u, err := lookupID(uidStr)
	if err != nil {

		return Session{}, false, nil
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return Session{}, false, fmt.Errorf("non-numeric GID %q for user %q", u.Gid, u.Username)
	}

	if props["Name"] != u.Username {
		return Session{}, false, fmt.Errorf(
			"loginctl Name=%q disagrees with passwd username %q for uid %d in session %q",
			props["Name"], u.Username, uid, id)
	}

	return Session{
		ID:         id,
		Username:   u.Username,
		UID:        uid,
		GID:        gid,
		Home:       u.HomeDir,
		RuntimeDir: "/run/user/" + uidStr,
		Type:       props["Type"],
	}, true, nil
}

func parseLoginctlProperties(s string) map[string]string {
	out := make(map[string]string, 8)
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		out[line[:eq]] = line[eq+1:]
	}
	return out
}

func isGraphicalType(t string) bool {
	switch t {
	case "x11", "wayland", "mir":
		return true
	default:
		return false
	}
}

func isLoginctlNoLogindStderr(stderr string) bool {
	switch {
	case strings.Contains(stderr, "has not been booted with systemd"):
		return true
	case strings.Contains(stderr, "Failed to connect to bus"):
		return true
	default:
		return false
	}
}
