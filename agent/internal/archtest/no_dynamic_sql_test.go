package archtest

import (
	"go/ast"
	"strings"
	"testing"
)

var sqlMethodNames = map[string]bool{
	"Exec": true, "ExecContext": true,
	"Query": true, "QueryContext": true,
	"QueryRow": true, "QueryRowContext": true,
	"Prepare": true, "PrepareContext": true,
}

var sqlHandleReceivers = map[string]bool{
	"db": true,
	"tx": true,
}

func TestNoDynamicSQL(t *testing.T) {
	root := moduleRoot(t)
	files := walkGoFiles(t, root, func(rel string) bool { return true })
	if len(files) == 0 {
		t.Fatal("matches-zero guard: walked zero production Go files")
	}
	generatedFiles := 0
	for _, gf := range files {
		if strings.HasPrefix(gf.rel, "internal/store/generated/") {
			generatedFiles++
			continue
		}
		ast.Inspect(gf.ast, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !sqlMethodNames[sel.Sel.Name] || !isSQLHandleReceiver(sel.X) {
				return true
			}
			t.Errorf("handwritten production database call at %s:%d: %s", gf.rel, gf.line(call), render(gf.fset, call))
			return true
		})
	}
	if generatedFiles == 0 {
		t.Fatal("matches-zero guard: generated SQL package is missing")
	}
}

func isSQLHandleReceiver(recv ast.Expr) bool {
	return sqlHandleReceivers[identName(recv)]
}
