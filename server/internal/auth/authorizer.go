package auth

import (
	"context"
	"sort"
)

var assignedPermissionBases = map[string]bool{
	"GetDevice":                       true,
	"GetDeviceCompliance":             true,
	"GetDeviceCompliancePolicyStatus": true,
	"ListDevices":                     true,
}

func AssignedPermissionBases() []string {
	out := make([]string, 0, len(assignedPermissionBases))
	for action := range assignedPermissionBases {
		out = append(out, action)
	}
	sort.Strings(out)
	return out
}

type AuthzInput struct {
	Permissions []string

	SubjectID string

	SelfEligible bool
	Action       string

	ResourceID string
}

func Authorize(input AuthzInput) bool {
	for _, p := range input.Permissions {
		if p == input.Action {
			return true
		}
		if p == input.Action+":self" && input.SelfEligible {
			if input.ResourceID == "" || input.ResourceID == input.SubjectID {
				return true
			}
		}
		if assignedPermissionBases[input.Action] && p == input.Action+":assigned" {
			return true
		}
	}
	return false
}

func AuthorizeContext(ctx context.Context, action, resourceID string) bool {
	user, ok := UserFromContext(ctx)
	if !ok {
		return false
	}
	return Authorize(AuthzInput{
		Permissions:  user.Permissions,
		SubjectID:    user.ID,
		SelfEligible: user.CanOwnResources(),
		Action:       action,
		ResourceID:   resourceID,
	})
}
