package desktop

import (
	"context"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"testing"
)

func currentUserSession(t *testing.T) (user.User, int, int) {
	t.Helper()
	u, err := user.Current()
	if err != nil {
		t.Skipf("cannot resolve current user: %v", err)
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		t.Skipf("non-numeric UID for current user: %v", err)
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		t.Skipf("non-numeric GID for current user: %v", err)
	}
	return *u, uid, gid
}

func TestHomeUsers_EmptyHomeRoot(t *testing.T) {
	m, _ := newManager(t, WithHomeRoot(t.TempDir()))
	got, err := m.HomeUsers(context.Background())
	if err != nil {
		t.Fatalf("unexpected error on empty /home: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("empty /home should return zero users, got %d: %v", len(got), got)
	}
}

func TestHomeUsers_MissingHomeRoot(t *testing.T) {

	m, _ := newManager(t, WithHomeRoot(filepath.Join(t.TempDir(), "does-not-exist")))
	got, err := m.HomeUsers(context.Background())
	if err != nil {
		t.Fatalf("missing /home should not error, got: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected zero users for missing /home, got %d", len(got))
	}

	if got == nil {
		t.Error("HomeUsers must return a non-nil empty slice for a missing home root")
	}
}

func TestHomeUsers_UnreadableHomeRoot(t *testing.T) {

	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses directory permissions; cannot exercise the unreadable path")
	}
	root := filepath.Join(t.TempDir(), "noperm")
	if err := os.Mkdir(root, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })
	m, _ := newManager(t, WithHomeRoot(root))
	if _, err := m.HomeUsers(context.Background()); err == nil {
		t.Error("an unreadable home root must surface an error, not return empty")
	}
}

func TestHomeUsers_SkipsNonDirectoryAndDotPrefixed(t *testing.T) {
	root := t.TempDir()

	if err := os.WriteFile(filepath.Join(root, "shared.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{".ecryptfs", ".pwd.lock", "lost+found"} {
		if err := os.Mkdir(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.Mkdir(filepath.Join(root, "_definitely_not_a_real_user_name_pmtest"), 0o755); err != nil {
		t.Fatal(err)
	}

	m, _ := newManager(t, WithHomeRoot(root))
	got, err := m.HomeUsers(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("non-real entries should produce zero users, got %d: %v", len(got), got)
	}
}

func TestHomeUsers_SkipsNonNumericUIDOrGID(t *testing.T) {

	root := t.TempDir()
	for _, d := range []string{"baduid", "badgid"} {
		if err := os.Mkdir(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	stubLookupUser(t, func(name string) (*user.User, error) {
		switch name {
		case "baduid":
			return &user.User{Username: name, Uid: "not-a-number", Gid: "1000", HomeDir: "/h"}, nil
		case "badgid":
			return &user.User{Username: name, Uid: "1000", Gid: "not-a-number", HomeDir: "/h"}, nil
		}
		return nil, errors.New("no such user")
	})
	m, _ := newManager(t, WithHomeRoot(root))
	got, err := m.HomeUsers(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("non-numeric uid/gid entries must be skipped, got %v", got)
	}
}

func TestHomeUsers_ResolvesRealAccount(t *testing.T) {
	u, uid, gid := currentUserSession(t)

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, u.Username), 0o755); err != nil {
		t.Fatal(err)
	}

	m, _ := newManager(t, WithHomeRoot(root))
	got, err := m.HomeUsers(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 user, got %d: %v", len(got), got)
	}
	s := got[0]
	if s.Username != u.Username {
		t.Errorf("Username: got %q, want %q", s.Username, u.Username)
	}
	if s.UID != uid {
		t.Errorf("UID: got %d, want %d", s.UID, uid)
	}
	if s.GID != gid {
		t.Errorf("GID: got %d, want %d", s.GID, gid)
	}

	if s.Home != u.HomeDir {
		t.Errorf("Home: got %q, want %q (the os/user lookup is authoritative, not the directory layout)",
			s.Home, u.HomeDir)
	}
	if s.RuntimeDir != "/run/user/"+u.Uid {
		t.Errorf("RuntimeDir: got %q, want %q", s.RuntimeDir, "/run/user/"+u.Uid)
	}
}

func TestUsersWithFlatpakInstall_RequiresAppID(t *testing.T) {
	m, _ := newManager(t)
	if _, err := m.UsersWithFlatpakInstall(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty appID — guards against silently uninstalling for everyone")
	}
}

func TestUsersWithFlatpakInstall_PropagatesHomeUsersError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses directory permissions")
	}
	root := filepath.Join(t.TempDir(), "noperm")
	if err := os.Mkdir(root, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })
	m, _ := newManager(t, WithHomeRoot(root))
	if _, err := m.UsersWithFlatpakInstall(context.Background(), "com.example.App"); err == nil {
		t.Error("an unreadable home root must fail the flatpak enumeration, not return empty")
	}
}

func flatpakFixture(t *testing.T) (Manager, string) {
	t.Helper()
	root := t.TempDir()
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "alice"), 0o755); err != nil {
		t.Fatal(err)
	}
	stubLookupUser(t, func(string) (*user.User, error) {
		return &user.User{Username: "alice", Uid: "1000", Gid: "1000", HomeDir: home}, nil
	})
	m, _ := newManager(t, WithHomeRoot(root))
	return m, home
}

func TestUsersWithFlatpakInstall_ReturnsUsersWithTheApp(t *testing.T) {
	const appID = "com.example.CadestroTest"
	m, home := flatpakFixture(t)
	appDir := filepath.Join(home, ".local", "share", "flatpak", "app", appID)
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := m.UsersWithFlatpakInstall(context.Background(), appID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Username != "alice" {
		t.Fatalf("want [alice] (the user with the app installed), got %v", got)
	}
}

func TestUsersWithFlatpakInstall_ExcludesUsersWithoutTheApp(t *testing.T) {
	const appID = "com.example.CadestroTest"
	m, _ := flatpakFixture(t)
	got, err := m.UsersWithFlatpakInstall(context.Background(), appID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("a user without the per-user install must be excluded, got %v", got)
	}
}
