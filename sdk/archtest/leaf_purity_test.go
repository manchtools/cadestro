package archtest

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The repository's module paths. Everything under repoModulePrefix is
// in-repo; the SDK may import nothing under it except itself.
const (
	repoModulePrefix = "github.com/manchtools/cadestro/"
	sdkModule        = "github.com/manchtools/cadestro/sdk"
)

// TestSDKImportsNothingElseInRepo is the leaf-purity guard. The SDK is a leaf
// of this repository: it must not import any sibling in-repo module. There
// are no exceptions — the last one (sys/osquery speaking the retired wire
// contract's types) was closed when the capability's API became proto-free.
//
// The licensing split is the reason. The SDK is MIT so that embedding a
// capability imposes no obligation; application modules may be copyleft.
// Permissive leaves feeding copyleft consumers is the safe direction and the
// reverse is not, which is why this is a test and not a convention.
//
// Test files are inspected too. A test-only edge still puts the imported
// module in this one's build list, which is exactly the coupling the leaf rule
// exists to prevent.
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
				continue // third-party or standard library
			}
			if path == sdkModule || strings.HasPrefix(path, sdkModule+"/") {
				continue // this module
			}
			t.Errorf("%s:%d imports %s — the SDK is a leaf module and must not depend on anything else in this repository (see LICENSING.md).",
				gf.rel, gf.line(spec), path)
		}
	}

	if inspectedImports == 0 {
		t.Fatal("inspected zero import specs — the guard cannot pass vacuously")
	}
}

// allGoFiles parses every .go file in the module, test files included: an
// import edge in a test is a real module dependency, so leaf purity has to see
// it.
func allGoFiles(t *testing.T) []*goFile {
	t.Helper()
	return walkGoFilesIncludingTests(t, moduleRoot(t), func(string) bool { return true })
}
