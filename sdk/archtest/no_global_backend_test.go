package archtest

import (
	"go/ast"
	"go/token"
	"regexp"
	"strings"
	"testing"
)

var noGlobalBackendVarAllowlist = map[string]string{}

var backendFuncRe = regexp.MustCompile(`(?i)^(set|get|current)[A-Za-z0-9_]*backend$`)

var backendVarRe = regexp.MustCompile(`(?i)^(current|active|default|global|the)?backend$`)

func TestNoGlobalBackendState(t *testing.T) {
	root := moduleRoot(t)
	files := walkGoFiles(t, root, func(rel string) bool {
		if strings.HasPrefix(rel, "gen/") || strings.HasPrefix(rel, "archtest/") {
			return false
		}
		return strings.HasPrefix(rel, "sys/") || strings.HasPrefix(rel, "pkg/")
	})
	if len(files) == 0 {
		t.Fatal("matches-zero guard: walked zero capability Go files — detector is mis-scoped")
	}

	allow := newAllowlist(noGlobalBackendVarAllowlist)
	inspectedDecls := 0

	for _, gf := range files {
		for _, decl := range gf.ast.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv != nil {
					continue
				}
				inspectedDecls++
				if backendFuncRe.MatchString(d.Name.Name) {
					t.Errorf("global backend selector func %s at %s:%d — Decision 1 forbids a process-global backend setter/getter. Pass the Backend to New(...) and read Runner.Backend() on the injected instance.",
						d.Name.Name, gf.rel, gf.line(d))
				}
			case *ast.GenDecl:
				if d.Tok != token.VAR {
					continue
				}
				for _, spec := range d.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for _, name := range vs.Names {
						inspectedDecls++
						if !backendVarRe.MatchString(name.Name) {
							continue
						}
						key := gf.rel + " :: var " + name.Name
						if allow.exempt(key) {
							continue
						}
						t.Errorf("package-level backend var %q at %s:%d — a global backend store is the rejected SetPrivilegeBackend pattern. Hold the Backend on a constructed instance instead. If this is genuinely not a backend selector, add a justified, guarded entry to noGlobalBackendVarAllowlist.",
							name.Name, gf.rel, gf.line(name))
					}
				}
			}
		}
	}

	if inspectedDecls == 0 {
		t.Fatal("matches-zero guard: inspected zero top-level declarations — the AST walk is broken and the guard would pass vacuously")
	}
	allow.assertNoStale(t)
}
