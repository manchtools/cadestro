package contract

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// recordingHandler implements StreamHandler plus every optional command
// interface (Inventory/LogQuery/Luks), counting how often each command
// callback is invoked. Used to prove that a malformed inbound command is
// dropped by validateInbound BEFORE it reaches the handler, and — as a
// guard against a vacuous pass — that a well-formed one still gets through.
type recordingHandler struct {
	osqueryCalls   int32
	inventoryCalls int32
	logQueryCalls  int32
	luksCalls      int32
}

func (h *recordingHandler) OnWelcome(context.Context, *cadestrov1.Welcome) error { return nil }
func (h *recordingHandler) OnQuery(context.Context, *cadestrov1.OSQuery) (*cadestrov1.OSQueryResult, error) {
	atomic.AddInt32(&h.osqueryCalls, 1)
	return nil, nil
}
func (h *recordingHandler) OnError(context.Context, *cadestrov1.Error) error { return nil }
func (h *recordingHandler) CollectInventory(context.Context) *cadestrov1.DeviceInventory {
	return nil
}
func (h *recordingHandler) OnRequestInventory(context.Context, *cadestrov1.RequestInventory) *cadestrov1.DeviceInventory {
	atomic.AddInt32(&h.inventoryCalls, 1)
	return nil
}
func (h *recordingHandler) OnLogQuery(context.Context, *cadestrov1.LogQuery) (*cadestrov1.LogQueryResult, error) {
	atomic.AddInt32(&h.logQueryCalls, 1)
	return nil, nil
}
func (h *recordingHandler) OnRevokeLuksDeviceKey(context.Context, *cadestrov1.RevokeLuksDeviceKey) (bool, string) {
	atomic.AddInt32(&h.luksCalls, 1)
	return false, ""
}

// validULID is a syntactically valid ULID; badULID fails the string.ulid
// rule the command payloads carry on their query_id / action_id field.
const (
	validULID = "01HQ0000000000000000000000"
	badULID   = "not-a-ulid"
)

// TestDispatch_RejectsInvalidInboundCommands pins the WS0 P0.3 fix: every
// command dispatch branch must run validateInbound, so a malformed-but-non-nil
// frame cannot cross the SDK boundary into a privileged handler. The
// RevokeLuksDeviceKey case matters most: it drives the irreversible LUKS slot-7
// wipe.
//
// mTLS now identifies the device, so the frames no longer carry a
// target_device_id for the boundary to check; the query_id / action_id ULID is
// what remains, and it is checked on every one of them — OSQuery included,
// which previously only had its target field covered.
//
// Each subtest asserts BOTH directions: a non-ULID id NEVER reaches the handler
// (the rejection — the point of the test), and a valid ULID DOES (so the test
// can't pass vacuously because the handler interface went unsatisfied).
func TestDispatch_RejectsInvalidInboundCommands(t *testing.T) {
	mkOSQuery := func(id string) *cadestrov1.ServerMessage {
		return &cadestrov1.ServerMessage{Id: "m", Payload: &cadestrov1.ServerMessage_Query{
			Query: &cadestrov1.OSQuery{QueryId: &cadestrov1.QueryId{Value: id}, Table: "processes"}}}
	}
	mkInventory := func(id string) *cadestrov1.ServerMessage {
		return &cadestrov1.ServerMessage{Id: "m", Payload: &cadestrov1.ServerMessage_RequestInventory{
			RequestInventory: &cadestrov1.RequestInventory{QueryId: &cadestrov1.QueryId{Value: id}}}}
	}
	mkLogQuery := func(id string) *cadestrov1.ServerMessage {
		return &cadestrov1.ServerMessage{Id: "m", Payload: &cadestrov1.ServerMessage_LogQuery{
			LogQuery: &cadestrov1.LogQuery{QueryId: &cadestrov1.QueryId{Value: id}}}}
	}
	mkLuks := func(id string) *cadestrov1.ServerMessage {
		return &cadestrov1.ServerMessage{Id: "m", Payload: &cadestrov1.ServerMessage_RevokeLuksDeviceKey{
		RevokeLuksDeviceKey: &cadestrov1.RevokeLuksDeviceKey{ActionId: &cadestrov1.ActionId{Value: id}}}}
	}

	cases := []struct {
		name  string
		build func(id string) *cadestrov1.ServerMessage
		count func(*recordingHandler) int32
	}{
		{"OSQuery", mkOSQuery, func(h *recordingHandler) int32 { return atomic.LoadInt32(&h.osqueryCalls) }},
		{"RequestInventory", mkInventory, func(h *recordingHandler) int32 { return atomic.LoadInt32(&h.inventoryCalls) }},
		{"LogQuery", mkLogQuery, func(h *recordingHandler) int32 { return atomic.LoadInt32(&h.logQueryCalls) }},
		{"RevokeLuksDeviceKey", mkLuks, func(h *recordingHandler) int32 { return atomic.LoadInt32(&h.luksCalls) }},
	}
	if len(cases) == 0 {
		t.Fatal("matches-zero: no inbound command surfaces under test")
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name+"/invalid_id_never_reaches_handler", func(t *testing.T) {
			c := NewClient("https://gw.invalid", WithAuth(validULID, ""))
			h := &recordingHandler{}
			if err := c.dispatchServerMessage(context.Background(), tc.build(badULID), h); err != nil {
				t.Fatalf("dispatch: %v", err)
			}
			// RequestInventory and RevokeLuksDeviceKey run the handler on a
			// spawned goroutine; settle so an unguarded dispatch would have
			// invoked it before we assert zero.
			settle()
			if got := tc.count(h); got != 0 {
				t.Fatalf("%s: a non-ULID id reached the handler %d time(s); validateInbound must drop it at the boundary", tc.name, got)
			}
		})

		t.Run(tc.name+"/valid_id_reaches_handler", func(t *testing.T) {
			c := NewClient("https://gw.invalid", WithAuth(validULID, ""))
			h := &recordingHandler{}
			if err := c.dispatchServerMessage(context.Background(), tc.build(validULID), h); err != nil {
				t.Fatalf("dispatch: %v", err)
			}
			waitForCond(t, func() bool { return tc.count(h) == 1 })
		})
	}
}

