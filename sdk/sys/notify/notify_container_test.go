//go:build container

package notify

import (
	"context"
	osexec "os/exec"
	"testing"
	"time"

	"github.com/manchtools/cadestro/sdk/sys/exec"
)

func TestNotifyAll_RealWall_Container(t *testing.T) {
	if _, err := osexec.LookPath("wall"); err != nil {
		t.Skip("wall not on PATH")
	}
	r, err := exec.NewRunner(exec.Direct)
	if err != nil {
		t.Fatalf("NewRunner(Direct): %v", err)
	}
	m, err := New(r)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := m.NotifyAll(ctx, "Cadestro Container Test", "hello from the container test"); err != nil {
		t.Fatalf("NotifyAll via real wall returned error: %v", err)
	}
}
