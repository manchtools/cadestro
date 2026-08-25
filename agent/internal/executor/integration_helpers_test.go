//go:build integration

package executor

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

var (
	testExecutorTmpDirsMu sync.Mutex
	testExecutorTmpDirs   []string
)

func TestMain(m *testing.M) {
	if !disposableHost() {
		fmt.Fprintln(os.Stderr,
			"executor integration tests skipped: not running in a container.\n"+
				"These mutate real host state (users, files, packages). Run them in "+
				"the container lane (`docker run ... -tags=integration`), or set "+
				"CADESTRO_ALLOW_DESTRUCTIVE_TESTS=1 to force them on this host.")
		os.Exit(0)
	}
	code := m.Run()
	testExecutorTmpDirsMu.Lock()
	for _, d := range testExecutorTmpDirs {
		_ = os.RemoveAll(d)
	}
	testExecutorTmpDirsMu.Unlock()
	os.Exit(code)
}

func disposableHost() bool {
	if os.Getenv("CADESTRO_ALLOW_DESTRUCTIVE_TESTS") == "1" {
		return true
	}
	for _, marker := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(marker); err == nil {
			return true
		}
	}
	return false
}

func checkCmdSuccess(name string, args ...string) bool {

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, err := executorRunner.Run(ctx, sysexec.Command{Name: name, Args: args})
	return err == nil && r.ExitCode == 0
}
