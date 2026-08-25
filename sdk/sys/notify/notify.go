// Package notify sends system-wide notifications to logged-in users through an
// injected exec.Runner. It uses wall for terminal sessions and notify-send for
// graphical sessions. Methods return an aggregated error of every delivery that
// ran and failed; an absent capability (no notify-send, no D-Bus socket, no
// sessions) is a graceful skip, not an error. The SDK surfaces failures; the
// caller decides whether to ignore them (notifications need not block an action).
//
//	r, _ := exec.NewRunner(exec.Sudo)
//	n, _ := notify.New(r)
//	n.NotifyAll(ctx, "Maintenance", "Reboot in 5 minutes")
package notify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	osexec "os/exec"
	"strconv"
	"strings"

	"github.com/manchtools/cadestro/sdk/sys/exec"
)

var (
	lookPath   = osexec.LookPath
	statSocket = os.Stat
)

type session struct {
	id   string
	user string
	uid  int
	typ  string
}

// Manager sends notifications to logged-in users. Methods return an aggregated
// error of deliveries that ran and failed; an absent capability is a graceful
// skip (nil).
type Manager interface {
	// NotifyAll notifies every logged-in user: a wall broadcast to terminal
	// sessions and a desktop notification to graphical ones. It returns an
	// aggregated error of every attempt that ran and failed; an absent capability
	// (no notify-send, no D-Bus socket, no sessions) is a graceful skip, not an
	// error. The SDK surfaces failures; the caller decides whether to ignore them.
	NotifyAll(ctx context.Context, title, message string) error
	// NotifyUsers notifies the named users only (wall still broadcasts, since it
	// has no per-user target). Error semantics match NotifyAll.
	NotifyUsers(ctx context.Context, usernames []string, title, message string) error
}

const maxNotificationField = 4096

// New returns a Manager driven by runner. A nil runner is rejected.
func New(runner exec.Runner) (Manager, error) {
	if runner == nil {
		return nil, fmt.Errorf("notify: %w", exec.ErrRunnerRequired)
	}
	return &notifier{r: runner}, nil
}

type notifier struct {
	r exec.Runner
}

func (n *notifier) NotifyAll(ctx context.Context, title, message string) error {
	if err := validateNotification(title, message); err != nil {
		return err
	}
	return errors.Join(
		n.sendWall(ctx, fmt.Sprintf("%s: %s", title, message)),
		n.sendDesktopNotifications(ctx, title, message, nil),
	)
}

func (n *notifier) NotifyUsers(ctx context.Context, usernames []string, title, message string) error {
	if err := validateNotification(title, message); err != nil {
		return err
	}
	for _, u := range usernames {
		if err := validateUsername(u); err != nil {
			return err
		}
	}
	filter := make(map[string]bool, len(usernames))
	for _, u := range usernames {
		filter[u] = true
	}
	return errors.Join(
		n.sendWall(ctx, fmt.Sprintf("%s: %s", title, message)),
		n.sendDesktopNotifications(ctx, title, message, filter),
	)
}

func validateNotification(title, message string) error {
	if err := validateField("notification title", title); err != nil {
		return err
	}
	return validateField("notification message", message)
}

func validateField(kind, s string) error {
	if containsControl(s) {
		return fmt.Errorf("invalid %s: must not contain control characters", kind)
	}
	if len(s) > maxNotificationField {
		return fmt.Errorf("invalid %s: must not exceed %d bytes", kind, maxNotificationField)
	}
	return nil
}

func validateUsername(u string) error {
	if containsControl(u) {
		return fmt.Errorf("invalid username %q: must not contain control characters", u)
	}
	return nil
}

func containsControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func (n *notifier) sendWall(ctx context.Context, message string) error {
	res, err := n.r.Run(ctx, exec.Command{
		Name:     "wall",
		Stdin:    strings.NewReader(message),
		Escalate: true,
	})
	if err != nil {
		return fmt.Errorf("wall notification: %w", err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("wall notification failed: exit %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr))
	}
	return nil
}

func (n *notifier) sendDesktopNotifications(ctx context.Context, title, message string, userFilter map[string]bool) error {
	if _, err := lookPath("notify-send"); err != nil {
		slog.Warn("notify-send not available, skipping desktop notifications")
		return nil
	}
	sessions, err := n.listGraphicalSessions(ctx)
	if err != nil {
		return err
	}
	slog.Info("discovered graphical sessions for desktop notification", "count", len(sessions))
	var errs []error
	for _, s := range sessions {
		if userFilter != nil && !userFilter[s.user] {
			continue
		}
		if e := n.sendDesktopNotification(ctx, s, title, message); e != nil {
			errs = append(errs, e)
		}
	}
	return errors.Join(errs...)
}

func (n *notifier) listGraphicalSessions(ctx context.Context) ([]session, error) {
	res, err := n.r.Run(ctx, exec.Command{Name: "loginctl", Args: []string{"list-sessions", "--no-legend"}, Escalate: true})
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("list sessions failed: exit %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr))
	}

	var sessions []session
	for _, sessionID := range parseLoginctlListSessions(res.Stdout) {

		info, err := n.r.Run(ctx, exec.Command{
			Name:     "loginctl",
			Args:     []string{"show-session", sessionID, "-p", "Type", "-p", "Name", "-p", "User"},
			Escalate: true,
		})
		if err != nil || info.ExitCode != 0 {
			continue
		}
		s, ok := parseLoginctlShowSession(sessionID, info.Stdout)
		if !ok {
			continue
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

func parseLoginctlListSessions(stdout string) []string {
	var ids []string
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		ids = append(ids, fields[0])
	}
	return ids
}

func parseLoginctlShowSession(sessionID, stdout string) (session, bool) {
	props := map[string]string{}
	for _, line := range strings.Split(stdout, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		props[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	typ := props["Type"]
	user := props["Name"]
	uidStr, hasUID := props["User"]
	if !hasUID || user == "" {
		return session{}, false
	}
	uid, err := strconv.Atoi(uidStr)
	if err != nil {
		return session{}, false
	}
	if typ != "x11" && typ != "wayland" && typ != "mir" {
		return session{}, false
	}
	return session{id: sessionID, user: user, uid: uid, typ: typ}, true
}

func (n *notifier) sendDesktopNotification(ctx context.Context, s session, title, message string) error {
	socketPath := fmt.Sprintf("/run/user/%d/bus", s.uid)
	if _, err := statSocket(socketPath); err != nil {

		slog.Warn("DBUS socket not found, skipping desktop notification", "user", s.user, "path", socketPath)
		return nil
	}
	dbusAddr := "unix:path=" + socketPath

	res, err := n.r.Run(ctx, exec.Command{
		Name: "env",
		Args: []string{
			"DBUS_SESSION_BUS_ADDRESS=" + dbusAddr,
			"runuser", "-u", s.user, "--",
			"notify-send", "-u", "critical", "-a", "Cadestro", "-i", "dialog-warning",
			title, message,
		},
		Escalate: true,
	})
	if err != nil {
		return fmt.Errorf("desktop notification to %s: %w", s.user, err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("desktop notification to %s failed: exit %d: %s", s.user, res.ExitCode, strings.TrimSpace(res.Stderr))
	}
	return nil
}
