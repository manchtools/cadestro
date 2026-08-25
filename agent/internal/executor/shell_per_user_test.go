package executor

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func TestRunShellScript_RunAsRootFalseNoSessions(t *testing.T) {
	sessions, err := desktopMgr.ActiveSessions(context.Background())
	if err != nil {
		t.Skipf("loginctl probe failed (%v) — skipping rather than asserting against an unknown session state", err)
	}
	if len(sessions) > 0 {
		t.Skipf("host has %d active desktop session(s) — empty-set branch not reachable here", len(sessions))
	}

	var buf bytes.Buffer
	e := NewExecutor(nil)
	e.logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	out, err := e.runShellScript(context.Background(), &pb.ShellParams{
		Script:    "echo hello",
		RunAsRoot: false,
	}, "echo hello")

	if err != nil {
		t.Fatalf("expected no error on empty-session per-user shell (no-op success), got: %v", err)
	}
	if out == nil || !strings.Contains(out.Stdout, "no signed-in desktop users") {
		t.Errorf("expected stdout to explain the empty-set deferral, got: %#v", out)
	}
	if !strings.Contains(buf.String(), "level=WARN") {
		t.Errorf("expected WARN log on per-user shell with no signed-in users, got:\n%s", buf.String())
	}
}

func TestRunShellScript_RunAsRootFalseDispatchesToLoop(t *testing.T) {
	if os.Geteuid() != 0 {

		t.Skip("runuser requires root to switch users; run this test under privileged CI")
	}
	sessions, err := desktopMgr.ActiveSessions(context.Background())
	if err != nil {
		t.Skipf("loginctl probe failed (%v)", err)
	}
	if len(sessions) == 0 {
		t.Skip("no active desktop sessions — TestRunShellScript_RunAsRootFalseNoSessions covers the empty-set path here")
	}

	e := NewExecutor(nil)
	out, err := e.runShellScript(context.Background(), &pb.ShellParams{

		Script:    "id -un",
		RunAsRoot: false,
	}, "id -un")

	if err != nil {

		t.Logf("per-user shell execution returned error (still asserting dispatch shape): %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil output from per-user dispatch")
	}
	if !strings.Contains(out.Stdout, "[user=") {
		t.Errorf("expected per-user prefix `[user=<name>] ` in merged stdout, got: %q", out.Stdout)
	}
	if strings.Contains(out.Stdout, "[user=root]") {
		t.Errorf("RunAsRoot=false must NOT impersonate root via the per-user fan-out path; output was: %q", out.Stdout)
	}
}

func TestStripHomeAndUser(t *testing.T) {
	in := []string{
		"PATH=/usr/bin:/bin",
		"HOME=/root",
		"LANG=en_US.UTF-8",
		"USER=root",
		"FLATPAK_USER_DIR=/foo",
	}
	got := stripHomeAndUser(in)
	for _, e := range got {
		if strings.HasPrefix(e, "HOME=") {
			t.Errorf("HOME entry survived strip: %q (full: %v)", e, got)
		}
		if strings.HasPrefix(e, "USER=") {
			t.Errorf("USER entry survived strip: %q (full: %v)", e, got)
		}
	}
	want := []string{"PATH=/usr/bin:/bin", "LANG=en_US.UTF-8", "FLATPAK_USER_DIR=/foo"}
	if len(got) != len(want) {
		t.Fatalf("got %d entries after strip, want %d (got=%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d: got %q, want %q", i, got[i], want[i])
		}
	}
}
