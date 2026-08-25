package architecture_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestComplianceStateHasOneWriter(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	generated := filepath.Join(root, "internal", "store", "generated")

	forbidden := []string{
		"INSERT INTO compliance_results",
		"INSERT INTO compliance_policy_evaluation",
		"compliance_status =",
	}

	writers := []string{
		"UpsertDeviceComplianceResult",
		"UpsertCompliancePolicyEvaluation",
		"RefreshDeviceComplianceStatus",
	}

	declared := make(map[string]bool, len(writers))
	called := make(map[string]bool, len(writers))
	scanned := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == "vendor" || name == "testdata" ||
				(path != root && (strings.HasPrefix(name, "_") || strings.HasPrefix(name, "."))) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(source)
		inGenerated := strings.HasPrefix(path, generated+string(filepath.Separator))
		if inGenerated {
			for _, writer := range writers {
				if strings.Contains(text, "func (q *Queries) "+writer+"(") {
					declared[writer] = true
				}
			}
			return nil
		}
		scanned++
		if path == file {
			return nil
		}
		for _, statement := range forbidden {
			if strings.Contains(text, statement) {
				t.Errorf("compliance state is written outside the generated query layer: %q in %s", statement, path)
			}
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		for _, writer := range writers {
			if strings.Contains(text, "."+writer+"(ctx,") {
				called[writer] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned == 0 {
		t.Fatal("matches-zero guard: inspected no Go files outside the generated query layer")
	}
	for _, writer := range writers {
		if !declared[writer] {
			t.Errorf("compliance writer %s no longer exists in the generated query layer", writer)
		}
		if !called[writer] {
			t.Errorf("compliance writer %s exists but no production code calls it", writer)
		}
	}
}
