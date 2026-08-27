//go:build container

package terminal

import (
	"bytes"
	"context"
	osexec "os/exec"
	"strings"
	"testing"
	"time"
)

func TestOpenRunsShellAsTargetUser_Container(t *testing.T) {
	if _, err := osexec.LookPath("useradd"); err != nil {
		t.Skip("useradd not on PATH")
	}
	const u = "cadestrottytest"
	_ = osexec.Command("userdel", "-r", u).Run()
	if out, err := osexec.Command("useradd", "-m", "-s", "/bin/bash", u).CombinedOutput(); err != nil {
		t.Skipf("cannot create test user (need root?): %v\n%s", err, out)
	}
	t.Cleanup(func() { _ = osexec.Command("userdel", "-r", u).Run() })

	m, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sess, err := m.Open(ctx, SessionConfig{User: u})
	if err != nil {
		t.Fatalf("Open as %q: %v", u, err)
	}
	defer sess.Close()

	if _, err := sess.Write([]byte("printf 'CADESTRO_USER:%s\\n' \"$(id -un)\"\nexit\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		b := make([]byte, 4096)
		for {
			n, rerr := sess.Read(b)
			if n > 0 {
				buf.Write(b[:n])
			}
			if rerr != nil {
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatalf("timed out reading PTY output; got so far:\n%s", buf.String())
	}

	if marker := "CADESTRO_USER:" + u; !strings.Contains(buf.String(), marker) {
		t.Errorf("shell did not run as %q (sentinel %q absent from PTY output):\n%s", u, marker, buf.String())
	}
}
