package executor

import (
	"context"
	"testing"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysfs "github.com/manchtools/cadestro/sdk/sys/fs"
)

func TestExecutorsKeepRunnerAndManagerOwnership(t *testing.T) {
	runnerOne := &recordingBaseRunner{}
	runnerTwo := &recordingBaseRunner{}
	eOne := NewExecutor(runnerOne)
	eTwo := NewExecutor(runnerTwo)

	fsOne := &fakeRemountFS{mounts: []sysfs.MountInfo{{Source: "/dev/one", Target: "/one", ReadOnly: true}}}
	fsTwo := &fakeRemountFS{mounts: []sysfs.MountInfo{{Source: "/dev/two", Target: "/two", ReadOnly: true}}}
	eOne.deps.fs = fsOne
	eTwo.deps.fs = fsTwo
	if !eOne.repairFilesystem(context.Background()) || !eTwo.repairFilesystem(context.Background()) {
		t.Fatal("both executor-owned filesystem managers should repair their own mount")
	}
	if len(fsOne.remounted) != 1 || fsOne.remounted[0] != "/one" {
		t.Fatalf("executor one used the wrong filesystem manager: %v", fsOne.remounted)
	}
	if len(fsTwo.remounted) != 1 || fsTwo.remounted[0] != "/two" {
		t.Fatalf("executor two used the wrong filesystem manager: %v", fsTwo.remounted)
	}

	params := &pb.ShellParams{RunAsRoot: true, Interpreter: "/bin/sh", WorkingDirectory: t.TempDir()}
	if _, err := eOne.runShellScript(context.Background(), params, "true", nil); err != nil {
		t.Fatalf("executor one shell: %v", err)
	}
	if _, err := eTwo.runShellScript(context.Background(), params, "true", nil); err != nil {
		t.Fatalf("executor two shell: %v", err)
	}
	if len(runnerOne.cmds) != 1 || len(runnerTwo.cmds) != 1 {
		t.Fatalf("runner calls leaked across executors: one=%d two=%d", len(runnerOne.cmds), len(runnerTwo.cmds))
	}
}
