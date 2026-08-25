package exec

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/go-cmd/cmd"
)

func readChildPID(t *testing.T, pidFile string) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		b, err := os.ReadFile(pidFile)
		if err == nil {
			if pid, perr := strconv.Atoi(strings.TrimSpace(string(b))); perr == nil && pid > 0 {
				return pid
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("child never wrote its PID to %s", pidFile)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func assertProcessGroupGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		err := syscall.Kill(-pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("process group %d still alive after grace (kill(-pid,0)=%v) — SIGKILL never delivered", pid, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func sigtermIgnoringScript(pidFile string) string {
	return fmt.Sprintf(`echo $$ > %s; trap "" TERM; while true; do sleep 30; done`, pidFile)
}

func TestAwaitStatusOrKill_DStateFallback(t *testing.T) {
	restore := killGrace
	killGrace = 50 * time.Millisecond
	defer func() { killGrace = restore }()

	c := cmd.NewCmd("sleep", "60")
	_ = c.Start()
	never := make(chan cmd.Status)

	start := time.Now()
	_ = awaitStatusOrKill(c, never)
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("awaitStatusOrKill blocked %v; the bounded D-state fallback did not fire", elapsed)
	}
}

func TestRunner_WellBehavedChildReapsOnSIGTERM(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := directRunner(t).Run(ctx, Command{Name: "sleep", Args: []string{"30"}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("well-behaved child took %v to reap on SIGTERM", elapsed)
	}
}
