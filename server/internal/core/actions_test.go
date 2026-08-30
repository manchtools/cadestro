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
	value := &cadestrov1.Action{Id: &cadestrov1.ActionId{Value: "action-1"}, Type: cadestrov1.ActionType_ACTION_TYPE_SHELL, DesiredState: cadestrov1.DesiredState_DESIRED_STATE_PRESENT, Params: &cadestrov1.Action_Shell{Shell: &cadestrov1.ShellParams{Script: "true"}}}
	blob, err := proto.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	stored := &db.Action{ID: "action-1", Type: value.Type, ActionBlob: blob, CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0)}
	managed, err := actionProto(stored)
	if err != nil {
		t.Fatal(err)
	}
	if managed.GetShell().GetScript() != "true" {
		t.Fatalf("managed action = %v", managed)
	}
	for _, invalid := range []*db.Action{{ID: stored.ID, Type: stored.Type, ActionBlob: []byte("bad")}, {ID: stored.ID, Type: cadestrov1.ActionType_ACTION_TYPE_PACKAGE, ActionBlob: blob}} {
		if _, err := actionProto(invalid); err == nil {
			t.Fatalf("expected invalid stored action %v", invalid)
		}
	}
}

func TestValidateActionBuildsConcreteParameters(t *testing.T) {
	cases := []struct {
		name          string
		typeID        cadestrov1.ActionType
		packageParams *cadestrov1.PackageParams
		updateParams  *cadestrov1.UpdateParams
		shellParams   *cadestrov1.ShellParams
	}{
		{name: "package", typeID: cadestrov1.ActionType_ACTION_TYPE_PACKAGE, packageParams: &cadestrov1.PackageParams{Name: "pkg"}},
		{name: "update", typeID: cadestrov1.ActionType_ACTION_TYPE_UPDATE, updateParams: &cadestrov1.UpdateParams{}},
		{name: "shell", typeID: cadestrov1.ActionType_ACTION_TYPE_SHELL, shellParams: &cadestrov1.ShellParams{Script: "true"}},
	}
	for _, testCase := range cases {
		action, err := validateAction(testCase.typeID, cadestrov1.DesiredState_DESIRED_STATE_PRESENT, 0, nil, testCase.packageParams, testCase.updateParams, testCase.shellParams)
		if err != nil || action.GetType() != testCase.typeID || action.GetParams() == nil {
			t.Fatalf("%s: action=%v err=%v", testCase.name, action, err)
		}
	}
}
