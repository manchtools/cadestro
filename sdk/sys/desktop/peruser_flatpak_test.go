package desktop_test

import (
	"context"
	"strings"
	"testing"

	"github.com/manchtools/cadestro/sdk/pkg"
	"github.com/manchtools/cadestro/sdk/sys/desktop"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/exec/exectest"
)

func TestPerUserFlatpak_Composition(t *testing.T) {
	base := exectest.New(sysexec.Direct)
	base.Push(sysexec.Result{}, nil)
	s := desktop.Session{Username: "alice", UID: 1000, Home: "/home/alice", RuntimeDir: "/run/user/1000"}

	ru, err := desktop.RunAsRunner(base, s)
	if err != nil {
		t.Fatalf("RunAsRunner: %v", err)
	}
	fp, err := pkg.NewUserFlatpak(ru)
	if err != nil {
		t.Fatalf("pkg.NewUserFlatpak: %v", err)
	}
	if _, err := fp.Install(context.Background(), "flathub", "org.x.App"); err != nil {
		t.Fatalf("Install: %v", err)
	}

	c := base.Calls()[0]
	got := strings.Join(append([]string{c.Name}, c.Args...), " ")
	if !strings.HasPrefix(got, "/usr/sbin/runuser -u alice -- /usr/bin/env ") {
		t.Errorf("install did not run as alice via runuser: %q", got)
	}
	if !strings.Contains(got, "HOME=/home/alice") || !strings.Contains(got, "XDG_RUNTIME_DIR=/run/user/1000") {
		t.Errorf("missing alice's session env: %q", got)
	}
	if !strings.HasSuffix(got, "flatpak install -y --noninteractive --user flathub org.x.App") {
		t.Errorf("not the expected per-user flatpak install (remote + --user): %q", got)
	}
	if c.Escalate {
		t.Error("per-user flatpak must not escalate (runuser already dropped privilege)")
	}
}
