package archtest

import (
	"go/ast"
	"strconv"
	"testing"
)

// sdkImportPath is the import path of the contract's root package, which
// owns the stream client and its mTLS ClientOption constructors.
const sdkImportPath = "github.com/manchtools/cadestro/contract"

// sdkDefaultLocalName is the package name that import path declares, used
// when a file imports it without an alias. A wrong value here is silent:
// sdkLocalName would hand the matchers an identifier no selector uses, and
// every guard below would skip the file instead of failing.
const sdkDefaultLocalName = "contract"

// strictTrustOption is the SDK option whose server-verification RootCAs
// pool is EXACTLY the CA PEM handed to it (the CA this device enrolled
// with), at TLS 1.3, with the host's system roots deliberately absent.
const strictTrustOption = "WithMTLSFromPEM"

// widenedTrustOption is retained as a reusable SDK capability, but is never a
// valid option at the agent's control-stream dial site.
const widenedTrustOption = "WithMTLSFromPEMAndSystemRoots"

// streamClientCtor is the SDK constructor for the bidirectional
// AgentService stream client.
const streamClientCtor = "NewClient"

// agentAddrField is the credentials field naming the control-stream
// endpoint; a function that both constructs an SDK client and names this
// field is a stream dial site.
const agentAddrField = "AgentAddr"

// streamMTLSConfigHelper is the small helper used by runAgent to select the
// active/pending credential and construct the strict mTLS option. Keeping the
// helper name here lets this guard follow that indirection without weakening
// the exact option check.
const streamMTLSConfigHelper = "configureAgentMTLS"

// TestStreamDialPinsEnrollmentCA pins the TLS trust posture of the agent's
// control-stream dial.
//
// The stream is the agent's standing, fully-privileged channel: it carries
// policy synchronization, terminal I/O, LUKS passphrases and LPS passwords. Its
// server-verification trust must therefore be the enrollment CA ALONE —
// sdk.WithMTLSFromPEM sets RootCAs to exactly that CA, so no publicly
// trusted certificate can impersonate control however the agent's DNS or
// routing is subverted.
//
// The failure this guard exists to catch is a one-identifier edit. Swapping
// the strict option compiles and can pass behavioural tests, so this guard
// keeps the standing privileged stream pinned to the enrollment CA.
//
// Discovery is self-locating: the whole module is walked and a dial site is
// any function that both calls <sdk>.NewClient and references .AgentAddr,
// so the guard follows the code when it moves between files or functions
// (it covers runAgent and runSelfTest today). Finding no site at all is a
// FATAL matches-zero failure, not a pass — an unlocatable dial site means
// the guard pins nothing.
//
// The required option is asserted by exact selector name, not by substring:
// strictTrustOption is a prefix of widenedTrustOption, so a textual
// "contains WithMTLSFromPEM" check would happily accept the swap it is
// supposed to reject.
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

// TestSystemRootsTrustIsConfinedToThePublicCAEndpoint ensures the widened
// reusable SDK capability is not wired into this agent binary.
//
// A "the dial site does not call sdk.WithMTLSFromPEMAndSystemRoots"
// A non-zero result is a failure; the generic option remains available to
// other SDK consumers without becoming a control-stream trust escape hatch.
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

// sdkLocalName returns the identifier under which file refers to the SDK
// root package, and whether the file imports it in a usable form. Blank and
// dot imports yield false: neither can produce a <sdk>.Foo selector, so
// there is nothing for the matchers to key on.
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

// sdkCallName returns the SDK function name for a call of the form
// <sdkName>.Foo(...), or "" for anything else. Matching is on the exact
// selector identifier, so similarly named options never alias each other.
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

// sdkCallNames returns the set of SDK package functions called anywhere in
// fd's body, closures included.
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

// buildsStreamClient reports whether fd is a control-stream dial site: it
// constructs an SDK client and names the credentials' stream endpoint in
// the same function. Requiring both keeps out the log statements that
// mention creds.AgentAddr without dialling anything.
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
