package contract_test

import (
	"testing"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestActionSurfaceIsTheCoreThree(t *testing.T) {
	want := map[cadestrov1.ActionType]bool{
		cadestrov1.ActionType_ACTION_TYPE_UNSPECIFIED: true,
		cadestrov1.ActionType_ACTION_TYPE_PACKAGE:     true,
		cadestrov1.ActionType_ACTION_TYPE_UPDATE:      true,
		cadestrov1.ActionType_ACTION_TYPE_SHELL:       true,
	}
	if len(cadestrov1.ActionType_name) != len(want) {
		t.Fatalf("ActionType has %d values, want %d", len(cadestrov1.ActionType_name), len(want))
	}
	for value := range cadestrov1.ActionType_name {
		if !want[cadestrov1.ActionType(value)] {
			t.Fatalf("unexpected action type %d", value)
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
