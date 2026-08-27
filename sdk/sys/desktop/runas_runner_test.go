package desktop

import (
	"context"
	"slices"
	"strings"
	"testing"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/exec/exectest"
)

func TestRunAsRunner_WrapsCommandAsUser(t *testing.T) {
	base := exectest.New(sysexec.Direct)
	base.Push(sysexec.Result{}, nil)
	s := Session{Username: "alice", UID: 1000, Home: "/home/alice", RuntimeDir: "/run/user/1000"}
	ra, err := RunAsRunner(base, s)
	if err != nil {
		t.Fatalf("RunAsRunner: %v", err)
	}
	if _, err := ra.Run(context.Background(), sysexec.Command{Name: "flatpak", Args: []string{"install", "--user", "org.x.App"}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	c := base.Calls()[0]
	got := strings.Join(append([]string{c.Name}, c.Args...), " ")

	want := runuserPath + " -u alice -- " + envPath +
		" HOME=/home/alice USER=alice LOGNAME=alice XDG_RUNTIME_DIR=/run/user/1000" +
		" DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus" +
		" PATH=" + UserPath(s) + " flatpak install --user org.x.App"
	if got != want {
		t.Errorf("wrapped command:\n got=%q\nwant=%q", got, want)
	}
	if c.Escalate {
		t.Error("runuser-from-root is a privilege DROP; the wrapped command must not also escalate")
	}
}

func TestRunAsRunner_CallerPathDropped(t *testing.T) {
	base := exectest.New(sysexec.Direct)
	base.Push(sysexec.Result{}, nil)
	s := Session{Username: "alice", UID: 1000, Home: "/home/alice", RuntimeDir: "/run/user/1000"}
	ra, _ := RunAsRunner(base, s)
	if _, err := ra.Run(context.Background(), sysexec.Command{Name: "flatpak", Env: []string{"PATH=/attacker/bin"}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	args := base.Calls()[0].Args
	if slices.Contains(args, "PATH=/attacker/bin") {
		t.Errorf("caller PATH must be dropped; args=%v", args)
	}
	var lastPath string
	for _, a := range args {
		if strings.HasPrefix(a, "PATH=") {
			lastPath = a
		}
	}
	if lastPath != "PATH="+UserPath(s) {
		t.Errorf("curated PATH must win; effective=%q", lastPath)
	}
}

func TestRunAsRunner_PropagatesCallerDir(t *testing.T) {
	base := exectest.New(sysexec.Direct)
	base.Push(sysexec.Result{}, nil)
	s := Session{Username: "alice", UID: 1000, Home: "/home/alice", RuntimeDir: "/run/user/1000"}
	ra, _ := RunAsRunner(base, s)
	if _, err := ra.Run(context.Background(), sysexec.Command{Name: "script.sh", Dir: "/home/alice/work"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := base.Calls()[0].Dir; got != "/home/alice/work" {
		t.Errorf("wrapped command Dir = %q, want /home/alice/work (caller working dir was dropped)", got)
	}
}

func TestRunAsRunner_Rejects(t *testing.T) {
	if _, err := RunAsRunner(nil, Session{Username: "alice"}); err == nil {
		t.Error("nil base Runner must be rejected")
	}
	if _, err := RunAsRunner(exectest.New(sysexec.Direct), Session{}); err == nil {
		t.Error("a session with no Username must be rejected (would silently run as the agent's UID)")
	}
}

func TestRunAsRunner_ScreensHijackEnv(t *testing.T) {
	base := exectest.New(sysexec.Direct)
	s := Session{Username: "alice", UID: 1000, Home: "/home/alice", RuntimeDir: "/run/user/1000"}
	ra, _ := RunAsRunner(base, s)
	_, err := ra.Run(context.Background(), sysexec.Command{Name: "flatpak", Args: []string{"list"}, Env: []string{"LD_PRELOAD=/tmp/evil.so"}})
	if err == nil {
		t.Fatal("LD_PRELOAD in the command env must be rejected")
	}
	if len(base.Calls()) != 0 {
		t.Error("a rejected hijack env must run nothing")
	}
}
