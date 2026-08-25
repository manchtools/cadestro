package exectest_test

import (
	"context"
	"errors"
	"testing"

	"github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/exec/exectest"
)

func TestFakeRunner_StreamNilCallback(t *testing.T) {
	f := exectest.New(exec.Direct)
	f.Push(exec.Result{ExitCode: 0, Stdout: "line\n"}, nil)

	res, err := f.Stream(context.Background(), exec.Command{Name: "journalctl"}, nil)
	if err != nil {
		t.Fatalf("Stream(nil callback) err = %v", err)
	}
	if res.Stdout != "line\n" {
		t.Errorf("Stdout = %q, want the scripted result", res.Stdout)
	}
	if len(f.Calls()) != 1 || f.Calls()[0].Name != "journalctl" {
		t.Errorf("Stream did not record the Command: %+v", f.Calls())
	}
}

func TestFakeRunner_StreamRespectsCancelledContext(t *testing.T) {
	f := exectest.New(exec.Direct)
	f.Push(exec.Result{Stdout: "must-not-replay\n"}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	replayed := 0
	res, err := f.Stream(ctx, exec.Command{Name: "journalctl"},
		func(exec.StreamType, string, int64) { replayed++ })
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if replayed != 0 {
		t.Errorf("replayed %d lines on a cancelled Stream, want 0", replayed)
	}
	if res.Stdout != "" {
		t.Errorf("res = %+v, want zero value (scripted result not consumed)", res)
	}

	if next, _ := f.Run(context.Background(), exec.Command{Name: "journalctl"}); next.Stdout != "must-not-replay\n" {
		t.Errorf("scripted result was wrongly consumed by the cancelled Stream: %q", next.Stdout)
	}
}
