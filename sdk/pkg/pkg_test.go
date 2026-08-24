package pkg

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/exec/exectest"
)

// --- shared test harness ---------------------------------------------------

// newFake returns a FakeRunner reporting the Direct backend. Escalate is still
// recorded on each Command, so escalation assertions work regardless.
func newFake() *exectest.FakeRunner { return exectest.New(sysexec.Direct) }

// argv renders a recorded Command as "name arg1 arg2 …" for substring asserts.
func argv(c sysexec.Command) string {
	return strings.Join(append([]string{c.Name}, c.Args...), " ")
}

// stubLookPath overrides the package lookPath seam so only the named binaries
// resolve on PATH. Restored at test end.
func stubLookPath(t *testing.T, present ...string) {
	t.Helper()
	set := make(map[string]bool, len(present))
	for _, p := range present {
		set[p] = true
	}
	orig := lookPath
	lookPath = func(name string) (string, error) {
		if set[name] {
			return "/usr/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
	t.Cleanup(func() { lookPath = orig })
}

// mustNew builds a Manager over a fresh FakeRunner and fails the test on error.
func mustNew(t *testing.T, b Backend) (Manager, *exectest.FakeRunner) {
	t.Helper()
	f := newFake()
	m, err := New(b, f)
	if err != nil {
		t.Fatalf("New(%v): %v", b, err)
	}
	return m, f
}

// ok scripts the next runner call as a clean success with the given stdout.
func ok(f *exectest.FakeRunner, stdout string) { f.Push(sysexec.Result{Stdout: stdout}, nil) }

// TestMutationsReturnCommandOutput pins the contract that a Manager mutation
// returns the package manager's command output (exec.Result), not just an error
// — operators need to see what an install/remove/update actually did. On a clean
// exit the Result carries stdout; on a non-zero exit it carries stdout + stderr
// + exit code AND a typed *exec.CommandError. Driven by the agent migration:
// the agent maps this Result into the pb.CommandOutput it returns to control.
func TestMutationsReturnCommandOutput(t *testing.T) {
	ctx := context.Background()

	t.Run("install surfaces stdout on success", func(t *testing.T) {
		m, f := mustNew(t, Apt)
		f.Push(sysexec.Result{Stdout: "Setting up vim ...\n", ExitCode: 0}, nil)
		res, err := m.Install(ctx, InstallOptions{}, InstallSpec{Name: "vim"})
		if err != nil {
			t.Fatalf("Install err = %v", err)
		}
		if res.Stdout != "Setting up vim ...\n" {
			t.Errorf("Result.Stdout = %q, want the command stdout", res.Stdout)
		}
		if res.ExitCode != 0 {
			t.Errorf("Result.ExitCode = %d, want 0", res.ExitCode)
		}
	})

	t.Run("install surfaces stdout+stderr+exit AND CommandError on failure", func(t *testing.T) {
		m, f := mustNew(t, Apt)
		f.Push(sysexec.Result{
			Stdout:   "Reading package lists...\n",
			Stderr:   "E: Unable to locate package vim\n",
			ExitCode: 100,
		}, nil)
		res, err := m.Install(ctx, InstallOptions{}, InstallSpec{Name: "vim"})
		if err == nil {
			t.Fatal("Install err = nil, want a non-zero-exit error")
		}
		var ce *sysexec.CommandError
		if !errors.As(err, &ce) {
			t.Fatalf("err = %v, want *exec.CommandError", err)
		}
		if res.ExitCode != 100 || res.Stdout != "Reading package lists...\n" || res.Stderr != "E: Unable to locate package vim\n" {
			t.Errorf("Result = %+v, want stdout+stderr+exit preserved", res)
		}
	})
}

// --- New -------------------------------------------------------------------

func TestNew_AllBackends(t *testing.T) {
	for _, b := range []Backend{Apt, Dnf, Dnf5, Pacman, Zypper} {
		m, err := New(b, newFake())
		if err != nil {
			t.Fatalf("New(%v) unexpected error: %v", b, err)
		}
		if m.Backend() != b {
			t.Errorf("New(%v).Backend() = %v, want %v", b, m.Backend(), b)
		}
	}
}

func TestNew_RejectsUnknownBackend(t *testing.T) {
	for _, b := range []Backend{0, Backend(99), Backend(-1)} {
		if _, err := New(b, newFake()); !errors.Is(err, ErrUnknownBackend) {
			t.Errorf("New(%d) error = %v, want ErrUnknownBackend", int(b), err)
		}
	}
}

func TestNew_RejectsNilRunner(t *testing.T) {
	_, err := New(Apt, nil)
	if !errors.Is(err, sysexec.ErrRunnerRequired) {
		t.Errorf("New(Apt, nil) error = %v, want ErrRunnerRequired", err)
	}
}

// --- Backend.String --------------------------------------------------------

func TestBackend_String(t *testing.T) {
	cases := map[Backend]string{
		Apt:         "apt",
		Dnf:         "dnf",
		Pacman:      "pacman",
		Zypper:      "zypper",
		Dnf5:        "dnf5",
		Backend(0):  "Backend(0)",
		Backend(99): "Backend(99)",
	}
	for b, want := range cases {
		if got := b.String(); got != want {
			t.Errorf("Backend(%d).String() = %q, want %q", int(b), got, want)
		}
	}
}

// --- Detect ----------------------------------------------------------------

func TestDetect(t *testing.T) {
	cases := []struct {
		name    string
		present []string
		want    []Backend
	}{
		{"none", nil, nil},
		{"apt only", []string{"apt-get"}, []Backend{Apt}},
		{"dnf only", []string{"dnf"}, []Backend{Dnf}},
		{"pacman only", []string{"pacman"}, []Backend{Pacman}},
		{"zypper only", []string{"zypper"}, []Backend{Zypper}},
		{"dnf5 preferred", []string{"dnf5", "dnf"}, []Backend{Dnf5}},
		{"priority order", []string{"zypper", "apt-get"}, []Backend{Apt, Zypper}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubLookPath(t, tc.present...)
			got := Detect()
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Detect() = %v, want %v", got, tc.want)
			}
		})
	}
}
