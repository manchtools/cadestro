package contract_test

import (
	"buf.build/go/protovalidate"
	"strings"
	"testing"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestActionResultContainsObservationsOnly(t *testing.T) {
	fields := (&cadestrov1.ActionResult{}).ProtoReflect().Descriptor().Fields()
	for _, name := range []protoreflect.Name{"error", "compliant"} {
		if fields.ByName(name) != nil {
			t.Fatalf("ActionResult retains server-derived field %q", name)
		}
	}
}

func TestActionAndStreamPayloadsAreRequired(t *testing.T) {
	for _, message := range []proto.Message{
		&cadestrov1.Action{},
		&cadestrov1.ManagedAction{},
		&cadestrov1.CreateActionRequest{},
		&cadestrov1.ConfigureActionRequest{},
		&cadestrov1.AgentMessage{Id: &cadestrov1.MessageId{Value: "01K00000000000000000000001"}},
		&cadestrov1.ServerMessage{Id: &cadestrov1.MessageId{Value: "01K00000000000000000000002"}},
	} {
		if err := protovalidate.Validate(message); err == nil {
			t.Fatalf("%T accepted a missing oneof", message)
		}
	}
}

func TestActionResultRequiresCompletionAndDigest(t *testing.T) {
	valid := &cadestrov1.ActionResult{
		ActionId:     &cadestrov1.ActionId{Value: "01K00000000000000000000003"},
		RunId:        &cadestrov1.RunId{Value: "01K00000000000000000000004"},
		Status:       cadestrov1.ExecutionStatus_EXECUTION_STATUS_SUCCESS,
		CompletedAt:  timestamppb.Now(),
		ActionDigest: make([]byte, 32),
	}
	if err := protovalidate.Validate(valid); err != nil {
		t.Fatal(err)
	}
	missingCompletion := proto.CloneOf(valid)
	missingCompletion.CompletedAt = nil
	if err := protovalidate.Validate(missingCompletion); err == nil {
		t.Fatal("action result accepted missing completion time")
	}
	wrongDigest := proto.CloneOf(valid)
	wrongDigest.ActionDigest = []byte{1}
	if err := protovalidate.Validate(wrongDigest); err == nil {
		t.Fatal("action result accepted wrong digest length")
	}
}

func TestControlServiceUsesNamedOperations(t *testing.T) {
	file := (&cadestrov1.RefreshTokenRequest{}).ProtoReflect().Descriptor().ParentFile()
	service := file.Services().ByName("ControlService")
	inputs := make(map[protoreflect.FullName]protoreflect.Name, service.Methods().Len())
	outputs := make(map[protoreflect.FullName]protoreflect.Name, service.Methods().Len())
	for index := range service.Methods().Len() {
		method := service.Methods().Get(index)
		if strings.HasPrefix(string(method.Name()), "Update") {
			t.Fatalf("ControlService retains ambiguous method %q", method.Name())
		}
		if previous, exists := inputs[method.Input().FullName()]; exists {
			t.Fatalf("%s and %s share input %s", previous, method.Name(), method.Input().FullName())
		}
		inputs[method.Input().FullName()] = method.Name()
		if previous, exists := outputs[method.Output().FullName()]; exists {
			t.Fatalf("%s and %s share output %s", previous, method.Name(), method.Output().FullName())
		}
		outputs[method.Output().FullName()] = method.Name()
	}
	for index := range file.Messages().Len() {
		message := file.Messages().Get(index)
		if strings.HasPrefix(string(message.Name()), "Update") {
			t.Fatalf("control.proto retains ambiguous message %q", message.Name())
		}
	}
	update := (&cadestrov1.UpdateActionParams{}).ProtoReflect().Descriptor()
	if update.ParentFile().Path() != "cadestro/v1/actions.proto" {
		t.Fatalf("UpdateActionParams belongs to %q", update.ParentFile().Path())
	}
}

func TestActionSurfaceIsTheCoreThree(t *testing.T) {
	for _, message := range []protoreflect.MessageDescriptor{
		(&cadestrov1.Action{}).ProtoReflect().Descriptor(),
		(&cadestrov1.ManagedAction{}).ProtoReflect().Descriptor(),
		(&cadestrov1.CreateActionRequest{}).ProtoReflect().Descriptor(),
		(&cadestrov1.ConfigureActionRequest{}).ProtoReflect().Descriptor(),
	} {
		if message.Fields().ByName("type") != nil {
			t.Fatalf("%s retains a redundant action discriminator", message.FullName())
		}
		oneof := message.Oneofs().ByName("params")
		if oneof == nil || oneof.Fields().Len() != 3 {
			t.Fatalf("%s params oneof is incomplete", message.FullName())
		}
		for index, name := range []protoreflect.Name{"package", "update", "shell"} {
			if oneof.Fields().Get(index).Name() != name {
				t.Fatalf("%s params arm %d is %q, want %q", message.FullName(), index, oneof.Fields().Get(index).Name(), name)
			}
		}
	}
}

func TestListActionsRequestMatchesImplementedFilters(t *testing.T) {
	fields := (&cadestrov1.ListActionsRequest{}).ProtoReflect().Descriptor().Fields()
	want := []protoreflect.Name{"page_size", "page_token"}
	if fields.Len() != len(want) {
		t.Fatalf("ListActionsRequest has %d fields, want %d", fields.Len(), len(want))
	}
	for index, name := range want {
		if fields.Get(index).Name() != name {
			t.Fatalf("ListActionsRequest field %d is %q, want %q", index, fields.Get(index).Name(), name)
		}
	}
}

func TestListDevicesRequestMatchesImplementedFilters(t *testing.T) {
	fields := (&cadestrov1.ListDevicesRequest{}).ProtoReflect().Descriptor().Fields()
	want := []protoreflect.Name{"page_size", "page_token"}
	if fields.Len() != len(want) {
		t.Fatalf("ListDevicesRequest has %d fields, want %d", fields.Len(), len(want))
	}
	for index, name := range want {
		if fields.Get(index).Name() != name {
			t.Fatalf("ListDevicesRequest field %d is %q, want %q", index, fields.Get(index).Name(), name)
		}
	}
}
