package archtest

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The repository's module paths. Everything under repoModulePrefix is in-repo;
// contractModule is the one in-repo module the SDK is allowed to name, and
// only from the packages recorded in contractImporters below.
const (
	repoModulePrefix = "github.com/manchtools/cadestro/"
	sdkModule        = "github.com/manchtools/cadestro/sdk"
	contractModule   = "github.com/manchtools/cadestro/contract"
)

// contractImporters is the exact set of SDK packages permitted to import the
// contract module, keyed by module-relative directory.
//
// sys/osquery is here because its Querier API is expressed in the contract's
// OSQuery messages — the capability hands its rows straight to the agent as
// wire types. adversary follows it: its security machine drives osquery
// alongside the other capabilities, so it names the same messages.
//
// This is not a "known violations" list that may grow. TestSDKImportsOnlyThe-
// Contract fails on any package outside it, and TestNoStaleContractImporter
// fails when an entry stops importing the contract — so closing the exception
// (by making the capability's API proto-free) forces this list to shrink in
// the same change.
var contractImporters = map[string]string{
	"sys/osquery": "Querier is defined in terms of pb.OSQuery/OSQueryResult/OSQueryRow",
	"adversary":   "drives sys/osquery in the cross-capability security machine",
}

// TestSDKImportsOnlyTheContract is the leaf-purity guard. The SDK is a leaf of
// this repository: it must not import the agent, the server, or anything else
// in-repo, and it may name the contract module only from contractImporters.
//
// The licensing split is the reason. contract and sdk are MIT so that speaking
// the protocol or embedding a capability imposes no obligation; agent and
// server are copyleft. Permissive leaves feeding copyleft consumers is the safe
// direction and the reverse is not, which is why this is a test and not a
// convention.
//
// Test files are inspected too. A test-only edge still puts the imported
// module in this one's build list, which is exactly the coupling the leaf rule
// exists to prevent.
func TestSDKImportsOnlyTheContract(t *testing.T) {
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
			if path == contractModule || strings.HasPrefix(path, contractModule+"/") {
				if _, ok := contractImporters[dir]; !ok {
					t.Errorf("%s:%d imports the contract module (%s), but %q is not a recorded contract importer.\n"+
						"The SDK is a leaf and is otherwise proto-free: express this capability in its own types, or record the package in contractImporters with the reason.",
						gf.rel, gf.line(spec), path, dir)
				}
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

// TestNoStaleContractImporter fails when a recorded exception no longer
// imports the contract. Without it the allowlist would quietly outlive the
// coupling it documents, and the next reader would take a package's presence
// here as evidence that the SDK still cannot be proto-free.
func TestNoStaleContractImporter(t *testing.T) {
	files := allGoFiles(t)
	if len(files) == 0 {
		t.Fatal("scanned zero Go files — the guard cannot pass vacuously")
	}
	if len(contractImporters) == 0 {
		t.Fatal("contractImporters is empty — delete this test and the allowlist together, not one of them")
	}

	importing := make(map[string]bool, len(contractImporters))
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
			if path == contractModule || strings.HasPrefix(path, contractModule+"/") {
				importing[dir] = true
			}
		}
	}

	for dir, reason := range contractImporters {
		if !importing[dir] {
			t.Errorf("contractImporters records %q (%s) but nothing there imports the contract any more — remove the entry", dir, reason)
		}
	}
}

// allGoFiles parses every .go file in the module, test files included: an
// import edge in a test is a real module dependency, so leaf purity has to see
// it. Generated contract code is not in this module, so nothing is excluded.
func allGoFiles(t *testing.T) []*goFile {
	t.Helper()
	return walkGoFilesIncludingTests(t, moduleRoot(t), func(string) bool { return true })
}
