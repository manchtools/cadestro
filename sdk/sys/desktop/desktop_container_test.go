//go:build container

package desktop

import (
	"context"
	"os"
	osexec "os/exec"
	"os/user"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

func requireUseradd(t *testing.T) {
	t.Helper()
	if _, err := osexec.LookPath("useradd"); err != nil {
		t.Skip("useradd not on PATH; account-based desktop tests not exercisable")
	}
}

func mkUser(t *testing.T, name, homeDir string) *user.User {
	t.Helper()
	_ = osexec.Command("userdel", "-r", name).Run()
	if out, err := osexec.Command("useradd", "-m", "-d", homeDir, "-s", "/bin/bash", name).CombinedOutput(); err != nil {
		t.Fatalf("useradd %s: %v\n%s", name, err, out)
	}
	t.Cleanup(func() { _ = osexec.Command("userdel", "-r", name).Run() })
	u, err := user.Lookup(name)
	if err != nil {
		t.Fatalf("lookup %s after useradd: %v", name, err)
	}
	return u
}

func realDesktop(t *testing.T, opts ...Option) Manager {
	t.Helper()
	r, err := sysexec.NewRunner(sysexec.Direct)
	if err != nil {
		t.Fatalf("NewRunner(Direct): %v", err)
	}
	m, err := New(r, opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return m
}

func deskCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestHomeUsers_Container(t *testing.T) {
	requireUseradd(t)
	root := t.TempDir()
	alice := mkUser(t, "cadestrohomealice", filepath.Join(root, "cadestrohomealice"))
	_ = mkUser(t, "cadestrohomebob", filepath.Join(root, "cadestrohomebob"))

	if err := os.Mkdir(filepath.Join(root, "ghost"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".hidden"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "lost+found"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := realDesktop(t, WithHomeRoot(root)).HomeUsers(deskCtx(t))
	if err != nil {
		t.Fatalf("HomeUsers: %v", err)
	}
	names := make([]string, 0, len(got))
	for _, s := range got {
		names = append(names, s.Username)
	}
	slices.Sort(names)
	if want := []string{"cadestrohomealice", "cadestrohomebob"}; !slices.Equal(names, want) {
		t.Errorf("HomeUsers = %v, want %v (ghost/.hidden/lost+found must be skipped)", names, want)
	}

	for _, s := range got {
		if s.Username == "cadestrohomealice" {
			if strconv.Itoa(s.UID) != alice.Uid || s.Home != alice.HomeDir {
				t.Errorf("alice session = uid %d home %q, want uid %s home %q", s.UID, s.Home, alice.Uid, alice.HomeDir)
			}
		}
	}
}

func TestRunAsRunner_Container(t *testing.T) {
	requireUseradd(t)
	home := filepath.Join(t.TempDir(), "cadestrorunas")
	u := mkUser(t, "cadestrorunas", home)
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	s := Session{Username: "cadestrorunas", UID: uid, GID: gid, Home: u.HomeDir, RuntimeDir: "/run/user/" + u.Uid}
	base, err := sysexec.NewRunner(sysexec.Direct)
	if err != nil {
		t.Fatalf("NewRunner(Direct): %v", err)
	}
	ru, err := RunAsRunner(base, s)
	if err != nil {
		t.Fatalf("RunAsRunner: %v", err)
	}
	ctx := deskCtx(t)

	res, err := ru.Run(ctx, sysexec.Command{Name: "id", Args: []string{"-un"}})
	if err != nil {
		t.Fatalf("run id -un as cadestrorunas: %v", err)
	}
	if got := strings.TrimSpace(res.Stdout); got != "cadestrorunas" {
		t.Errorf("id -un = %q, want cadestrorunas (privilege was not dropped)", got)
	}

	res2, err := ru.Run(ctx, sysexec.Command{Name: "sh", Args: []string{"-c", `printf '%s|%s|%s' "$HOME" "$USER" "$PATH"`}})
	if err != nil {
		t.Fatalf("run env probe as cadestrorunas: %v", err)
	}
	parts := strings.SplitN(strings.TrimRight(res2.Stdout, "\n"), "|", 3)
	if len(parts) != 3 || parts[0] != u.HomeDir || parts[1] != "cadestrorunas" {
		t.Errorf("env = %v, want HOME=%q USER=cadestrorunas", parts, u.HomeDir)
	}
	if !strings.HasPrefix(parts[2], u.HomeDir+"/.local/bin:") {
		t.Errorf("PATH = %q, want it to start with the user's ~/.local/bin", parts[2])
	}

	res3, err := ru.Run(ctx, sysexec.Command{Name: "pwd", Dir: u.HomeDir})
	if err != nil {
		t.Fatalf("run pwd as cadestrorunas with Dir: %v", err)
	}
	if got := strings.TrimSpace(res3.Stdout); got != u.HomeDir {
		t.Errorf("pwd = %q, want %q (Command.Dir was not honored)", got, u.HomeDir)
	}
}

func TestUsersWithFlatpakInstall_Container(t *testing.T) {
	requireUseradd(t)
	const appID = "org.test.CadestroApp"
	root := t.TempDir()
	flat := mkUser(t, "cadestroflatuser", filepath.Join(root, "cadestroflatuser"))
	_ = mkUser(t, "cadestroplainuser", filepath.Join(root, "cadestroplainuser"))
	if err := os.MkdirAll(filepath.Join(flat.HomeDir, ".local/share/flatpak/app", appID), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := realDesktop(t, WithHomeRoot(root)).UsersWithFlatpakInstall(deskCtx(t), appID)
	if err != nil {
		t.Fatalf("UsersWithFlatpakInstall: %v", err)
	}
	if len(got) != 1 || got[0].Username != "cadestroflatuser" {
		names := make([]string, len(got))
		for i, s := range got {
			names[i] = s.Username
		}
		t.Errorf("UsersWithFlatpakInstall = %v, want [cadestroflatuser] only", names)
	}
}

func TestActiveSessions_NoLoginctl_Container(t *testing.T) {
	if _, err := osexec.LookPath("loginctl"); err == nil {
		t.Skip("loginctl present here; the absent-path assertion does not apply")
	}
	got, err := realDesktop(t).ActiveSessions(deskCtx(t))
	if err != nil {
		t.Fatalf("ActiveSessions with loginctl absent must not error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ActiveSessions = %v, want empty when loginctl is absent", got)
	}
}
