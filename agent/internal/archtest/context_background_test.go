package archtest

import (
	"go/ast"
	"go/token"
	"strings"
	"testing"
)

func TestNoContextBackgroundInRequestPaths(t *testing.T) {
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

	allow := newAllowlist(map[string]string{
		"internal/credentials/credentials.go :: writeFile":      "local credential-file write on the enrollment/cert-rotation path, not an RPC request; fs write is synchronous",
		"internal/deviceauth/enroll_server.go :: Shutdown":      "enrollment-socket shutdown path; no caller context to inherit; bounded 5s",
		"internal/executor/agent_update.go :: getBinaryVersion": "bounded 10s subprocess version probe during self-update verification",
		"internal/handler/handler.go :: BuildHeartbeat":         "best-effort periodic heartbeat metrics; no RPC caller; osquery bounds each call (defaultTimeout)",
		"internal/handler/terminal.go :: OnTerminalStart":       "terminal session must outlive the start RPC; sessionCtx roots its own lifecycle and teardown must run even when the start ctx is gone",
		"internal/handler/terminal.go :: pumpTerminalOutput":    "detached per-session output pump; owns sessionCtx; its sends/cleanup are bounded by their own timeouts",
		"internal/handler/terminal.go :: sweepIdleTerminals":    "background idle-terminal sweep ticker; no RPC caller; bounded cleanup",
	})

	sawCtxRoot := 0
	for _, gf := range files {
		underCmd := strings.HasPrefix(gf.rel, "cmd/")
		ast.Inspect(gf.ast, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name, ok := contextRootCall(call)
			if !ok {
				return true
			}
			sawCtxRoot++
			if underCmd {
				return true
			}
			fn := enclosingFuncName(gf.ast, call.Pos())
			if allow.exempt(gf.rel + " :: " + fn) {
				return true
			}
			t.Errorf("context.%s() rooted in a request path at %s:%d (enclosing func %q) — propagate the caller's context.Context instead; a fresh root drops the request deadline and cancellation. If this is detached daemon-lifecycle work, allowlist it by enclosing function with a justification.",
				name, gf.rel, gf.line(call), fn)
			return true
		})
	}
	if sawCtxRoot == 0 {
		t.Fatal("matches-zero guard: found no context.Background()/context.TODO() anywhere (not even in cmd/ bootstrap) — the detector is dead, the guard would pass vacuously")
	}
	allow.assertNoStale(t)
}

func contextRootCall(call *ast.CallExpr) (string, bool) {
	if len(call.Args) != 0 {
		return "", false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	if sel.Sel.Name != "Background" && sel.Sel.Name != "TODO" {
		return "", false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || id.Name != "context" {
		return "", false
	}
	return sel.Sel.Name, true
}

func enclosingFuncName(file *ast.File, pos token.Pos) string {
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if pos >= fd.Pos() && pos <= fd.End() {
			return fd.Name.Name
		}
	}
	return "<file-scope>"
}
