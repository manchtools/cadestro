package contract_test

import (
	"testing"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
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
