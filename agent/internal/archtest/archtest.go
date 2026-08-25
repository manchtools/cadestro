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
	abs  string
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

func repositoryRoot(t *testing.T) string {
	t.Helper()
	module := moduleRoot(t)
	dir := filepath.Dir(module)
	for {
		if info, err := os.Stat(filepath.Join(dir, ".github", "workflows")); err == nil && info.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate a .github/workflows directory above the module root %s", module)
		}
		dir = parent
	}
}

func walkGoFiles(t *testing.T, root string, keep func(rel string) bool) []*goFile {
	t.Helper()
	var out []*goFile
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || name == "testdata" || name == ".git" {
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
		if strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		if strings.HasPrefix(rel, "internal/archtest/") {
			return nil
		}
		if !keep(rel) {
			return nil
		}
		fset := token.NewFileSet()
		f, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if perr != nil {
			t.Fatalf("parse %s: %v", rel, perr)
		}
		out = append(out, &goFile{abs: path, rel: rel, fset: fset, ast: f})
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

func render(fset *token.FileSet, n ast.Node) string {
	var b strings.Builder
	if err := printer.Fprint(&b, fset, n); err != nil {
		return "<unprintable node>"
	}

	return strings.Join(strings.Fields(b.String()), " ")
}

func (gf *goFile) line(n ast.Node) int {
	return gf.fset.Position(n.Pos()).Line
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
