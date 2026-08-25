package archtest

import (
	"go/ast"
	"go/token"
	"strings"
	"testing"
)

var protoJSONAllowlist = map[string]string{}

const protoPkgPathSuffix = "/contract/gen/go/cadestro/v1"

func TestNoStdlibJSONOfProtoMessage(t *testing.T) {
	root := moduleRoot(t)
	files := walkGoFiles(t, root, func(string) bool { return true })
	if len(files) == 0 {
		t.Fatal("matches-zero guard: walked zero production Go files — detector is mis-scoped")
	}

	allow := newAllowlist(protoJSONAllowlist)
	sawProtoImport := false
	sawJSONCall := false

	for _, gf := range files {
		protoAliases := protoImportAliases(gf.ast)
		if len(protoAliases) > 0 {
			sawProtoImport = true
		}
		jsonAlias := importAliasFor(gf.ast, "encoding/json")
		if jsonAlias == "" {
			continue
		}
		for _, decl := range gf.ast.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			protoLocals := protoTypedLocals(fn, protoAliases)
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				target, isJSON := jsonSerdeTarget(call, jsonAlias)
				if !isJSON {
					return true
				}
				sawJSONCall = true
				if target == nil || !exprIsProtoTyped(target, protoAliases, protoLocals) {
					return true
				}
				key := gf.rel + " :: " + render(gf.fset, call)
				if allow.exempt(key) {
					return true
				}
				t.Errorf("stdlib encoding/json applied to a proto message at %s:%d — %s\n  proto messages must use protojson (google.golang.org/protobuf/encoding/protojson); stdlib json silently corrupts oneofs/enums/int64/well-known types. If this operand is a proto ENUM (a plain int32, safe under stdlib json), add a justified, guarded entry to protoJSONAllowlist.",
					gf.rel, gf.line(call), render(gf.fset, call))
				return true
			})
		}
	}

	if !sawProtoImport {
		t.Fatal("matches-zero guard: no file imports the generated proto package — the proto-type detector is dead, the guard would pass vacuously")
	}
	if !sawJSONCall {
		t.Fatal("matches-zero guard: detected no encoding/json calls in the module — the json-call detector is dead, the guard would pass vacuously")
	}
	allow.assertNoStale(t)
}

func protoImportAliases(f *ast.File) map[string]bool {
	out := map[string]bool{}
	for _, imp := range f.Imports {
		path := unquoteLit(imp.Path)
		if !strings.HasSuffix(path, protoPkgPathSuffix) {
			continue
		}
		if imp.Name != nil {
			out[imp.Name.Name] = true
		} else {

			out["cadestrov1"] = true
			out["v1"] = true
		}
	}
	return out
}

func importAliasFor(f *ast.File, path string) string {
	for _, imp := range f.Imports {
		if unquoteLit(imp.Path) != path {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		if i := strings.LastIndex(path, "/"); i >= 0 {
			return path[i+1:]
		}
		return path
	}
	return ""
}

func jsonSerdeTarget(call *ast.CallExpr, jsonAlias string) (ast.Expr, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, false
	}

	if id, ok := sel.X.(*ast.Ident); ok && id.Name == jsonAlias {
		switch sel.Sel.Name {
		case "Marshal", "MarshalIndent":
			if len(call.Args) >= 1 {
				return call.Args[0], true
			}
		case "Unmarshal":
			if len(call.Args) >= 2 {
				return call.Args[1], true
			}
		}
		return nil, false
	}

	if (sel.Sel.Name == "Encode" || sel.Sel.Name == "Decode") && len(call.Args) >= 1 {
		if inner, ok := sel.X.(*ast.CallExpr); ok {
			if innerSel, ok := inner.Fun.(*ast.SelectorExpr); ok {
				if id, ok := innerSel.X.(*ast.Ident); ok && id.Name == jsonAlias &&
					(innerSel.Sel.Name == "NewEncoder" || innerSel.Sel.Name == "NewDecoder") {
					return call.Args[0], true
				}
			}
		}
	}
	return nil, false
}

func protoTypedLocals(fn *ast.FuncDecl, aliases map[string]bool) map[string]bool {
	out := map[string]bool{}
	if len(aliases) == 0 {
		return out
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.DeclStmt:
			gd, ok := x.Decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				return true
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok || vs.Type == nil {
					continue
				}
				if isProtoTypeExpr(vs.Type, aliases) {
					for _, name := range vs.Names {
						out[name.Name] = true
					}
				}
			}
		case *ast.AssignStmt:
			if x.Tok != token.DEFINE || len(x.Lhs) != len(x.Rhs) {
				return true
			}
			for i, rhs := range x.Rhs {
				if exprIsProtoComposite(rhs, aliases) {
					if id, ok := x.Lhs[i].(*ast.Ident); ok {
						out[id.Name] = true
					}
				}
			}
		}
		return true
	})
	return out
}

func isProtoTypeExpr(e ast.Expr, aliases map[string]bool) bool {
	switch x := e.(type) {
	case *ast.StarExpr:
		return isProtoTypeExpr(x.X, aliases)
	case *ast.SelectorExpr:
		id, ok := x.X.(*ast.Ident)
		return ok && aliases[id.Name]
	}
	return false
}

func exprIsProtoComposite(e ast.Expr, aliases map[string]bool) bool {
	switch x := e.(type) {
	case *ast.UnaryExpr:
		if x.Op == token.AND {
			return exprIsProtoComposite(x.X, aliases)
		}
	case *ast.ParenExpr:
		return exprIsProtoComposite(x.X, aliases)
	case *ast.CompositeLit:
		return x.Type != nil && isProtoTypeExpr(x.Type, aliases)
	}
	return false
}

func exprIsProtoTyped(e ast.Expr, aliases, protoLocals map[string]bool) bool {
	switch x := e.(type) {
	case *ast.UnaryExpr:
		if x.Op == token.AND {
			return exprIsProtoTyped(x.X, aliases, protoLocals)
		}
	case *ast.ParenExpr:
		return exprIsProtoTyped(x.X, aliases, protoLocals)
	case *ast.CompositeLit:
		return x.Type != nil && isProtoTypeExpr(x.Type, aliases)
	case *ast.Ident:
		return protoLocals[x.Name]
	}
	return false
}