// settle waits a short, fixed period for a dispatch-spawned goroutine to have
// run, so a "handler must NOT be called" assertion is not racing the spawn.
func settle() { time.Sleep(150 * time.Millisecond) }

// TestDispatchValidatesEveryInboundCommand is the self-discovering regression
// guard for P0.3: it walks EVERY ServerMessage oneof arm whose payload carries
// buf.validate rules and asserts that dispatchServerMessage runs validateInbound
// for it — so a newly-added command RPC cannot silently skip validation again.
//
// Exemptions are by intrinsic KIND, not a name list:
//   - response arms whose case delivers to a pending caller (deliverPending) —
//     validated at the request site, not at dispatch; and
//   - connection-lifecycle arms (OnWelcome / OnError) — these carry no
//     operator-issued command parameters that drive privileged device work.
//
// The set is discovered from the proto descriptor + the dispatch AST, with a
// matches-zero guard and a guard that every discovered arm is actually handled
// by a dispatch case, so descriptor/registry drift can't pass the check
// vacuously.
func TestDispatchValidatesEveryInboundCommand(t *testing.T) {
	// 1. Discover, from the descriptor, the ServerMessage oneof arms whose Go
	//    payload type carries buf.validate rules.
	md := (&cadestrov1.ServerMessage{}).ProtoReflect().Descriptor()
	oneof := md.Oneofs().ByName("payload")
	if oneof == nil {
		t.Fatal("ServerMessage has no 'payload' oneof — descriptor drift")
	}
	validatable := map[string]bool{} // wrapper Go type name -> has buf.validate rules
	for i := 0; i < oneof.Fields().Len(); i++ {
		fd := oneof.Fields().Get(i)
		if fd.Message() == nil {
			continue // scalar oneof arm (none today)
		}
		if messageHasValidateRule(fd.Message(), map[protoreflect.FullName]bool{}) {
			validatable["ServerMessage_"+goCamel(string(fd.Name()))] = true
		}
	}
	if len(validatable) == 0 {
		t.Fatal("matches-zero guard: discovered zero validatable ServerMessage oneof arms — descriptor/registry drift?")
	}

	// 2. Classify each dispatch case from the AST.
	cases := parseDispatchCases(t)

	commandArms := 0
	for wrapper := range validatable {
		info, handled := cases[wrapper]
		if !handled {
			t.Errorf("oneof arm %q carries buf.validate rules but has no dispatchServerMessage case — unhandled inbound (drift)", wrapper)
			continue
		}
		if info.deliversPending || info.lifecycle {
			continue // exempt by kind (response / lifecycle)
		}
		commandArms++
		if !info.validates {
			t.Errorf("dispatchServerMessage case %q drives a command handler but does NOT call validateInbound — inbound-validation gap (WS0 P0.3)", wrapper)
		}
	}
	if commandArms == 0 {
		t.Fatal("matches-zero guard: classified zero command arms requiring validateInbound — AST classifier drift?")
	}
}

