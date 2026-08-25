package user

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/exec/exectest"
)

func TestLastLogin_ParsesTimestamp(t *testing.T) {
	f := exectest.New(exec.Direct)

	f.Push(exec.Result{Stdout: "deploy   pts/0        192.168.1.10     Mon Jun 16 14:23:01 2025 - Mon Jun 16 16:01:55 2025  (01:38)\n\nwtmp begins Mon Jun  2 09:14:00 2025\n"}, nil)

	got, err := mgr(t, f).LastLogin(context.Background(), "deploy")
	if err != nil {
		t.Fatalf("LastLogin err = %v, want nil", err)
	}

	want := time.Date(2025, time.June, 16, 14, 23, 1, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("LastLogin = %v, want %v", got, want)
	}

	calls := f.Calls()
	if len(calls) != 1 {
		t.Fatalf("got %d commands, want 1: %+v", len(calls), calls)
	}
	c := calls[0]
	if c.Name != "last" {
		t.Errorf("command = %q, want last", c.Name)
	}
	if want := []string{"-1", "-F", "deploy"}; !equalArgs(c.Args, want) {
		t.Errorf("argv = %v, want %v", c.Args, want)
	}
}

func TestLastLogin_NeverLoggedIn(t *testing.T) {
	cases := []struct {
		name   string
		stdout string
	}{

		{"only wtmp-begins footer", "\nwtmp begins Mon Jun  2 09:14:00 2025\n"},

		{"empty output", ""},
		{"blank lines only", "\n\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := exectest.New(exec.Direct)
			f.Push(exec.Result{Stdout: tc.stdout}, nil)
			got, err := mgr(t, f).LastLogin(context.Background(), "deploy")
			if err != nil {
				t.Fatalf("never-logged-in must NOT error, got %v", err)
			}
			if !got.IsZero() {
				t.Fatalf("never-logged-in must be the zero time, got %v", got)
			}
		})
	}
}

func TestLastLogin_RejectsInvalidUsername(t *testing.T) {

	bad := []string{
		"",
		"-F",
		"--help",
		"de ploy",
		"deploy\nroot",
		"Deploy",
		"1deploy",
		"root;id",
	}
	for _, name := range bad {
		f := exectest.New(exec.Direct)
		_, err := mgr(t, f).LastLogin(context.Background(), name)
		if err == nil {
			t.Errorf("LastLogin(%q) err = nil, want a validation rejection", name)
		}
		if n := len(f.Calls()); n != 0 {
			t.Errorf("LastLogin(%q) ran %d commands, want 0 (rejected before the Runner)", name, n)
		}
	}
}

func TestLastLogin_RunnerErrorPropagates(t *testing.T) {
	t.Run("runner error", func(t *testing.T) {
		f := exectest.New(exec.Direct)
		f.Push(exec.Result{}, errors.New("exec: \"last\": executable file not found in $PATH"))
		if _, err := mgr(t, f).LastLogin(context.Background(), "deploy"); err == nil {
			t.Fatal("a runner failure must propagate, got nil")
		}
	})
	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		f := exectest.New(exec.Direct)
		if _, err := mgr(t, f).LastLogin(ctx, "deploy"); !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	})
}

func equalArgs(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
