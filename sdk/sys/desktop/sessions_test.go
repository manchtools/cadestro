package desktop

import (
	"context"
	"errors"
	"os/user"
	"strings"
	"testing"

	"github.com/manchtools/cadestro/sdk/sys/exec"
)

func TestParseLoginctlProperties(t *testing.T) {
	in := strings.Join([]string{
		"Name=alice",
		"User=1000",
		"Type=wayland",
		"Active=yes",
		"Remote=no",
		"",
	}, "\n")

	got := parseLoginctlProperties(in)
	want := map[string]string{
		"Name":   "alice",
		"User":   "1000",
		"Type":   "wayland",
		"Active": "yes",
		"Remote": "no",
	}
	if len(got) != len(want) {
		t.Fatalf("parseLoginctlProperties: got %d keys, want %d (got=%v)", len(got), len(want), got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("key %q: got %q, want %q", k, got[k], v)
		}
	}
}

func TestParseLoginctlProperties_IgnoresMalformed(t *testing.T) {
	in := strings.Join([]string{
		"Name=alice",
		"no_equals_here",
		"=empty_key",
		"Active=yes",
	}, "\n")
	got := parseLoginctlProperties(in)
	if got["Name"] != "alice" || got["Active"] != "yes" {
		t.Errorf("legitimate keys lost when parsing malformed lines: %v", got)
	}
	if _, ok := got["no_equals_here"]; ok {
		t.Errorf("malformed line treated as a key: %v", got)
	}
	if _, ok := got[""]; ok {
		t.Errorf("empty-key line silently kept: %v", got)
	}
}

