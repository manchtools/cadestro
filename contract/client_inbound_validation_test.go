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

const (
	validULID = "01HQ0000000000000000000000"
	badULID   = "not-a-ulid"
)

func TestDispatch_RejectsInvalidInboundCommands(t *testing.T) {
	mkOSQuery := func(id string) *cadestrov1.ServerMessage {
		return &cadestrov1.ServerMessage{Id: &cadestrov1.MessageId{Value: "m"}, Payload: &cadestrov1.ServerMessage_Query{
			Query: &cadestrov1.OSQuery{QueryId: &cadestrov1.QueryId{Value: id}, Table: "processes"}}}
	}
	mkInventory := func(id string) *cadestrov1.ServerMessage {
		return &cadestrov1.ServerMessage{Id: &cadestrov1.MessageId{Value: "m"}, Payload: &cadestrov1.ServerMessage_RequestInventory{
			RequestInventory: &cadestrov1.RequestInventory{QueryId: &cadestrov1.QueryId{Value: id}}}}
	}
	mkLogQuery := func(id string) *cadestrov1.ServerMessage {
		return &cadestrov1.ServerMessage{Id: &cadestrov1.MessageId{Value: "m"}, Payload: &cadestrov1.ServerMessage_LogQuery{
			LogQuery: &cadestrov1.LogQuery{QueryId: &cadestrov1.QueryId{Value: id}}}}
	}
	mkLuks := func(id string) *cadestrov1.ServerMessage {
		return &cadestrov1.ServerMessage{Id: &cadestrov1.MessageId{Value: "m"}, Payload: &cadestrov1.ServerMessage_RevokeLuksDeviceKey{
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

func settle() { time.Sleep(150 * time.Millisecond) }

func TestDispatchValidatesEveryInboundCommand(t *testing.T) {

	md := (&cadestrov1.ServerMessage{}).ProtoReflect().Descriptor()
	oneof := md.Oneofs().ByName("payload")
	if oneof == nil {
		t.Fatal("ServerMessage has no 'payload' oneof — descriptor drift")
	}
	validatable := map[string]bool{}
	for i := 0; i < oneof.Fields().Len(); i++ {
		fd := oneof.Fields().Get(i)
		if fd.Message() == nil {
			continue
		}
		if messageHasValidateRule(fd.Message(), map[protoreflect.FullName]bool{}) {
			validatable["ServerMessage_"+goCamel(string(fd.Name()))] = true
		}
	}
	if len(validatable) == 0 {
		t.Fatal("matches-zero guard: discovered zero validatable ServerMessage oneof arms — descriptor/registry drift?")
	}

	cases := parseDispatchCases(t)

	commandArms := 0
	for wrapper := range validatable {
		info, handled := cases[wrapper]
		if !handled {
			t.Errorf("oneof arm %q carries buf.validate rules but has no dispatchServerMessage case — unhandled inbound (drift)", wrapper)
			continue
		}
		if info.deliversPending || info.lifecycle {
			continue
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

type dispatchCaseInfo struct {
	validates       bool
	deliversPending bool
	lifecycle       bool
}

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
