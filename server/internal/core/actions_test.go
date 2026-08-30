package core

import (
	"testing"
	"time"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
	"google.golang.org/protobuf/proto"
)

func TestValidateActionRejectsMismatchedParameters(t *testing.T) {
	t.Parallel()
	_, err := validateAction(
		cadestrov1.ActionType_ACTION_TYPE_PACKAGE,
		cadestrov1.DesiredState_DESIRED_STATE_PRESENT,
		0,
		nil,
		nil,
		&cadestrov1.UpdateParams{},
		nil,
	)
	if err == nil || err.Error() != "package action requires package parameters" {
		t.Fatalf("validateAction() error = %v", err)
	}
}

func TestValidateActionRequiresComplianceDetection(t *testing.T) {
	t.Parallel()
	_, err := validateAction(
		cadestrov1.ActionType_ACTION_TYPE_SHELL,
		cadestrov1.DesiredState_DESIRED_STATE_PRESENT,
		0,
		nil,
		nil,
		nil,
		&cadestrov1.ShellParams{Script: "true", IsCompliance: true},
	)
	if err == nil || err.Error() != "compliance action requires a detection script" {
		t.Fatalf("validateAction() error = %v", err)
	}
}

func TestStoredActionRoundTripAndRejectsInvalidMetadata(t *testing.T) {
	cases := []*cadestrov1.Action{
		{Type: cadestrov1.ActionType_ACTION_TYPE_PACKAGE, DesiredState: cadestrov1.DesiredState_DESIRED_STATE_PRESENT, Params: &cadestrov1.Action_Package{Package: &cadestrov1.PackageParams{Name: "pkg", Version: "1"}}},
		{Type: cadestrov1.ActionType_ACTION_TYPE_UPDATE, DesiredState: cadestrov1.DesiredState_DESIRED_STATE_PRESENT, Params: &cadestrov1.Action_Update{Update: &cadestrov1.UpdateParams{}}},
		{Type: cadestrov1.ActionType_ACTION_TYPE_SHELL, DesiredState: cadestrov1.DesiredState_DESIRED_STATE_PRESENT, Params: &cadestrov1.Action_Shell{Shell: &cadestrov1.ShellParams{Script: "true"}}},
	}
	var validBlob []byte
	for _, expected := range cases {
		validated, err := validateAction(expected.Type, expected.DesiredState, expected.TimeoutSeconds, expected.Schedule, expected.GetPackage(), expected.GetUpdate(), expected.GetShell())
		if err != nil {
			t.Fatal(err)
		}
		validated.Id = &cadestrov1.ActionId{Value: "action-1"}
		expected.Id = &cadestrov1.ActionId{Value: "action-1"}
		if !proto.Equal(expected, validated) {
			t.Fatalf("validated action = %v, expected = %v", validated, expected)
		}
		blob, err := proto.Marshal(validated)
		if err != nil {
			t.Fatal(err)
		}
		stored := &db.Action{ID: expected.GetId().GetValue(), Type: expected.Type, ActionBlob: blob, CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0)}
		managed, err := actionProto(stored)
		if err != nil {
			t.Fatal(err)
		}
		executable, err := executableAction(stored)
		if err != nil || !proto.Equal(expected, executable) {
			t.Fatalf("executable action = %v, err = %v", executable, err)
		}
		if managed.GetType() != expected.Type {
			t.Fatalf("managed action = %v", managed)
		}
		if expected.Type == cadestrov1.ActionType_ACTION_TYPE_SHELL {
			validBlob = blob
		}
	}
	for _, invalid := range []*db.Action{
		{ID: "action-1", Type: cadestrov1.ActionType_ACTION_TYPE_SHELL, ActionBlob: []byte("bad")},
		{ID: "action-1", Type: cadestrov1.ActionType_ACTION_TYPE_PACKAGE, ActionBlob: validBlob},
		{ID: "action-2", Type: cadestrov1.ActionType_ACTION_TYPE_SHELL, ActionBlob: validBlob},
	} {
		if _, err := actionProto(invalid); err == nil {
			t.Fatalf("expected invalid stored action %v", invalid)
		}
	}
}
