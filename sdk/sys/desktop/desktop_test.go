package desktop

import (
	"errors"
	"os/user"
	"testing"

	"github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/exec/exectest"
)

func newManager(t *testing.T, opts ...Option) (*manager, *exectest.FakeRunner) {
	t.Helper()
	r := exectest.New(exec.Direct)
	m, err := New(r, opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return m.(*manager), r
}

func stubLookPath(t *testing.T, found bool) {
	t.Helper()
	prev := lookPath
	t.Cleanup(func() { lookPath = prev })
	lookPath = func(string) (string, error) {
		if found {
			return loginctlPath, nil
		}
		return "", errors.New("loginctl: not found")
	}
}

func stubLookupID(t *testing.T, fn func(string) (*user.User, error)) {
	t.Helper()
	prev := lookupID
	t.Cleanup(func() { lookupID = prev })
	lookupID = fn
}

func stubLookupUser(t *testing.T, fn func(string) (*user.User, error)) {
	t.Helper()
	prev := lookupUser
	t.Cleanup(func() { lookupUser = prev })
	lookupUser = fn
}

func TestNew_NilRunner(t *testing.T) {
	if _, err := New(nil); !errors.Is(err, exec.ErrRunnerRequired) {
		t.Errorf("New(_, nil) error = %v, want ErrRunnerRequired", err)
	}
}

func TestNew_DefaultHomeRoot(t *testing.T) {
	m, _ := newManager(t)
	if m.homeRoot != defaultHomeRoot {
		t.Errorf("default homeRoot = %q, want %q", m.homeRoot, defaultHomeRoot)
	}
}

func TestNew_WithHomeRoot(t *testing.T) {
	m, _ := newManager(t, WithHomeRoot("/custom/home"))
	if m.homeRoot != "/custom/home" {
		t.Errorf("WithHomeRoot homeRoot = %q, want /custom/home", m.homeRoot)
	}
}

func TestNew_NilOptionIgnored(t *testing.T) {
	m, err := New(exectest.New(exec.Direct), nil, WithHomeRoot("/custom/home"), nil)
	if err != nil {
		t.Fatalf("New with a nil option returned error: %v", err)
	}
	if m.(*manager).homeRoot != "/custom/home" {
		t.Errorf("a nil option must be skipped and the real one applied; homeRoot = %q", m.(*manager).homeRoot)
	}
}