func TestIsGraphicalType(t *testing.T) {
	tests := map[string]bool{

		"x11":     true,
		"wayland": true,
		"mir":     true,

		"tty":         false,
		"unspecified": false,
		"":            false,
		"WAYLAND":     false,
	}
	for in, want := range tests {
		if got := isGraphicalType(in); got != want {
			t.Errorf("isGraphicalType(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestIsLoginctlNoLogindStderr(t *testing.T) {

	tests := map[string]bool{

		"System has not been booted with systemd as init system (PID 1). Can't operate.": true,

		"Failed to connect to bus: No such file or directory": true,
		"Failed to connect to bus: Connection refused":        true,

		"Permission denied":           false,
		"loginctl: command not found": false,
		"":                            false,
	}
	for stderr, want := range tests {
		if got := isLoginctlNoLogindStderr(stderr); got != want {
			t.Errorf("isLoginctlNoLogindStderr(%q) = %v, want %v", stderr, got, want)
		}
	}
}

var liveResultUser = &user.User{Uid: "1000", Gid: "1000", Username: "alice", HomeDir: "/home/alice"}

const graphicalSessionProps = "Name=alice\nUser=1000\nType=wayland\nActive=yes\nRemote=no\n"

func TestActiveSessions_NoLoginctl(t *testing.T) {

	stubLookPath(t, false)
	m, r := newManager(t)
	got, err := m.ActiveSessions(context.Background())
	if err != nil {
		t.Fatalf("ActiveSessions: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want no sessions, got %v", got)
	}

	if got == nil {
		t.Error("ActiveSessions must return a non-nil empty slice when loginctl is absent")
	}
	if n := len(r.Calls()); n != 0 {
		t.Errorf("loginctl absent must run nothing, but %d calls ran", n)
	}
}

func TestActiveSessions_NoLogind(t *testing.T) {

	stubLookPath(t, true)
	m, r := newManager(t)
	r.Push(exec.Result{ExitCode: 1, Stderr: "Failed to connect to bus: No such file or directory"}, nil)
	got, err := m.ActiveSessions(context.Background())
	if err != nil {
		t.Fatalf("ActiveSessions: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("no-logind must yield zero sessions, got %v", got)
	}
}

func TestActiveSessions_ListError(t *testing.T) {

	stubLookPath(t, true)
	m, r := newManager(t)
	r.Push(exec.Result{ExitCode: 1, Stderr: "Permission denied"}, nil)
	if _, err := m.ActiveSessions(context.Background()); err == nil {
		t.Error("ActiveSessions swallowed a genuine list-sessions failure")
	}
}

func TestActiveSessions_ListRunError(t *testing.T) {

	stubLookPath(t, true)
	m, r := newManager(t)
	r.Push(exec.Result{}, errors.New("boom"))
	if _, err := m.ActiveSessions(context.Background()); err == nil {
		t.Error("ActiveSessions swallowed a Runner error")
	}
}

func TestActiveSessions_FiltersAndBuilds(t *testing.T) {

	stubLookPath(t, true)
	stubLookupID(t, func(string) (*user.User, error) { return liveResultUser, nil })
	m, r := newManager(t)
	r.Push(exec.Result{Stdout: "c1 1000 alice seat0\nc2 1001 bob\n"}, nil)
	r.Push(exec.Result{Stdout: graphicalSessionProps}, nil)
	r.Push(exec.Result{Stdout: "Name=bob\nUser=1001\nType=x11\nActive=yes\nRemote=yes\n"}, nil)

	got, err := m.ActiveSessions(context.Background())
	if err != nil {
		t.Fatalf("ActiveSessions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want exactly the one local graphical session, got %d: %v", len(got), got)
	}
	s := got[0]
	want := Session{ID: "c1", Username: "alice", UID: 1000, GID: 1000, Home: "/home/alice", RuntimeDir: "/run/user/1000", Type: "wayland"}
	if s != want {
		t.Errorf("built session = %+v, want %+v", s, want)
	}

	for _, c := range r.Calls() {
		if c.Escalate {
			t.Errorf("loginctl probe must not escalate: %+v", c)
		}
		if c.Name != loginctlPath {
			t.Errorf("probe ran %q, want %q", c.Name, loginctlPath)
		}
	}
}

func TestActiveSessions_LoadErrorPropagates(t *testing.T) {

	stubLookPath(t, true)
	m, r := newManager(t)
	r.Push(exec.Result{Stdout: "c1 1000 alice\n"}, nil)
	r.Push(exec.Result{ExitCode: 1, Stderr: "Permission denied"}, nil)
	if _, err := m.ActiveSessions(context.Background()); err == nil {
		t.Error("a hard show-session failure must propagate")
	}
}

func TestLoadSession_Branches(t *testing.T) {
	props := func(name, uid, typ, active, remote string) string {
		return "Name=" + name + "\nUser=" + uid + "\nType=" + typ + "\nActive=" + active + "\nRemote=" + remote + "\n"
	}

	t.Run("run error", func(t *testing.T) {
		m, r := newManager(t)
		r.Push(exec.Result{}, errors.New("boom"))
		if _, _, err := m.loadSession(context.Background(), "c1"); err == nil {
			t.Error("want error on Runner failure")
		}
	})
	t.Run("no session race skipped", func(t *testing.T) {
		m, r := newManager(t)
		r.Push(exec.Result{ExitCode: 1, Stderr: "Failed to get session: No session 'c1'."}, nil)
		_, ok, err := m.loadSession(context.Background(), "c1")
		if err != nil || ok {
			t.Errorf("a logged-out session must skip cleanly: ok=%v err=%v", ok, err)
		}
	})
	t.Run("other non-zero exit errors", func(t *testing.T) {
		m, r := newManager(t)
		r.Push(exec.Result{ExitCode: 1, Stderr: "Permission denied"}, nil)
		if _, _, err := m.loadSession(context.Background(), "c1"); err == nil {
			t.Error("a non-race non-zero exit must error")
		}
	})
	t.Run("remote skipped", func(t *testing.T) {
		m, r := newManager(t)
		r.Push(exec.Result{Stdout: props("alice", "1000", "wayland", "yes", "yes")}, nil)
		if _, ok, _ := m.loadSession(context.Background(), "c1"); ok {
			t.Error("remote session must be filtered out")
		}
	})
	t.Run("inactive skipped", func(t *testing.T) {
		m, r := newManager(t)
		r.Push(exec.Result{Stdout: props("alice", "1000", "wayland", "no", "no")}, nil)
		if _, ok, _ := m.loadSession(context.Background(), "c1"); ok {
			t.Error("inactive session must be filtered out")
		}
	})
	t.Run("non-graphical skipped", func(t *testing.T) {
		m, r := newManager(t)
		r.Push(exec.Result{Stdout: props("alice", "1000", "tty", "yes", "no")}, nil)
		if _, ok, _ := m.loadSession(context.Background(), "c1"); ok {
			t.Error("tty session must be filtered out")
		}
	})
	t.Run("non-numeric user errors", func(t *testing.T) {
		m, r := newManager(t)
		r.Push(exec.Result{Stdout: props("alice", "notanumber", "wayland", "yes", "no")}, nil)
		if _, _, err := m.loadSession(context.Background(), "c1"); err == nil {
			t.Error("a non-numeric User= must error")
		}
	})
	t.Run("passwd miss skipped", func(t *testing.T) {
		stubLookupID(t, func(string) (*user.User, error) { return nil, errors.New("no such user") })
		m, r := newManager(t)
		r.Push(exec.Result{Stdout: props("alice", "1000", "wayland", "yes", "no")}, nil)
		_, ok, err := m.loadSession(context.Background(), "c1")
		if err != nil || ok {
			t.Errorf("a uid in logind but not passwd must skip: ok=%v err=%v", ok, err)
		}
	})
	t.Run("non-numeric gid errors", func(t *testing.T) {
		stubLookupID(t, func(uid string) (*user.User, error) {
			return &user.User{Uid: uid, Gid: "notanumber", Username: "alice", HomeDir: "/home/alice"}, nil
		})
		m, r := newManager(t)
		r.Push(exec.Result{Stdout: props("alice", "1000", "wayland", "yes", "no")}, nil)
		if _, _, err := m.loadSession(context.Background(), "c1"); err == nil {
			t.Error("a passwd entry with a non-numeric gid must error")
		}
	})
	t.Run("logind name must match passwd uid", func(t *testing.T) {
		stubLookupID(t, func(uid string) (*user.User, error) {
			return &user.User{Uid: uid, Gid: "1000", Username: "alice", HomeDir: "/home/alice"}, nil
		})
		m, r := newManager(t)
		r.Push(exec.Result{Stdout: props("mallory", "1000", "wayland", "yes", "no")}, nil)
		if _, _, err := m.loadSession(context.Background(), "c1"); err == nil {
			t.Error("mismatched logind Name and passwd username must fail closed")
		}
	})
	t.Run("success builds session", func(t *testing.T) {
		stubLookupID(t, func(string) (*user.User, error) { return liveResultUser, nil })
		m, r := newManager(t)
		r.Push(exec.Result{Stdout: props("alice", "1000", "x11", "yes", "no")}, nil)
		s, ok, err := m.loadSession(context.Background(), "c9")
		if err != nil || !ok {
			t.Fatalf("loadSession = (%+v,%v,%v), want a built session", s, ok, err)
		}
		want := Session{ID: "c9", Username: "alice", UID: 1000, GID: 1000, Home: "/home/alice", RuntimeDir: "/run/user/1000", Type: "x11"}
		if s != want {
			t.Errorf("session = %+v, want %+v", s, want)
		}
	})
}

func TestListSessionIDs_ParsesAndSkipsBlankLines(t *testing.T) {
	m, r := newManager(t)
	r.Push(exec.Result{Stdout: "c1 1000 alice seat0\n\n  c2 1001 bob \n"}, nil)
	ids, err := m.listSessionIDs(context.Background())
	if err != nil {
		t.Fatalf("listSessionIDs: %v", err)
	}
	if len(ids) != 2 || ids[0] != "c1" || ids[1] != "c2" {
		t.Errorf("ids = %v, want [c1 c2]", ids)
	}
}

func TestActiveSessions_RealLoginctl(t *testing.T) {
	if _, err := lookPath(loginctlPath); err != nil {
		t.Skipf("loginctl not on PATH (%v) — the missing-binary branch covers this", err)
	}
	r, err := exec.NewRunner(exec.Direct)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	m, err := New(r)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	sessions, err := m.ActiveSessions(context.Background())
	if err != nil {
		t.Errorf("ActiveSessions returned error on this host (regression #88): %v", err)
	}
	_ = sessions
}

func TestEnvFor_HasMinimumDesktopEnv(t *testing.T) {
	s := Session{
		Username:   "alice",
		UID:        1000,
		Home:       "/home/alice",
		RuntimeDir: "/run/user/1000",
	}
	env := EnvFor(s)

	mustContain := []string{
		"HOME=/home/alice",
		"USER=alice",
		"LOGNAME=alice",
		"XDG_RUNTIME_DIR=/run/user/1000",

		"DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus",
	}
	envSet := make(map[string]struct{}, len(env))
	for _, e := range env {
		envSet[e] = struct{}{}
	}
	for _, e := range mustContain {
		if _, ok := envSet[e]; !ok {
			t.Errorf("EnvFor missing %q\nfull env: %v", e, env)
		}
	}

	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			t.Errorf("EnvFor must not set PATH (caller picks one); got %q", e)
		}
	}
}
