package archtest

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	repoModulePrefix = "github.com/manchtools/cadestro/"
	sdkModule        = "github.com/manchtools/cadestro/sdk"
)

func TestSDKImportsNothingElseInRepo(t *testing.T) {
	files := allGoFiles(t)
	if len(files) == 0 {
		t.Fatal("scanned zero Go files — the guard cannot pass vacuously")
	}
	inspectedImports := 0

	for _, gf := range files {
		dir := filepath.ToSlash(filepath.Dir(gf.rel))
		if dir == "." {
			dir = ""
		}
		for _, spec := range gf.ast.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("%s: unquote import %s: %v", gf.rel, spec.Path.Value, err)
			}
			inspectedImports++
			if !strings.HasPrefix(path, repoModulePrefix) {
				continue
			}
			if path == sdkModule || strings.HasPrefix(path, sdkModule+"/") {
				continue
			}
			t.Errorf("%s:%d imports %s — the SDK is a leaf module and must not depend on anything else in this repository (see LICENSING.md).",
				gf.rel, gf.line(spec), path)
		}
	}

	if inspectedImports == 0 {
		t.Fatal("inspected zero import specs — the guard cannot pass vacuously")
	}
}

func allGoFiles(t *testing.T) []*goFile {
	t.Helper()
	return walkGoFilesIncludingTests(t, moduleRoot(t), func(string) bool { return true })
}
