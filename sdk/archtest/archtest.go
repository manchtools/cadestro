package archtest

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type goFile struct {
	rel  string
	fset *token.FileSet
	ast  *ast.File
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate go.mod above %s", dir)
		}
		dir = parent
	}
}

func walkGoFiles(t *testing.T, root string, keep func(rel string) bool) []*goFile {
	t.Helper()
	return walkGoFilesIncludingTests(t, root, func(rel string) bool {
		return !strings.HasSuffix(rel, "_test.go") && keep(rel)
	})
}

func walkGoFilesIncludingTests(t *testing.T, root string, keep func(rel string) bool) []*goFile {
	t.Helper()
	var out []*goFile
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "vendor", "testdata", ".git":
				return filepath.SkipDir
			}

			if name := d.Name(); name != "." && (strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !keep(rel) {
			return nil
		}
		fset := token.NewFileSet()
		f, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if perr != nil {
			t.Fatalf("parse %s: %v", rel, perr)
		}
		out = append(out, &goFile{rel: rel, fset: fset, ast: f})
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

func (gf *goFile) line(n ast.Node) int { return gf.fset.Position(n.Pos()).Line }

func render(fset *token.FileSet, n ast.Node) string {
	var b strings.Builder
	if err := printer.Fprint(&b, fset, n); err != nil {
		return "<unprintable node>"
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

type allowlist struct {
	reason map[string]string
	used   map[string]bool
}

func newAllowlist(reasons map[string]string) *allowlist {
	return &allowlist{reason: reasons, used: make(map[string]bool)}
}

func (a *allowlist) exempt(key string) bool {
	if _, ok := a.reason[key]; ok {
		a.used[key] = true
		return true
	}
	return false
}

func (a *allowlist) assertNoStale(t *testing.T) {
	t.Helper()
	for key := range a.reason {
		if !a.used[key] {
			t.Errorf("stale allowlist entry never matched any site (remove it or fix the key): %q", key)
		}
	}
}
