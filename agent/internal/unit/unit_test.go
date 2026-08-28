package unit

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"strings"
	"testing"
)

type fakeManager struct {
	content    string
	readErr    error
	writeErr   error
	reloadErr  error
	pending    bool
	pendingErr error
	writes     int
	reloads    int
	written    string
}

func (manager *fakeManager) ReadUnit(context.Context, string) (string, error) {
	return manager.content, manager.readErr
}

func (manager *fakeManager) WriteUnit(_ context.Context, _ string, content string) error {
	manager.writes++
	manager.written = content
	return manager.writeErr
}

func (manager *fakeManager) DaemonReload(context.Context) error {
	manager.reloads++
	return manager.reloadErr
}

func (manager *fakeManager) NeedsReload(context.Context, string) (bool, error) {
	return manager.pending, manager.pendingErr
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

var testParams = Params{BinaryPath: "/usr/local/bin/cadestrod", DataDir: "/var/lib/cadestro"}

func TestRenderRootUnit(t *testing.T) {
	rendered, err := Render(testParams)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"User=root",
		"ExecStart=/usr/local/bin/cadestrod -data-dir=/var/lib/cadestro -log-level=info",
		"RuntimeDirectoryMode=0700",
	} {
		if !strings.Contains(rendered, expected) {
			t.Errorf("unit missing %q", expected)
		}
	}
	if _, err := Render(Params{BinaryPath: "cadestrod", DataDir: testParams.DataDir}); err == nil {
		t.Fatal("relative binary path accepted")
	}
}

func TestReconcile(t *testing.T) {
	manager := &fakeManager{content: "stale"}
	changed, err := Reconcile(context.Background(), manager, testLogger(), testParams)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || manager.writes != 1 || manager.reloads != 1 {
		t.Fatalf("changed=%v writes=%d reloads=%d", changed, manager.writes, manager.reloads)
	}

	manager = &fakeManager{content: manager.written}
	changed, err = Reconcile(context.Background(), manager, testLogger(), testParams)
	if err != nil {
		t.Fatal(err)
	}
	if changed || manager.writes != 0 || manager.reloads != 0 {
		t.Fatalf("identical unit changed: changed=%v writes=%d reloads=%d", changed, manager.writes, manager.reloads)
	}
}

func TestAbsentUnitBehavior(t *testing.T) {
	manager := &fakeManager{readErr: fs.ErrNotExist}
	changed, err := Reconcile(context.Background(), manager, testLogger(), testParams)
	if err != nil {
		t.Fatal(err)
	}
	if changed || manager.writes != 0 {
		t.Fatal("startup reconcile installed an absent unit")
	}
	if err := EnsureInstalled(context.Background(), manager, testLogger(), testParams); err != nil {
		t.Fatal(err)
	}
	if manager.writes != 1 || manager.reloads != 1 {
		t.Fatalf("install writes=%d reloads=%d", manager.writes, manager.reloads)
	}
}

func TestReconcileErrors(t *testing.T) {
	manager := &fakeManager{content: "stale", writeErr: errors.New("read only")}
	if _, err := Reconcile(context.Background(), manager, testLogger(), testParams); err == nil {
		t.Fatal("write error ignored")
	}
	manager = &fakeManager{content: "stale", reloadErr: errors.New("reload failed")}
	changed, err := Reconcile(context.Background(), manager, testLogger(), testParams)
	if !changed || err == nil {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
}
