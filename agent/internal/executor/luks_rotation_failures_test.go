package executor

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/manchtools/cadestro/agent/internal/store"
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func TestRecordLuksTimestampFailure_EscalatesAtThreshold(t *testing.T) {
	var buf bytes.Buffer
	e := &Executor{logger: slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), now: time.Now}

	const actionID = "01HXTEST0000000000000ABCDE"
	for i := 1; i <= luksTimestampFailureThreshold+1; i++ {
		e.recordLuksTimestampFailure(actionID, "post_rotation", errors.New("disk full"))
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if got, want := len(lines), luksTimestampFailureThreshold+1; got != want {
		t.Fatalf("expected %d log lines, got %d:\n%s", want, got, buf.String())
	}

	for i, line := range lines {
		switch {
		case i < luksTimestampFailureThreshold-1:

			if !strings.Contains(line, "level=WARN") {
				t.Errorf("line %d (consecutive=%d) expected WARN, got: %s", i+1, i+1, line)
			}
			if !strings.Contains(line, "consecutive_failures="+itoa(i+1)) {
				t.Errorf("line %d expected consecutive_failures=%d, got: %s", i+1, i+1, line)
			}
		default:

			if !strings.Contains(line, "level=ERROR") {
				t.Errorf("line %d (consecutive=%d) expected ERROR, got: %s", i+1, i+1, line)
			}
			if !strings.Contains(line, "consecutive_failures="+itoa(i+1)) {
				t.Errorf("line %d expected consecutive_failures=%d, got: %s", i+1, i+1, line)
			}
			if !strings.Contains(line, "rotation may hot-loop") {
				t.Errorf("line %d expected hot-loop hint in error msg, got: %s", i+1, line)
			}
		}
	}
}

func TestClearLuksTimestampFailures_ResetsCounter(t *testing.T) {
	var buf bytes.Buffer
	e := &Executor{logger: slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), now: time.Now}

	const actionID = "01HXTEST0000000000000ABCDE"

	for i := 1; i <= luksTimestampFailureThreshold; i++ {
		e.recordLuksTimestampFailure(actionID, "post_rotation", errors.New("disk full"))
	}

	e.clearLuksTimestampFailures(actionID)

	buf.Reset()

	e.recordLuksTimestampFailure(actionID, "post_rotation", errors.New("disk full"))
	got := buf.String()
	if !strings.Contains(got, "level=WARN") {
		t.Errorf("expected first post-recovery failure at WARN, got: %s", got)
	}
	if !strings.Contains(got, "consecutive_failures=1") {
		t.Errorf("expected consecutive_failures=1 after recovery, got: %s", got)
	}
}

func TestRecordLuksTimestampFailure_PerActionIsolation(t *testing.T) {
	var buf bytes.Buffer
	e := &Executor{logger: slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), now: time.Now}

	for i := 1; i < luksTimestampFailureThreshold; i++ {
		e.recordLuksTimestampFailure("action-A", "post_rotation", errors.New("disk full"))
	}
	buf.Reset()

	e.recordLuksTimestampFailure("action-B", "post_rotation", errors.New("disk full"))
	got := buf.String()
	if !strings.Contains(got, "level=WARN") {
		t.Errorf("action-B's first failure must be WARN, not promoted by action-A's streak; got: %s", got)
	}
	if !strings.Contains(got, "consecutive_failures=1") {
		t.Errorf("action-B's first failure must show consecutive_failures=1; got: %s", got)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func TestCheckAndRotate_InitialTimestampPersistFailure_FailsLoud(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	e := &Executor{logger: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), now: time.Now}
	e.SetStore(st)
	e.SetLuksKeyStore(&fakeLuksKeyStore{})

	params := &pb.EncryptionParams{RotationIntervalDays: 30}
	changed, err := e.checkAndRotate(context.Background(), params, &store.LuksState{}, "01HXROTATEFAIL000000000000", "/dev/sda2")
	if err == nil {
		t.Fatal("checkAndRotate must fail loudly when the initial rotation timestamp cannot be persisted — (false, nil) parks rotation forever")
	}
	if changed {
		t.Fatal("no rotation may be reported on the failure path")
	}
	if !strings.Contains(err.Error(), "rotation cannot start") {
		t.Fatalf("error must name the stuck-rotation consequence, got: %v", err)
	}
}
