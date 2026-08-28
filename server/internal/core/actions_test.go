package core

import (
	"testing"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
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
