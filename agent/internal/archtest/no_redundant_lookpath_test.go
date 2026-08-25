package archtest

import (
	"go/ast"
	"go/token"
	"strings"
	"testing"
)

func TestNoRedundantPackageManagerLookPath(t *testing.T) {
	root := moduleRoot(t)
	files := walkGoFiles(t, root, func(rel string) bool {
		return strings.HasPrefix(rel, "internal/executor/")
	})
	if len(files) == 0 {
		t.Fatal("matches-zero guard: walked zero executor Go files")
	}

	sdkDetectedBinaries := map[string]string{
		"flatpak": "FlatpakAvailable already checks for flatpak; use the executor's pkgBackend for native managers",
		"dpkg":    "pkg.Detect() already checks for apt which requires dpkg; use the executor's pkgBackend",
		"rpm":     "pkg.Detect() already checks for dnf/zypper which require rpm; use the executor's pkgBackend",
	}

	findings := 0
	for _, gf := range files {
		ast.Inspect(gf.ast, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			fnName, literal, isLookPath := lookPathCall(call)
			if !isLookPath {
				return true
			}
			if reason, found := sdkDetectedBinaries[literal]; found {
				t.Errorf("%s:%d: %s(%q) — %s", gf.rel, gf.line(call), fnName, literal, reason)
				findings++
			}
			return true
		})
	}
	if findings > 0 {
		t.Logf("Found %d redundant exec.LookPath calls for package-manager binaries the SDK already detects", findings)
	}
}

func lookPathCall(call *ast.CallExpr) (string, string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "LookPath" {
		return "", "", false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", "", false
	}
	if id.Name != "exec" && id.Name != "osexec" {
		return "", "", false
	}
	if len(call.Args) != 1 {
		return "", "", false
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", "", false
	}
	val := strings.Trim(lit.Value, `"`)
	return id.Name + ".LookPath", val, true
}
