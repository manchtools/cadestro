package remote

import (
	"context"
	"errors"
	"sync"
	"testing"
)

type stubVCBackend struct {
	mu     sync.Mutex
	tag    string
	syncs  int
	resolv int
}

func (s *stubVCBackend) CloneOrSync(_ context.Context, _ GitConfig, _ string) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncs++
	return Result{Revision: "stub:" + s.tag, Changed: true}, nil
}

func (s *stubVCBackend) Resolve(_ context.Context, _ GitConfig) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resolv++
	return "stub:" + s.tag, nil
}

func withSnapshotRegistry(t *testing.T, body func()) {
	t.Helper()
	vcRegistry.mu.Lock()
	snapshot := make(map[string]VersionControlBackend, len(vcRegistry.m))
	for k, v := range vcRegistry.m {
		snapshot[k] = v
	}
	vcRegistry.mu.Unlock()
	t.Cleanup(func() {
		vcRegistry.mu.Lock()
		vcRegistry.m = snapshot
		vcRegistry.mu.Unlock()
	})
	body()
}

func TestRegisterVersionControlBackend_LookupAndOverride(t *testing.T) {
	withSnapshotRegistry(t, func() {
		first := &stubVCBackend{tag: "first"}
		RegisterVersionControlBackend("stub", first)

		got, err := versionControlBackend("stub")
		if err != nil {
			t.Fatalf("versionControlBackend(stub): %v", err)
		}
		if got != first {
			t.Fatalf("lookup returned wrong backend; got %p want %p", got, first)
		}

		second := &stubVCBackend{tag: "second"}
		RegisterVersionControlBackend("stub", second)
		got2, err := versionControlBackend("stub")
		if err != nil {
			t.Fatalf("versionControlBackend(stub) after override: %v", err)
		}
		if got2 != second {
			t.Fatalf("override didn't win; got %p want %p", got2, second)
		}
	})
}

func TestRegisterVersionControlBackend_IgnoresEmptyNameOrNil(t *testing.T) {
	withSnapshotRegistry(t, func() {

		RegisterVersionControlBackend("", &stubVCBackend{tag: "ignored"})
		RegisterVersionControlBackend("nilcheck", nil)

		if _, err := versionControlBackend(""); !errors.Is(err, ErrBackendNotFound) {
			t.Fatalf("versionControlBackend(``) = %v; want ErrBackendNotFound", err)
		}
		if _, err := versionControlBackend("nilcheck"); !errors.Is(err, ErrBackendNotFound) {
			t.Fatalf("versionControlBackend(nilcheck) = %v; want ErrBackendNotFound (nil ignored)", err)
		}
	})
}

func TestVersionControlBackend_UnknownDriver(t *testing.T) {
	withSnapshotRegistry(t, func() {
		_, err := versionControlBackend("never-registered")
		if !errors.Is(err, ErrBackendNotFound) {
			t.Fatalf("versionControlBackend(never-registered) = %v; want ErrBackendNotFound", err)
		}
	})
}

func TestRegisterVersionControlBackend_ConcurrentLookup(t *testing.T) {
	withSnapshotRegistry(t, func() {
		var wg sync.WaitGroup
		for i := 0; i < 16; i++ {
			wg.Add(2)
			go func() {
				defer wg.Done()
				RegisterVersionControlBackend("race", &stubVCBackend{tag: "r"})
			}()
			go func() {
				defer wg.Done()
				_, _ = versionControlBackend("race")
			}()
		}
		wg.Wait()
	})
}
