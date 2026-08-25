package archtest

import (
	"go/ast"
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	_ "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

var secretLogSinkAllowlist = map[string]string{}

func TestProtoSecretFieldSinks(t *testing.T) {
	secrets := secretFieldNamesFromDescriptors(t)
	if len(secrets) == 0 {
		t.Fatal("matches-zero guard: no debug_redact fields found in the contract — either the markers were " +
			"dropped (the fields are now unguarded) or this discovery is broken")
	}

	root := moduleRoot(t)
	files := walkGoFiles(t, root, func(rel string) bool {
		if strings.HasPrefix(rel, "internal/archtest/") {
			return false
		}
		return !strings.HasSuffix(rel, "_test.go")
	})
	if len(files) == 0 {
		t.Fatal("matches-zero guard: walked zero Go files — the detector is mis-scoped")
	}

	allow := newAllowlist(secretLogSinkAllowlist)
	sawSink := false

	for _, gf := range files {
		ast.Inspect(gf.ast, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !isLogOrFormatSink(call) {
				return true
			}
			sawSink = true
			for _, arg := range call.Args {
				name, found := findSecretSelector(arg, secrets)
				if !found {
					continue
				}
				key := gf.rel + " :: " + render(gf.fset, call)
				if allow.exempt(key) {
					continue
				}
				t.Errorf("%s:%d: secret field %q reaches a log/format sink — a credential written to a log "+
					"outlives the request and the operator never sees it happen:\n\t%s",
					gf.rel, gf.line(call), name, render(gf.fset, call))
			}
			return true
		})
	}

	if !sawSink {
		t.Fatal("matches-zero guard: no logging or formatting call matched anywhere in the tree — " +
			"isLogOrFormatSink is broken and this test proves nothing")
	}
	allow.assertNoStale(t)
}

func secretFieldNamesFromDescriptors(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		if !strings.HasPrefix(string(fd.Package()), "cadestro.") {
			return true
		}
		msgs := fd.Messages()
		for i := 0; i < msgs.Len(); i++ {
			collectRedactedFields(msgs.Get(i), out)
		}
		return true
	})
	return out
}

func collectRedactedFields(md protoreflect.MessageDescriptor, out map[string]bool) {
	fields := md.Fields()
	for i := 0; i < fields.Len(); i++ {
		f := fields.Get(i)
		if opts, ok := f.Options().(*descriptorpb.FieldOptions); ok && opts.GetDebugRedact() {
			out[goFieldName(string(f.Name()))] = true
		}
	}
	nested := md.Messages()
	for i := 0; i < nested.Len(); i++ {
		collectRedactedFields(nested.Get(i), out)
	}
}

func goFieldName(protoName string) string {
	parts := strings.Split(protoName, "_")
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]))
		b.WriteString(p[1:])
	}
	return b.String()
}

func isLogOrFormatSink(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	switch sel.Sel.Name {

	case "Debug", "Info", "Warn", "Error",
		"DebugContext", "InfoContext", "WarnContext", "ErrorContext":
		return true

	case "Sprintf", "Printf", "Sprint", "Sprintln", "Println", "Print", "Fprintf", "Fprintln", "Errorf":
		return true
	}
	return false
}

func findSecretSelector(e ast.Expr, secrets map[string]bool) (string, bool) {
	var name string
	ast.Inspect(e, func(n ast.Node) bool {
		if name != "" {
			return false
		}
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		field := strings.TrimPrefix(sel.Sel.Name, "Get")
		if secrets[sel.Sel.Name] || secrets[field] {
			name = sel.Sel.Name
			return false
		}
		return true
	})
	return name, name != ""
}
