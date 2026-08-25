package executor

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	sysfs "github.com/manchtools/cadestro/sdk/sys/fs"
)

type fakeRemountFS struct {
	sysfs.Manager
	mounts     []sysfs.MountInfo
	remounted  []string
	remountErr error
}

func (f *fakeRemountFS) ListMounts(ctx context.Context) ([]sysfs.MountInfo, error) {
	return f.mounts, nil
}

func (f *fakeRemountFS) RemountRW(ctx context.Context, target string) error {
	f.remounted = append(f.remounted, target)
	return f.remountErr
}

func TestRepairFilesystem_RemountsOnlyReadOnlyBlockDevices(t *testing.T) {
	fake := &fakeRemountFS{mounts: []sysfs.MountInfo{
		{Source: "/dev/sda1", Target: "/", FSType: "ext4", ReadOnly: true},
		{Source: "/dev/sda2", Target: "/usr", FSType: "ext4", ReadOnly: true},
		{Source: "/dev/sda3", Target: "/home", FSType: "ext4", ReadOnly: false},
		{Source: "proc", Target: "/proc", FSType: "proc", ReadOnly: true},
		{Source: "sysfs", Target: "/sys", FSType: "sysfs", ReadOnly: true},
		{Source: "tmpfs", Target: "/run", FSType: "tmpfs", ReadOnly: true},
	}}
	e := NewExecutor(nil)
	e.logger = slog.Default()
	e.deps.fs = fake
	if ok := e.repairFilesystem(context.Background()); !ok {
		t.Fatal("repairFilesystem reported failure though every remount succeeded")
	}

	want := []string{"/", "/usr"}
	if len(fake.remounted) != len(want) {
		t.Fatalf("remounted %v; want exactly %v (only read-only /dev mounts)", fake.remounted, want)
	}
	for i, w := range want {
		if fake.remounted[i] != w {
			t.Errorf("remounted[%d] = %q; want %q", i, fake.remounted[i], w)
		}
	}
}

func TestRepairFilesystem_RemountFailureReportsNotAllOk(t *testing.T) {
	e := NewExecutor(nil)
	e.logger = slog.Default()
	e.deps.fs = &fakeRemountFS{
		mounts:     []sysfs.MountInfo{{Source: "/dev/sda1", Target: "/", ReadOnly: true}},
		remountErr: errors.New("remount: read-only file system"),
	}

	if ok := e.repairFilesystem(context.Background()); ok {
		t.Error("a failed remount of a read-only block device must report not-all-ok (false)")
	}
}
