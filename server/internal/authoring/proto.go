package authoring

import (
	"fmt"

	"google.golang.org/protobuf/types/known/timestamppb"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/actionparams"
	"github.com/manchtools/cadestro/server/internal/store"
)

// ActionToProto decodes one trusted stored Action for API consumers that need
// the same complete authoring representation as the Action handlers.
func ActionToProto(row store.ActionRow) (*cadestrov1.ManagedAction, error) {
	action := &cadestrov1.ManagedAction{
		Id: row.ID, Name: row.Name, Type: cadestrov1.ActionType(row.ActionType),
		DesiredState: cadestrov1.DesiredState(row.DesiredState), TimeoutSeconds: row.TimeoutSeconds,
		CreatedBy: row.CreatedBy,
	}
	if row.Description != nil {
		action.Description = *row.Description
	}
	if row.CreatedAt != nil {
		action.CreatedAt = timestamppb.New(*row.CreatedAt)
	}
	if row.UpdatedAt != nil {
		action.UpdatedAt = timestamppb.New(*row.UpdatedAt)
	}
	if err := populateManagedParams(action, action.Type, row.Params); err != nil {
		return nil, fmt.Errorf("authoring: decode stored action params: %w", err)
	}
	schedule, err := actionparams.ParseSchedule(row.Schedule)
	if err != nil {
		return nil, fmt.Errorf("authoring: decode stored action schedule: %w", err)
	}
	action.Schedule = schedule
	return action, nil
}

// ActionSetToProto decodes one trusted stored ActionSet.
func ActionSetToProto(row store.ActionSetRow, memberCount int64) (*cadestrov1.ActionSet, error) {
	if !validFailurePolicy(cadestrov1.OnFailure(row.OnFailure)) {
		return nil, fmt.Errorf("authoring: invalid stored action set failure policy %d", row.OnFailure)
	}
	schedule, err := actionparams.ParseSchedule(row.Schedule)
	if err != nil {
		return nil, fmt.Errorf("authoring: decode stored action set schedule: %w", err)
	}
	if schedule == nil {
		return nil, fmt.Errorf("authoring: stored action set schedule is empty")
	}
	set := &cadestrov1.ActionSet{
		Id: row.ID, Name: row.Name, Description: row.Description,
		MemberCount: boundedCount(memberCount), CreatedBy: row.CreatedBy,
		Schedule: schedule, OnFailure: cadestrov1.OnFailure(row.OnFailure),
	}
	if row.CreatedAt != nil {
		set.CreatedAt = timestamppb.New(*row.CreatedAt)
	}
	if row.UpdatedAt != nil {
		set.UpdatedAt = timestamppb.New(*row.UpdatedAt)
	}
	return set, nil
}

// ActionSetMembersToProto converts the ordered member edge list.
func ActionSetMembersToProto(rows []store.ActionSetMemberView) []*cadestrov1.ActionSetMember {
	members := make([]*cadestrov1.ActionSetMember, len(rows))
	for i, row := range rows {
		members[i] = &cadestrov1.ActionSetMember{
			ActionId: &cadestrov1.ActionId{Value: row.ActionID}, SortOrder: row.SortOrder,
			ActionName: row.ActionName, ActionType: cadestrov1.ActionType(row.ActionType),
		}
	}
	return members
}

// DefinitionToProto decodes one trusted stored Definition.
func DefinitionToProto(row store.DefinitionRow, memberCount int64) (*cadestrov1.Definition, error) {
	schedule, err := actionparams.ParseSchedule(row.Schedule)
	if err != nil {
		return nil, fmt.Errorf("authoring: decode stored definition schedule: %w", err)
	}
	if schedule == nil {
		return nil, fmt.Errorf("authoring: stored definition schedule is empty")
	}
	definition := &cadestrov1.Definition{
		Id: row.ID, Name: row.Name, Description: row.Description,
		MemberCount: boundedCount(memberCount), CreatedBy: row.CreatedBy, Schedule: schedule,
	}
	if row.CreatedAt != nil {
		definition.CreatedAt = timestamppb.New(*row.CreatedAt)
	}
	if row.UpdatedAt != nil {
		definition.UpdatedAt = timestamppb.New(*row.UpdatedAt)
	}
	return definition, nil
}

// DefinitionMembersToProto converts the ordered member edge list.
func DefinitionMembersToProto(rows []store.DefinitionMemberView) []*cadestrov1.DefinitionMember {
	members := make([]*cadestrov1.DefinitionMember, len(rows))
	for i, row := range rows {
		members[i] = &cadestrov1.DefinitionMember{
			ActionSetId: &cadestrov1.ActionSetId{Value: row.ActionSetID}, SortOrder: row.SortOrder, ActionSetName: row.ActionSetName,
		}
	}
	return members
}
