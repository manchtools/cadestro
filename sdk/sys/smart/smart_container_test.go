//go:build container

package smart_test

import (
	"context"
	"errors"
	osexec "os/exec"
	"testing"
	"time"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/smart"
)

func realSmart(t *testing.T) smart.Collector {
	t.Helper()
	if _, err := osexec.LookPath("smartctl"); err != nil {
		t.Skip("smartctl not installed here; SMART collector not exercisable")
	}
	r, err := sysexec.NewRunner(sysexec.Direct)
	if err != nil {
		t.Fatalf("NewRunner(Direct): %v", err)
	}
	c, err := smart.New(r)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func smartCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestDevice_RejectsInvalidPath_Container(t *testing.T) {
	c := realSmart(t)
	ctx := smartCtx(t)
	for _, bad := range []string{"etc/passwd", "/etc/shadow", "/dev/../etc/shadow"} {
		if _, err := c.Device(ctx, bad); !errors.Is(err, smart.ErrInvalidDevice) {
			t.Errorf("Device(%q) = %v, want ErrInvalidDevice", bad, err)
		}
	}
}

func TestScan_RealSmartctl_Container(t *testing.T) {
	if _, err := realSmart(t).Scan(smartCtx(t)); err != nil {
		t.Fatalf("Scan against real `smartctl --scan -j`: %v", err)
	}
}

func TestDevice_NotInspectable_Container(t *testing.T) {
	c := realSmart(t)
	if _, err := c.Device(smartCtx(t), "/dev/null"); err == nil {
		t.Error("Device(/dev/null) returned no error; smartctl fatal exit-status bits must surface")
	}
}
