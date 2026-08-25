package executor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var transientSkipMarkers = []string{
	"no signed-in desktop users",
}

func TestNoSilentSkipSuccessReturns(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("matches-zero guard: no source files found — guard is not scanning anything")
	}

	var offending []string
	transientSeen := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if !strings.Contains(line, `"skipped:`) && !strings.Contains(line, `("skipped:`) {
				continue
			}
			transient := false
			for _, marker := range transientSkipMarkers {
				if strings.Contains(line, marker) {
					transient = true
					transientSeen++
					break
				}
			}
			if !transient {
				offending = append(offending, fmt.Sprintf("%s:%d: %s", f, i+1, strings.TrimSpace(line)))
			}
		}
	}

	if len(offending) > 0 {
		t.Errorf("silent \"skipped:\" success returns found — use notApplicable(reason) for structural inapplicability (spec 23), or add a transient marker if this genuinely re-runs on the next reconciliation:\n%s",
			strings.Join(offending, "\n"))
	}

	if transientSeen == 0 {
		t.Fatal("matches-zero guard: no transient skip sites matched transientSkipMarkers — update the markers to track the moved/renamed sites")
	}
}