// dispatchCaseInfo records what an AST dispatch case does, for kind-based
// classification.
type dispatchCaseInfo struct {
	validates       bool // calls c.validateInbound(...)
	deliversPending bool // calls c.deliverPending(...) — request-response arm
	lifecycle       bool // calls handler.OnWelcome / handler.OnError — lifecycle arm
}

// parseDispatchCases parses client.go, locates dispatchServerMessage's type
// switch, and returns per-arm (ServerMessage_X wrapper name) classification.
func parseDispatchCases(t *testing.T) map[string]dispatchCaseInfo {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "client.go", nil, 0)
	if err != nil {
		t.Fatalf("parse client.go: %v", err)
	}

	out := map[string]dispatchCaseInfo{}
	var fn *ast.FuncDecl
	for _, decl := range f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok && fd.Name.Name == "dispatchServerMessage" {
			fn = fd
			break
		}
	}
	if fn == nil {
		t.Fatal("dispatchServerMessage not found in client.go — refactor without updating the parity guard?")
	}

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		cc, ok := n.(*ast.CaseClause)
		if !ok {
			return true
		}
		// Collect the ServerMessage_X wrapper names this case matches.
		var wrappers []string
		for _, expr := range cc.List {
			star, ok := expr.(*ast.StarExpr)
			if !ok {
				continue
			}
			sel, ok := star.X.(*ast.SelectorExpr)
			if !ok {
				continue
			}
			if strings.HasPrefix(sel.Sel.Name, "ServerMessage_") {
				wrappers = append(wrappers, sel.Sel.Name)
			}
		}
		if len(wrappers) == 0 {
			return true
		}
		// Scan the case body for the classifying calls.
		var info dispatchCaseInfo
		for _, stmt := range cc.Body {
			ast.Inspect(stmt, func(m ast.Node) bool {
				call, ok := m.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				switch sel.Sel.Name {
				case "validateInbound":
					info.validates = true
				case "deliverPending":
					info.deliversPending = true
				case "OnWelcome", "OnError":
					info.lifecycle = true
				}
				return true
			})
		}
		for _, w := range wrappers {
			out[w] = info
		}
		return true
	})
	return out
}

// messageGoType resolves a proto message full name to its generated Go pointer
// type via the global type registry.
// messageHasValidateRule reports whether md (or any nested message type
// reachable from it) carries a buf.validate field or message rule. seen
// guards against recursive proto types; pass a fresh map per top-level query.
func messageHasValidateRule(md protoreflect.MessageDescriptor, seen map[protoreflect.FullName]bool) bool {
	if seen[md.FullName()] {
		return false
	}
	seen[md.FullName()] = true
	if proto.HasExtension(md.Options(), validate.E_Message) {
		return true
	}
	fields := md.Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if proto.HasExtension(fd.Options(), validate.E_Field) {
			return true
		}
		if fd.Message() != nil && messageHasValidateRule(fd.Message(), seen) {
			return true
		}
	}
	return false
}

// goCamel converts a proto field name (snake_case) to the Go camel-case used in
// the generated oneof wrapper type name (matches protoc-gen-go for the
// digit-free field names of ServerMessage).
func goCamel(snake string) string {
	parts := strings.Split(snake, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, "")
}
