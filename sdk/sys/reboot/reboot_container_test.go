//go:build container

package reboot_test

import (
	"context"
	"os"
	"testing"
	"time"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/reboot"
)

const debianRebootMarker = "/var/run/reboot-required"

func TestIsRequired_Marker_Container(t *testing.T) {
	r, err := sysexec.NewRunner(sysexec.Direct)
	if err != nil {
		t.Fatalf("NewRunner(Direct): %v", err)
	}
	m, err := reboot.New(r)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_ = os.Remove(debianRebootMarker)
	t.Cleanup(func() { _ = os.Remove(debianRebootMarker) })

	if req, err := m.IsRequired(ctx); err != nil {
		t.Fatalf("IsRequired (no marker): %v", err)
	} else if req {
		t.Error("IsRequired = true with no marker present, want false")
	}

	if err := os.WriteFile(debianRebootMarker, []byte("*** System restart required ***\n"), 0o644); err != nil {
		t.Fatalf("create marker: %v", err)
	}
	if req, err := m.IsRequired(ctx); err != nil {
		t.Fatalf("IsRequired (marker present): %v", err)
	} else if !req {
		t.Error("IsRequired = false with the marker present, want true")
	}

	if err := os.Remove(debianRebootMarker); err != nil {
		t.Fatalf("remove marker: %v", err)
	}
	if req, err := m.IsRequired(ctx); err != nil {
		t.Fatalf("IsRequired (marker removed): %v", err)
	} else if req {
		t.Error("IsRequired = true after the marker was removed, want false")
	}
}
