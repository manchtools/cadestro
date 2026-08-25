package desktop

import (
	"strings"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

// EnvFor builds the minimum environment a user-scoped command needs
// to behave like it was launched from inside that user's desktop
// session: HOME, USER, LOGNAME, XDG_RUNTIME_DIR, plus DBUS_SESSION_BUS_ADDRESS
// (so commands that talk to the user's session bus — Flatpak's
// notification path, GNOME settings, etc. — find it without falling
// back to a fresh autolaunched bus).
//
// PATH is not added here — callers append it (via UserPath) so they can
// pick a curated value rather than inherit the agent's PATH; RunAsRunner
// passes UserPath(s) as the trusted child PATH.
func EnvFor(s Session) []string {
	return []string{
		"HOME=" + s.Home,
		"USER=" + s.Username,
		"LOGNAME=" + s.Username,
		"XDG_RUNTIME_DIR=" + s.RuntimeDir,
		"DBUS_SESSION_BUS_ADDRESS=unix:path=" + s.RuntimeDir + "/bus",
	}
}

// UserPath returns the curated PATH a per-user command should run with.
// It is built from the session — never from the agent's (root's) PATH —
// so a user-scoped command does not inherit root-only entries, and the
// user's own bin dirs come first so ~/.local/bin shadows a system binary
// of the same name. sbin dirs are included because usr-merged distros
// put them on every user's default PATH and a user running an sbin
// binary does so unprivileged (their own UID); excluding them only
// breaks command resolution without any privilege benefit.
func UserPath(s Session) string {
	return strings.Join([]string{
		s.Home + "/.local/bin",
		s.Home + "/bin",
		"/usr/local/bin",
		"/usr/bin",
		"/bin",
		"/usr/local/sbin",
		"/usr/sbin",
		"/sbin",
	}, ":")
}

func validateExtraEnv(extraEnv []string) error {
	filtered := make([]string, 0, len(extraEnv))
	for _, e := range extraEnv {
		if key, _, ok := strings.Cut(e, "="); ok && key == "PATH" {
			continue
		}
		filtered = append(filtered, e)
	}
	return sysexec.ValidateCommandEnv(filtered)
}
