package executor

import (
	"bytes"
	"context"
	"log/slog"
	"os/exec"
	"strings"
	"testing"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func TestExecuteFlatpak_PerUserPresentNoSessions(t *testing.T) {
	if _, err := exec.LookPath("flatpak"); err != nil {
		t.Skip("flatpak is not available on this system; the per-user empty-set branch fires after the lookup")
	}
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

	out, changed, err := e.executeFlatpak(context.Background(), &pb.FlatpakParams{
		AppId:      &pb.FlatpakAppId{Value: "org.nonexistent.surely_does_not_exist_pmtest"},
		SystemWide: false,
	}, pb.DesiredState_DESIRED_STATE_PRESENT)

	if err != nil {
		t.Fatalf("expected no error on empty-session per-user install (no-op success), got: %v", err)
	}
	if changed {
		t.Errorf("expected changed=false on empty-session no-op, got changed=true")
	}
	if out == nil || !strings.Contains(out.Stdout, "no signed-in desktop users") {
		t.Errorf("expected stdout to explain the empty-set deferral, got: %#v", out)
	}
	if !strings.Contains(buf.String(), "level=WARN") {
		t.Errorf("expected WARN log on per-user install with no signed-in users, got:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "deferred until a user signs in") {
		t.Errorf("expected the warn body to explain the action will retry, got:\n%s", buf.String())
	}
}

func TestExecuteFlatpak_PerUserPresentDispatchesToLoop(t *testing.T) {
	if _, err := exec.LookPath("flatpak"); err != nil {
		t.Skip("flatpak is not available on this system")
	}
	sessions, err := desktopMgr.ActiveSessions(context.Background())
	if err != nil {
		t.Skipf("loginctl probe failed (%v) — skipping rather than asserting against an unknown session state", err)
	}
	if len(sessions) == 0 {
		t.Skip("no active desktop sessions — TestExecuteFlatpak_PerUserPresentNoSessions covers the empty-set path here")
	}

	e := NewExecutor(nil)
	out, _, _ := e.executeFlatpak(context.Background(), &pb.FlatpakParams{
		AppId:      &pb.FlatpakAppId{Value: "org.nonexistent.surely_does_not_exist_pmtest"},
		SystemWide: false,
	}, pb.DesiredState_DESIRED_STATE_PRESENT)

	if out == nil || !strings.Contains(out.Stdout, "user=") {
		t.Errorf("expected per-user fan-out output (lines tagged with user=<name>), got: %#v", out)
	}
}

func TestExecuteFlatpak_PerUserAbsentNoUsers(t *testing.T) {
	if _, err := exec.LookPath("flatpak"); err != nil {
		t.Skip("flatpak is not available on this system")
	}

	e := NewExecutor(nil)
	out, changed, err := e.executeFlatpak(context.Background(), &pb.FlatpakParams{
		AppId:      &pb.FlatpakAppId{Value: "org.nonexistent.surely_does_not_exist_pmtest"},
		SystemWide: false,
	}, pb.DesiredState_DESIRED_STATE_ABSENT)

	if err != nil {
		t.Fatalf("expected no error on already-absent per-user uninstall, got: %v", err)
	}
	if changed {
		t.Errorf("expected changed=false when nobody has the app installed, got changed=true")
	}
	if out == nil || !strings.Contains(out.Stdout, "already not installed") {
		t.Errorf("expected stdout to confirm policy is already converged, got: %#v", out)
	}
}

func TestExecuteFlatpak_SystemWideRoutesUnchanged(t *testing.T) {
	if _, err := exec.LookPath("flatpak"); err != nil {
		t.Skip("flatpak is not available on this system")
	}

	e := NewExecutor(nil)
	out, _, err := e.executeFlatpak(context.Background(), &pb.FlatpakParams{
		AppId:      &pb.FlatpakAppId{Value: "org.nonexistent.surely_does_not_exist_pmtest"},
		SystemWide: true,
	}, pb.DesiredState_DESIRED_STATE_PRESENT)

	if out != nil && strings.Contains(out.Stdout, "no signed-in") {
		t.Errorf("SystemWide=true must not enter the per-user empty-session branch; got %q", out.Stdout)
	}
	if err == nil {
		t.Error("expected real install error for nonexistent app on SystemWide=true path, got nil")
	}
}
