package archtest

import (
	"go/ast"
	"strconv"
	"testing"
)

const sdkImportPath = "github.com/manchtools/cadestro/contract"

const sdkDefaultLocalName = "contract"

const strictTrustOption = "WithMTLSFromPEM"

const widenedTrustOption = "WithMTLSFromPEMAndSystemRoots"

const streamClientCtor = "NewClient"

const agentAddrField = "AgentAddr"

const streamMTLSConfigHelper = "configureAgentMTLS"

func TestStreamDialPinsEnrollmentCA(t *testing.T) {
	root := moduleRoot(t)
	files := walkGoFiles(t, root, func(string) bool { return true })
	if len(files) == 0 {
		t.Fatal("matches-zero guard: walked zero production Go files — the detector is mis-scoped")
	}

	sites := 0
	helperOptions := make(map[string]map[string]bool)
	for _, gf := range files {
		for _, decl := range gf.ast.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil || fd.Name.Name != streamMTLSConfigHelper {
				continue
			}
			helperSDKName, ok := sdkLocalName(gf.ast)
			if ok {
				helperOptions[fd.Name.Name] = sdkCallNames(fd, helperSDKName)
			}
		}
	}
	for _, gf := range files {
		sdkName, ok := sdkLocalName(gf.ast)
		if !ok {
			continue
		}
		for _, decl := range gf.ast.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			if !buildsStreamClient(fd, sdkName) {
				continue
			}
			sites++
			opts := sdkCallNames(fd, sdkName)
			if opts[widenedTrustOption] {
				t.Errorf("%s :: %s builds the control-stream client but configures mTLS with sdk.%s — the stream's server trust must be the enrollment CA ALONE (sdk.%s). Unioning the host's system roots lets any publicly-trusted certificate answering for control's hostname terminate the agent's privileged stream.",
					gf.rel, fd.Name.Name, widenedTrustOption, strictTrustOption)
			}
			if !opts[strictTrustOption] && !directCall(fd, streamMTLSConfigHelper) {
				t.Errorf("%s :: %s builds the control-stream client but calls no sdk.%s in the same function — this guard can no longer see which trust anchor the stream dial uses. Keep the option construction at the dial site, or re-point this guard at wherever it moved.",
					gf.rel, fd.Name.Name, strictTrustOption)
			}
			if directCall(fd, streamMTLSConfigHelper) {
				opts, ok := helperOptions[streamMTLSConfigHelper]
				if !ok || !opts[strictTrustOption] {
					t.Errorf("%s :: %s delegates mTLS configuration to %s, but that helper does not call sdk.%s", gf.rel, fd.Name.Name, streamMTLSConfigHelper, strictTrustOption)
				}
			}
		}
	}
	if sites == 0 {
		t.Fatalf("matches-zero guard: no function in the module both calls sdk.%s and references .%s — the stream dial site moved or was renamed, so this guard now pins nothing. Re-point it at the new construction site.",
			streamClientCtor, agentAddrField)
	}
}

func directCall(fd *ast.FuncDecl, name string) bool {
	called := false
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		id, ok := call.Fun.(*ast.Ident)
		if ok && id.Name == name {
			called = true
		}
		return true
	})
	return called
}

func TestSystemRootsTrustIsConfinedToThePublicCAEndpoint(t *testing.T) {
	root := moduleRoot(t)
	files := walkGoFiles(t, root, func(string) bool { return true })
	if len(files) == 0 {
		t.Fatal("matches-zero guard: walked zero production Go files — the detector is mis-scoped")
	}

	seen := 0
	for _, gf := range files {
		sdkName, ok := sdkLocalName(gf.ast)
		if !ok {
			continue
		}
		ast.Inspect(gf.ast, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sdkCallName(call, sdkName) != widenedTrustOption {
				return true
			}
			seen++
			t.Errorf("%s:%d: sdk.%s widens trust in the agent binary; keep the control stream on sdk.%s.",
				gf.rel, gf.line(call), widenedTrustOption, strictTrustOption)
			return true
		})
	}
}

func sdkLocalName(file *ast.File) (string, bool) {
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path != sdkImportPath {
			continue
		}
		if imp.Name == nil {
			return sdkDefaultLocalName, true
		}
		if imp.Name.Name == "_" || imp.Name.Name == "." {
			return "", false
		}
		return imp.Name.Name, true
	}
	return "", false
}

func sdkCallName(call *ast.CallExpr, sdkName string) string {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || id.Name != sdkName {
		return ""
	}
	return sel.Sel.Name
}

func sdkCallNames(fd *ast.FuncDecl, sdkName string) map[string]bool {
	out := make(map[string]bool)
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if name := sdkCallName(call, sdkName); name != "" {
				out[name] = true
			}
		}
		return true
	})
	return out
}

func buildsStreamClient(fd *ast.FuncDecl, sdkName string) bool {
	var ctor, streamAddr bool
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			if sdkCallName(x, sdkName) == streamClientCtor {
				ctor = true
			}
		case *ast.SelectorExpr:
			if x.Sel.Name == agentAddrField {
				streamAddr = true
			}
		}
		return true
	})
	return ctor && streamAddr
}
