package contract_test

import (
	"testing"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestActionSurfaceIsTheCoreThree(t *testing.T) {
	for _, message := range []protoreflect.MessageDescriptor{
		(&cadestrov1.Action{}).ProtoReflect().Descriptor(),
		(&cadestrov1.ManagedAction{}).ProtoReflect().Descriptor(),
		(&cadestrov1.CreateActionRequest{}).ProtoReflect().Descriptor(),
		(&cadestrov1.UpdateActionParamsRequest{}).ProtoReflect().Descriptor(),
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
