package archtest

import (
	"go/ast"
	"go/token"
	"strings"
	"testing"
)

func TestNoUnabstractedTimeNow(t *testing.T) {
	root := moduleRoot(t)
	files := walkGoFiles(t, root, func(rel string) bool {
		if strings.HasPrefix(rel, "internal/store/generated/") {
			return false
		}
		if strings.HasPrefix(rel, "internal/testutil/") {
			return false
		}
		return true
	})
	if len(files) == 0 {
		t.Fatal("matches-zero guard: walked zero production Go files — detector is mis-scoped")
	}

	sawTimeNowRef := 0
	for _, gf := range files {
		exempt := map[token.Pos]bool{}
		ast.Inspect(gf.ast, func(n ast.Node) bool {
			if isTimeNowSelector(n) {
				sawTimeNowRef++
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			if isULIDTimestampCall(call) {
				if inner, ok := call.Args[0].(*ast.CallExpr); ok && isTimeNowCall(inner) {
					exempt[inner.Pos()] = true
				}
			}
			if isTimeNowCall(call) {
				if exempt[call.Pos()] {
					return true
				}
				t.Errorf("unabstracted time.Now() at %s:%d — read the clock through an injected `now func() time.Time` seam (default `time.Now`) and call it, e.g. s.now(); never call time.Now() directly in runtime code.",
					gf.rel, gf.line(call))
			}
			return true
		})
	}
	if sawTimeNowRef == 0 {
		t.Fatal("matches-zero guard: found no reference to the time.Now selector anywhere (not even a seam default) — the detector is dead, the guard would pass vacuously")
	}
}

func isTimeNowSelector(n ast.Node) bool {
	sel, ok := n.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Now" {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == "time"
}

func isTimeNowCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Now" || len(call.Args) != 0 {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == "time"
}

func isULIDTimestampCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Timestamp" || len(call.Args) != 1 {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == "ulid"
}
