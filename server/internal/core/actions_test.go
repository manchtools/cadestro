package core

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
	"google.golang.org/protobuf/proto"
)

func TestValidateActionRequiresComplianceDetection(t *testing.T) {
	t.Parallel()
	_, err := validateAction(
		cadestrov1.DesiredState_DESIRED_STATE_PRESENT,
		0,
		nil,
		nil,
		nil,
		&cadestrov1.ShellActionParams{Script: "true", IsCompliance: true},
	)
	if err == nil || err.Error() != "compliance action requires a detection script" {
		t.Fatalf("validateAction() error = %v", err)
	}
}

func TestStoredActionRoundTripAndRejectsInvalidMetadata(t *testing.T) {
	cases := []*cadestrov1.Action{
		{DesiredState: cadestrov1.DesiredState_DESIRED_STATE_PRESENT, Params: &cadestrov1.Action_Package{Package: &cadestrov1.PackageActionParams{Name: "pkg", Version: "1"}}},
		{DesiredState: cadestrov1.DesiredState_DESIRED_STATE_PRESENT, Params: &cadestrov1.Action_Update{Update: &cadestrov1.UpdateActionParams{}}},
		{DesiredState: cadestrov1.DesiredState_DESIRED_STATE_PRESENT, Params: &cadestrov1.Action_Shell{Shell: &cadestrov1.ShellActionParams{Script: "true"}}},
	}
	var validBlob []byte
	for _, expected := range cases {
		validated, err := validateAction(expected.DesiredState, expected.TimeoutSeconds, expected.Schedule, expected.GetPackage(), expected.GetUpdate(), expected.GetShell())
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
		stored := &db.Action{ID: expected.GetId().GetValue(), ActionBlob: blob, CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0)}
		managed, err := actionProto(stored)
		if err != nil {
			t.Fatal(err)
		}
		executable, err := executableAction(stored)
		if err != nil || !proto.Equal(expected, executable) {
			t.Fatalf("executable action = %v, err = %v", executable, err)
		}
		if managed.GetParams() == nil {
			t.Fatalf("managed action = %v", managed)
		}
		if expected.GetShell() != nil {
			validBlob = blob
		}
	}
	missingKindBlob, err := proto.Marshal(&cadestrov1.Action{Id: &cadestrov1.ActionId{Value: "action-1"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []*db.Action{
		{ID: "action-1", ActionBlob: []byte("bad")},
		{ID: "action-1", ActionBlob: missingKindBlob},
		{ID: "action-2", ActionBlob: validBlob},
	} {
		if _, err := actionProto(invalid); err == nil {
			t.Fatalf("expected invalid stored action %v", invalid)
		}
	}
}

func TestUpdateActionParamsRejectsKindChange(t *testing.T) {
	service, ctx, _, _ := testService(t)
	created, err := service.CreateAction(ctx, connect.NewRequest(&cadestrov1.CreateActionRequest{
		Name: "update", DesiredState: cadestrov1.DesiredState_DESIRED_STATE_PRESENT,
		Params: &cadestrov1.CreateActionRequest_Update{Update: &cadestrov1.UpdateActionParams{}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.UpdateActionParams(ctx, connect.NewRequest(&cadestrov1.UpdateActionParamsRequest{
		Id: created.Msg.Action.Id, DesiredState: cadestrov1.DesiredState_DESIRED_STATE_PRESENT,
		Params: &cadestrov1.UpdateActionParamsRequest_Shell{Shell: &cadestrov1.ShellActionParams{Script: "true"}},
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("UpdateActionParams() error = %v", err)
	}
}
