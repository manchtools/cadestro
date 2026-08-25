package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/manchtools/cadestro/server/internal/store/generated"
	"github.com/manchtools/cadestro/server/internal/store/sqlitetype"
)

type AuditOperationRow = generated.AuditOperation

type AuditEffectRow = generated.AuditEffect

type AuditEventRow = generated.AuditEventRow

type AuditEventFilter struct {
	ActorID      string
	StreamTypes  []string
	EventType    string
	OccurredFrom time.Time
	OccurredTo   time.Time
	BeforeSeq    int64
	Limit        int32
}

type DeviceRow = generated.Device

type JobRow = generated.Job

type LuksKeyRow = generated.GetCurrentLuksKeyForAgentRow

type ActionRow = generated.Action

type ActionListFilter struct {
	AfterID         string
	Limit           int32
	Type            int32
	UnassignedOnly  bool
	ScopeRestricted bool
	ScopeGroupIDs   []string
}

type AssignmentTarget = generated.ListAuthoringAssignmentsForSourceRow

type AssignmentView struct {
	ID         string
	SourceType string
	SourceID   string
	TargetType string
	TargetID   string
	Mode       int32
	CreatedAt  *time.Time
	CreatedBy  string
	SourceName string
	TargetName string
}

type ResolvedAssignmentSource = generated.ListResolvedAssignmentSourcesForDeviceRow

type UserSelectionRow = generated.UserSelection

type AssignmentListFilter struct {
	AfterID    string
	Limit      int32
	SourceType string
	SourceID   string
	TargetType string
	TargetID   string
}

type DeviceGroupView = generated.GetDeviceGroupRow

type DeviceGroupMemberView = generated.ListDeviceGroupMembersRow

type DynamicDeviceView = generated.ListDevicesForDynamicEvaluationRow

type DeviceGroupListFilter struct {
	AfterID         string
	Limit           int32
	ScopeRestricted bool
	ScopeGroupIDs   []string
}

func (s *Store) GetDeviceGroupID(ctx context.Context, id string) (string, error) {
	rowID, err := s.queries.GetDeviceGroupID(ctx, id)
	if err != nil {
		return "", fmt.Errorf("device group: get: %w", translateNotFound(err))
	}
	return rowID, nil
}

func (s *Store) GetDeviceGroup(ctx context.Context, id string) (DeviceGroupView, error) {
	row, err := s.queries.GetDeviceGroup(ctx, id)
	if err != nil {
		return DeviceGroupView{}, fmt.Errorf("device group: get: %w", translateNotFound(err))
	}
	return row, nil
}

func (s *Store) ListDeviceGroupMembers(ctx context.Context, id string) ([]DeviceGroupMemberView, error) {
	rows, err := s.queries.ListDeviceGroupMembers(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("device group: list members: %w", err)
	}
	return rows, nil
}

func (s *Store) ListDevicesForDynamicEvaluation(ctx context.Context) ([]DynamicDeviceView, error) {
	rows, err := s.queries.ListDevicesForDynamicEvaluation(ctx)
	if err != nil {
		return nil, fmt.Errorf("device group: list evaluation devices: %w", err)
	}
	return rows, nil
}

func (s *Store) ListDeviceGroups(ctx context.Context, filter DeviceGroupListFilter) ([]DeviceGroupView, error) {
	if filter.Limit < 0 || filter.Limit > 101 {
		return nil, fmt.Errorf("device group: list limit must be between 0 and 101")
	}
	if filter.Limit == 0 {
		filter.Limit = 50
	}
	rows, err := s.queries.ListDeviceGroups(ctx, generated.ListDeviceGroupsParams{
		AfterID: filter.AfterID, RowLimit: int64(filter.Limit),
		ScopeRestricted: filter.ScopeRestricted, ScopeGroupIdsJson: sqlitetype.StringList(filter.ScopeGroupIDs),
	})
	if err != nil {
		return nil, fmt.Errorf("device group: list: %w", err)
	}
	groups := make([]DeviceGroupView, len(rows))
	for i, row := range rows {
		groups[i] = DeviceGroupView(row)
	}
	return groups, nil
}

func (s *Store) CountDeviceGroups(ctx context.Context, filter DeviceGroupListFilter) (int64, error) {
	n, err := s.queries.CountDeviceGroups(ctx, generated.CountDeviceGroupsParams{
		ScopeRestricted: filter.ScopeRestricted, ScopeGroupIdsJson: sqlitetype.StringList(filter.ScopeGroupIDs),
	})
	if err != nil {
		return 0, fmt.Errorf("device group: count: %w", err)
	}
	return n, nil
}

func (s *Store) ListDeviceGroupsForDevice(ctx context.Context, deviceID string, filter DeviceGroupListFilter) ([]DeviceGroupView, error) {
	rows, err := s.queries.ListDeviceGroupsForDevice(ctx, generated.ListDeviceGroupsForDeviceParams{
		DeviceID: deviceID, ScopeRestricted: filter.ScopeRestricted, ScopeGroupIdsJson: sqlitetype.StringList(filter.ScopeGroupIDs),
	})
	if err != nil {
		return nil, fmt.Errorf("device group: list for device: %w", err)
	}
	groups := make([]DeviceGroupView, len(rows))
	for i, row := range rows {
		groups[i] = DeviceGroupView(row)
	}
	return groups, nil
}

type ActionSetRow = generated.ActionSet

type ActionSetListFilter struct {
	AfterID         string
	Limit           int32
	UnassignedOnly  bool
	ScopeRestricted bool
	ScopeGroupIDs   []string
}

type ActionSetView struct {
	ActionSetRow
	MemberCount int64
}

type ActionSetMemberView = generated.ListActionSetMembersRow

type DefinitionRow = generated.Definition

type DefinitionListFilter struct {
	AfterID         string
	Limit           int32
	ScopeRestricted bool
	ScopeGroupIDs   []string
}

type DefinitionView struct {
	DefinitionRow
	LiveMemberCount int64
}

type DefinitionMemberView = generated.ListDefinitionMembersRow

type DefinitionManifestAction struct {
	ActionSetID string
	Action      ActionRow
}

type CompliancePolicyRow = generated.CompliancePolicy

type CompliancePolicyListFilter struct {
	AfterID         string
	Limit           int32
	ScopeRestricted bool
	ScopeGroupIDs   []string
}

type CompliancePolicyView struct {
	CompliancePolicyRow
	LiveRuleCount int64
}

type CompliancePolicyRuleView = generated.ListCompliancePolicyRulesRow

type DeviceStatusFilter int32

const (
	DeviceStatusAny     DeviceStatusFilter = 0
	DeviceStatusOnline  DeviceStatusFilter = 1
	DeviceStatusOffline DeviceStatusFilter = 2
)

type DeviceListFilter struct {
	AfterID         string
	Limit           int32
	Status          DeviceStatusFilter
	OnlineSince     time.Time
	Labels          map[string]string
	AssignedUserID  *string
	ScopeRestricted bool
	ScopeGroupIDs   []string
}

type DeviceView struct {
	DeviceRow
	Labels                           map[string]string
	AssignedUserIDs                  []string
	AssignedGroupIDs                 []string
	LastInventoryAt                  *time.Time
	ResolvedInventoryIntervalMinutes int32
}

type DeviceAssigneeView struct {
	ID   string
	Kind string
	Name string
}

type DeviceInventoryTable = generated.ListDeviceInventoryRow

type OSQueryResult = generated.GetOSQueryResultRow

type DeviceLogResult = generated.GetDeviceLogResultRow

type DeviceComplianceResult = generated.ListDeviceComplianceResultsRow

type DeviceComplianceEvaluation = generated.ListDeviceComplianceEvaluationsRow

type LpsPasswordView struct {
	ID, DeviceID, DeviceHostname, ActionID, ActionName string
	Username, RotationReason                           string
	RotatedAt                                          time.Time
}

type LuksKeyView struct {
	ID, DeviceID, DeviceHostname, ActionID, ActionName string
	DevicePath, RotationReason                         string
	RotatedAt                                          time.Time
	RevocationStatus, RevocationError                  *string
	RevocationAt                                       *time.Time
}

type LpsPasswordSecret = generated.GetLpsPasswordForRevealRow
type LuksKeySecret = generated.GetLuksKeyForRevealRow
type DeviceSecretRow = generated.GetDeviceSecretRow

type LuksRevocationTarget = generated.GetLuksRevocationTargetRow

type OpenTerminalSession = generated.GetOpenTerminalSessionRow

const DefaultInventoryIntervalMinutes int32 = 1440

type UserRow = generated.User

func (s *Store) GetAuditOperation(ctx context.Context, operationID string) (AuditOperationRow, error) {
	row, err := s.queries.GetAuditOperation(ctx, operationID)
	if err != nil {
		return AuditOperationRow{}, fmt.Errorf("audit: get operation: %w", translateNotFound(err))
	}
	return row, nil
}

func (s *Store) ListAuditEffects(ctx context.Context, operationID string) ([]AuditEffectRow, error) {
	rows, err := s.queries.ListAuditEffectsForOperation(ctx, operationID)
	if err != nil {
		return nil, fmt.Errorf("audit: list effects: %w", err)
	}
	return rows, nil
}

func (s *Store) ListAuditEventRows(ctx context.Context, filter AuditEventFilter) ([]AuditEventRow, error) {
	if filter.Limit < 1 || filter.Limit > 1001 {
		return nil, fmt.Errorf("audit: list limit must be between 1 and 1001")
	}
	if filter.BeforeSeq < 0 {
		return nil, fmt.Errorf("audit: list cursor must not be negative")
	}
	if filter.OccurredFrom.IsZero() {
		filter.OccurredFrom = time.Unix(0, 0).UTC()
	}
	if filter.OccurredTo.IsZero() {
		filter.OccurredTo = time.Date(9999, 12, 31, 23, 59, 59, 999999000, time.UTC)
	}
	if filter.OccurredFrom.After(filter.OccurredTo) {
		return nil, fmt.Errorf("audit: occurred-from must not follow occurred-to")
	}
	if filter.StreamTypes == nil {
		filter.StreamTypes = []string{}
	}
	rows, err := s.queries.ListAuditEventRows(ctx, generated.ListAuditEventRowsParams{
		ActorID:         filter.ActorID,
		StreamTypesJson: sqlitetype.StringList(filter.StreamTypes),
		EventType:       filter.EventType,
		FilterFromTime:  filter.OccurredFrom,
		FilterToTime:    filter.OccurredTo,
		BeforeSeq:       filter.BeforeSeq,
		RowLimit:        int64(filter.Limit),
	})
	if err != nil {
		return nil, fmt.Errorf("audit: list event rows: %w", err)
	}
	return rows, nil
}

func (s *Store) CountAuditEventRows(ctx context.Context, filter AuditEventFilter) (int64, error) {
	if filter.StreamTypes == nil {
		filter.StreamTypes = []string{}
	}
	n, err := s.queries.CountAuditEventRows(ctx, generated.CountAuditEventRowsParams{
		ActorID: filter.ActorID, StreamTypesJson: sqlitetype.StringList(filter.StreamTypes), EventType: filter.EventType,
	})
	if err != nil {
		return 0, fmt.Errorf("audit: count event rows: %w", err)
	}
	return n, nil
}

func (s *Store) CountAuditOperations(ctx context.Context, stream string) (int64, error) {
	if stream == "" {
		stream = DefaultAuditStream
	}
	n, err := s.queries.CountAuditOperations(ctx, stream)
	if err != nil {
		return 0, fmt.Errorf("audit: count operations: %w", err)
	}
	return n, nil
}

func (s *Store) GetDevice(ctx context.Context, id string) (DeviceRow, error) {
	row, err := s.queries.GetDevice(ctx, id)
	if err != nil {
		return DeviceRow{}, fmt.Errorf("device: get: %w", translateNotFound(err))
	}
	return row, nil
}

func (s *Store) CountDevices(ctx context.Context) (int64, error) {
	n, err := s.queries.CountDevices(ctx)
	if err != nil {
		return 0, fmt.Errorf("device: count: %w", err)
	}
	return n, nil
}

func (s *Store) CountActions(ctx context.Context) (int64, error) {
	return s.CountAuthoringActions(ctx, ActionListFilter{})
}

func (s *Store) ListAuthoringActions(ctx context.Context, filter ActionListFilter) ([]ActionRow, error) {
	rows, err := s.queries.ListAuthoringActions(ctx, generated.ListAuthoringActionsParams{
		AfterID: filter.AfterID, TypeFilter: filter.Type,
		UnassignedOnly: filter.UnassignedOnly, ScopeRestricted: filter.ScopeRestricted,
		RowLimit: int64(filter.Limit), ScopeGroupIdsJson: sqlitetype.StringList(filter.ScopeGroupIDs),
	})
	if err != nil {
		return nil, fmt.Errorf("action: list: %w", err)
	}
	return rows, nil
}

func (s *Store) CountAuthoringActions(ctx context.Context, filter ActionListFilter) (int64, error) {
	n, err := s.queries.CountAuthoringActions(ctx, generated.CountAuthoringActionsParams{
		TypeFilter: filter.Type, UnassignedOnly: filter.UnassignedOnly,
		ScopeRestricted: filter.ScopeRestricted, ScopeGroupIdsJson: sqlitetype.StringList(filter.ScopeGroupIDs),
	})
	if err != nil {
		return 0, fmt.Errorf("action: count: %w", err)
	}
	return n, nil
}

func (s *Store) ListAuthoringAssignmentTargets(ctx context.Context, sourceType, sourceID string) ([]AssignmentTarget, error) {
	rows, err := s.queries.ListAuthoringAssignmentsForSource(ctx, generated.ListAuthoringAssignmentsForSourceParams{
		SourceType: sourceType, SourceID: sourceID,
	})
	if err != nil {
		return nil, fmt.Errorf("authoring: list assignment targets: %w", err)
	}
	return rows, nil
}

func (s *Store) GetAssignment(ctx context.Context, id string) (AssignmentView, error) {
	row, err := s.queries.GetAssignmentByID(ctx, id)
	if err != nil {
		return AssignmentView{}, fmt.Errorf("assignment: get: %w", translateNotFound(err))
	}
	return AssignmentView{
		ID: row.ID, SourceType: row.SourceType, SourceID: row.SourceID,
		TargetType: row.TargetType, TargetID: row.TargetID, Mode: int32(row.Mode),
		CreatedAt: row.CreatedAt, CreatedBy: row.CreatedBy,
		SourceName: row.ResolvedSourceName, TargetName: row.ResolvedTargetName,
	}, nil
}

func (s *Store) FindAssignment(ctx context.Context, sourceType, sourceID, targetType, targetID string) (AssignmentView, error) {
	row, err := s.queries.GetAssignmentByTuple(ctx, generated.GetAssignmentByTupleParams{
		SourceType: sourceType, SourceID: sourceID, TargetType: targetType, TargetID: targetID,
	})
	if err != nil {
		return AssignmentView{}, fmt.Errorf("assignment: find: %w", translateNotFound(err))
	}
	return AssignmentView{
		ID: row.ID, SourceType: row.SourceType, SourceID: row.SourceID,
		TargetType: row.TargetType, TargetID: row.TargetID, Mode: int32(row.Mode),
		CreatedAt: row.CreatedAt, CreatedBy: row.CreatedBy,
		SourceName: row.SourceName, TargetName: row.TargetName,
	}, nil
}

func (s *Store) ListAssignments(ctx context.Context, filter AssignmentListFilter) ([]AssignmentView, error) {
	if filter.Limit < 0 || filter.Limit > 101 {
		return nil, fmt.Errorf("assignment: list limit must be between 0 and 101")
	}
	if filter.Limit == 0 {
		filter.Limit = 50
	}
	rows, err := s.queries.ListAssignmentViews(ctx, generated.ListAssignmentViewsParams{
		AfterID: filter.AfterID, SourceType: filter.SourceType, SourceID: filter.SourceID,
		TargetType: filter.TargetType, TargetID: filter.TargetID, RowLimit: int64(filter.Limit),
	})
	if err != nil {
		return nil, fmt.Errorf("assignment: list: %w", err)
	}
	views := make([]AssignmentView, len(rows))
	for i, row := range rows {
		views[i] = AssignmentView{
			ID: row.ID, SourceType: row.SourceType, SourceID: row.SourceID,
			TargetType: row.TargetType, TargetID: row.TargetID, Mode: int32(row.Mode),
			CreatedAt: row.CreatedAt, CreatedBy: row.CreatedBy,
			SourceName: row.ResolvedSourceName, TargetName: row.ResolvedTargetName,
		}
	}
	return views, nil
}

func (s *Store) CountAssignments(ctx context.Context, filter AssignmentListFilter) (int64, error) {
	n, err := s.queries.CountAssignmentViews(ctx, generated.CountAssignmentViewsParams{
		SourceType: filter.SourceType, SourceID: filter.SourceID,
		TargetType: filter.TargetType, TargetID: filter.TargetID,
	})
	if err != nil {
		return 0, fmt.Errorf("assignment: count: %w", err)
	}
	return n, nil
}

func (s *Store) ListAssignmentsForUser(ctx context.Context, userID string) ([]AssignmentView, error) {
	rows, err := s.queries.ListAssignmentViewsForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("assignment: list for user: %w", err)
	}
	views := make([]AssignmentView, len(rows))
	for i, row := range rows {
		views[i] = AssignmentView{
			ID: row.ID, SourceType: row.SourceType, SourceID: row.SourceID,
			TargetType: row.TargetType, TargetID: row.TargetID, Mode: int32(row.Mode),
			CreatedAt: row.CreatedAt, CreatedBy: row.CreatedBy,
			SourceName: row.ResolvedSourceName, TargetName: row.ResolvedTargetName,
		}
	}
	return views, nil
}

func (s *Store) ListAvailableSources(ctx context.Context, deviceID string) ([]ResolvedAssignmentSource, error) {
	rows, err := s.ListResolvedSources(ctx, deviceID)
	if err != nil {
		return nil, err
	}

	nonOptional := make(map[string]bool, len(rows))
	for _, row := range rows {
		if row.Mode != 1 {
			nonOptional[row.SourceType+":"+row.SourceID] = true
		}
	}
	available := make([]ResolvedAssignmentSource, 0, len(rows))
	for _, row := range rows {
		if row.Mode == 1 && !nonOptional[row.SourceType+":"+row.SourceID] {
			available = append(available, row)
		}
	}
	return available, nil
}

func (s *Store) ListResolvedSources(ctx context.Context, deviceID string) ([]ResolvedAssignmentSource, error) {
	rows, err := s.queries.ListResolvedAssignmentSourcesForDevice(ctx, deviceID)
	if err != nil {
		return nil, fmt.Errorf("assignment: list resolved sources: %w", err)
	}
	return rows, nil
}

func (s *Store) ListContainingActionSetIDs(ctx context.Context, actionID string) ([]string, error) {
	ids, err := s.queries.ListContainingActionSetIDs(ctx, actionID)
	if err != nil {
		return nil, fmt.Errorf("authoring: list containing action sets: %w", err)
	}
	return ids, nil
}

func (s *Store) ListContainingDefinitionIDs(ctx context.Context, actionSetID string) ([]string, error) {
	ids, err := s.queries.ListContainingDefinitionIDs(ctx, actionSetID)
	if err != nil {
		return nil, fmt.Errorf("authoring: list containing definitions: %w", err)
	}
	return ids, nil
}

func (s *Store) ListCompliancePolicyIDsForAction(ctx context.Context, actionID string) ([]string, error) {
	ids, err := s.queries.ListContainingCompliancePolicyIDs(ctx, actionID)
	if err != nil {
		return nil, fmt.Errorf("compliance policy: list containing policies: %w", err)
	}
	return ids, nil
}

func (s *Store) CountActionSets(ctx context.Context) (int64, error) {
	return s.CountAuthoringActionSets(ctx, ActionSetListFilter{})
}

func (s *Store) ListAuthoringActionSets(ctx context.Context, filter ActionSetListFilter) ([]ActionSetView, error) {
	rows, err := s.queries.ListAuthoringActionSets(ctx, generated.ListAuthoringActionSetsParams{
		AfterID: filter.AfterID, UnassignedOnly: filter.UnassignedOnly,
		ScopeRestricted: filter.ScopeRestricted, RowLimit: int64(filter.Limit),
		ScopeGroupIdsJson: sqlitetype.StringList(filter.ScopeGroupIDs),
	})
	if err != nil {
		return nil, fmt.Errorf("action set: list: %w", err)
	}
	views := make([]ActionSetView, len(rows))
	for i, row := range rows {
		views[i] = ActionSetView{ActionSetRow: ActionSetRow{
			ID: row.ID, Name: row.Name, Description: row.Description,
			Schedule: row.Schedule, OnFailure: row.OnFailure,
			CreatedAt: row.CreatedAt, CreatedBy: row.CreatedBy, UpdatedAt: row.UpdatedAt,
			IsDeleted: row.IsDeleted,
		}, MemberCount: row.MemberCount}
	}
	return views, nil
}

func (s *Store) CountAuthoringActionSets(ctx context.Context, filter ActionSetListFilter) (int64, error) {
	n, err := s.queries.CountAuthoringActionSets(ctx, generated.CountAuthoringActionSetsParams{
		UnassignedOnly: filter.UnassignedOnly, ScopeRestricted: filter.ScopeRestricted,
		ScopeGroupIdsJson: sqlitetype.StringList(filter.ScopeGroupIDs),
	})
	if err != nil {
		return 0, fmt.Errorf("action set: count: %w", err)
	}
	return n, nil
}

func (s *Store) ListActionSetMembers(ctx context.Context, id string) ([]ActionSetMemberView, error) {
	rows, err := s.queries.ListActionSetMembers(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("action set: list members: %w", err)
	}
	return rows, nil
}

func (s *Store) CountDefinitions(ctx context.Context) (int64, error) {
	return s.CountAuthoringDefinitions(ctx, DefinitionListFilter{})
}

func (s *Store) ListAuthoringDefinitions(ctx context.Context, filter DefinitionListFilter) ([]DefinitionView, error) {
	rows, err := s.queries.ListAuthoringDefinitions(ctx, generated.ListAuthoringDefinitionsParams{
		AfterID: filter.AfterID, ScopeRestricted: filter.ScopeRestricted,
		ScopeGroupIdsJson: sqlitetype.StringList(filter.ScopeGroupIDs), RowLimit: int64(filter.Limit),
	})
	if err != nil {
		return nil, fmt.Errorf("definition: list: %w", err)
	}
	views := make([]DefinitionView, len(rows))
	for i, row := range rows {
		views[i] = DefinitionView{DefinitionRow: DefinitionRow{
			ID: row.ID, Name: row.Name, Description: row.Description,
			Schedule: row.Schedule, CreatedAt: row.CreatedAt, CreatedBy: row.CreatedBy,
			UpdatedAt: row.UpdatedAt, IsDeleted: row.IsDeleted,
		}, LiveMemberCount: row.MemberCount}
	}
	return views, nil
}

func (s *Store) CountAuthoringDefinitions(ctx context.Context, filter DefinitionListFilter) (int64, error) {
	n, err := s.queries.CountAuthoringDefinitions(ctx, generated.CountAuthoringDefinitionsParams{
		ScopeRestricted: filter.ScopeRestricted, ScopeGroupIdsJson: sqlitetype.StringList(filter.ScopeGroupIDs),
	})
	if err != nil {
		return 0, fmt.Errorf("definition: count: %w", err)
	}
	return n, nil
}

func (s *Store) ListDefinitionMembers(ctx context.Context, id string) ([]DefinitionMemberView, error) {
	rows, err := s.queries.ListDefinitionMembers(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("definition: list members: %w", err)
	}
	return rows, nil
}

func (s *Store) GetAuthoringCompliancePolicy(ctx context.Context, id string) (CompliancePolicyRow, error) {
	row, err := s.queries.GetAuthoringCompliancePolicy(ctx, id)
	if err != nil {
		return CompliancePolicyRow{}, fmt.Errorf("compliance policy: get: %w", translateNotFound(err))
	}
	return row, nil
}

func (s *Store) ListAuthoringCompliancePolicies(ctx context.Context, filter CompliancePolicyListFilter) ([]CompliancePolicyView, error) {
	rows, err := s.queries.ListAuthoringCompliancePolicies(ctx, generated.ListAuthoringCompliancePoliciesParams{
		AfterID: filter.AfterID, ScopeRestricted: filter.ScopeRestricted,
		ScopeGroupIdsJson: sqlitetype.StringList(filter.ScopeGroupIDs), RowLimit: int64(filter.Limit),
	})
	if err != nil {
		return nil, fmt.Errorf("compliance policy: list: %w", err)
	}
	views := make([]CompliancePolicyView, len(rows))
	for i, row := range rows {
		views[i] = CompliancePolicyView{CompliancePolicyRow: CompliancePolicyRow{
			ID: row.ID, Name: row.Name, Description: row.Description,
			CreatedAt: row.CreatedAt, CreatedBy: row.CreatedBy,
			IsDeleted: row.IsDeleted,
		}, LiveRuleCount: row.RuleCount}
	}
	return views, nil
}

func (s *Store) CountAuthoringCompliancePolicies(ctx context.Context, filter CompliancePolicyListFilter) (int64, error) {
	n, err := s.queries.CountAuthoringCompliancePolicies(ctx, generated.CountAuthoringCompliancePoliciesParams{
		ScopeRestricted: filter.ScopeRestricted, ScopeGroupIdsJson: sqlitetype.StringList(filter.ScopeGroupIDs),
	})
	if err != nil {
		return 0, fmt.Errorf("compliance policy: count: %w", err)
	}
	return n, nil
}

func (s *Store) ListCompliancePolicyRules(ctx context.Context, policyID string) ([]CompliancePolicyRuleView, error) {
	rows, err := s.queries.ListCompliancePolicyRules(ctx, policyID)
	if err != nil {
		return nil, fmt.Errorf("compliance policy: list rules: %w", err)
	}
	return rows, nil
}

func (s *Store) ListDeviceMaintenanceWindows(ctx context.Context, deviceID string) ([][]byte, error) {
	rows, err := s.queries.ListDeviceMaintenanceWindows(ctx, deviceID)
	if err != nil {
		return nil, fmt.Errorf("device: list maintenance windows: %w", err)
	}
	windows := make([][]byte, len(rows))
	for i := range rows {
		windows[i] = rows[i]
	}
	return windows, nil
}

func (s *Store) GetCurrentLuksKeyForAgent(ctx context.Context, deviceID, actionID string) (LuksKeyRow, error) {
	row, err := s.queries.GetCurrentLuksKeyForAgent(ctx, generated.GetCurrentLuksKeyForAgentParams{
		DeviceID: deviceID, ActionID: actionID,
	})
	if err != nil {
		return LuksKeyRow{}, fmt.Errorf("LUKS key: get current for agent: %w", translateNotFound(err))
	}
	return row, nil
}

func (s *Store) GetJob(ctx context.Context, id string) (JobRow, error) {
	row, err := s.queries.GetJob(ctx, id)
	if err != nil {
		return JobRow{}, fmt.Errorf("job: get: %w", translateNotFound(err))
	}
	return row, nil
}

func (s *Store) GetLiveJobByDedupe(ctx context.Context, key string) (JobRow, error) {
	row, err := s.queries.GetLiveJobByDedupe(ctx, &key)
	if err != nil {
		return JobRow{}, fmt.Errorf("job: get live singleton: %w", translateNotFound(err))
	}
	return row, nil
}

func (s *Store) ListClaimableJobs(ctx context.Context, at time.Time, limit int32) ([]JobRow, error) {
	rows, err := s.queries.ListClaimableJobs(ctx, generated.ListClaimableJobsParams{Now: at, PageSize: int64(limit)})
	if err != nil {
		return nil, fmt.Errorf("job: list claimable: %w", err)
	}
	return rows, nil
}

func (s *Store) GetManifestAction(ctx context.Context, id string) (ActionRow, error) {
	row, err := s.queries.GetManifestAction(ctx, id)
	if err != nil {
		return ActionRow{}, fmt.Errorf("manifest: get action: %w", translateNotFound(err))
	}
	return row, nil
}

func (s *Store) GetManifestActionSet(ctx context.Context, id string) (ActionSetRow, error) {
	row, err := s.queries.GetManifestActionSet(ctx, id)
	if err != nil {
		return ActionSetRow{}, fmt.Errorf("manifest: get action set: %w", translateNotFound(err))
	}
	return row, nil
}

func (s *Store) ListManifestActionSetActions(ctx context.Context, id string) ([]ActionRow, error) {
	rows, err := s.queries.ListManifestActionSetActions(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("manifest: list action set actions: %w", err)
	}
	return rows, nil
}

func (s *Store) GetManifestDefinition(ctx context.Context, id string) (DefinitionRow, error) {
	row, err := s.queries.GetManifestDefinition(ctx, id)
	if err != nil {
		return DefinitionRow{}, fmt.Errorf("manifest: get definition: %w", translateNotFound(err))
	}
	return row, nil
}

func (s *Store) ListManifestDefinitionActionSets(ctx context.Context, id string) ([]ActionSetRow, error) {
	rows, err := s.queries.ListManifestDefinitionActionSets(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("manifest: list definition action sets: %w", err)
	}
	return rows, nil
}

func (s *Store) ListManifestDefinitionActions(ctx context.Context, id string) ([]DefinitionManifestAction, error) {
	rows, err := s.queries.ListManifestDefinitionActions(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("manifest: list definition actions: %w", err)
	}
	out := make([]DefinitionManifestAction, 0, len(rows))
	for _, row := range rows {
		out = append(out, DefinitionManifestAction{
			ActionSetID: row.ActionSetID,
			Action: ActionRow{
				ID: row.ID, Name: row.Name, Description: row.Description,
				ActionType: row.ActionType, DesiredState: row.DesiredState,
				Params: row.Params, ParamsCanonical: row.ParamsCanonical,
				TimeoutSeconds: row.TimeoutSeconds, Schedule: row.Schedule,
				IsSystem: row.IsSystem, CreatedAt: row.CreatedAt,
				CreatedBy: row.CreatedBy, UpdatedAt: row.UpdatedAt,
				IsDeleted: row.IsDeleted,
			},
		})
	}
	return out, nil
}

func (s *Store) GetDeviceView(ctx context.Context, id string) (DeviceView, error) {
	row, err := s.GetDevice(ctx, id)
	if err != nil {
		return DeviceView{}, err
	}
	labels, err := s.queries.ListDeviceLabels(ctx, id)
	if err != nil {
		return DeviceView{}, fmt.Errorf("device: list labels: %w", err)
	}
	users, err := s.queries.ListDeviceAssignedUserIDs(ctx, id)
	if err != nil {
		return DeviceView{}, fmt.Errorf("device: list assigned users: %w", err)
	}
	groups, err := s.queries.ListDeviceAssignedGroupIDs(ctx, id)
	if err != nil {
		return DeviceView{}, fmt.Errorf("device: list assigned groups: %w", err)
	}
	view := DeviceView{
		DeviceRow:        row,
		Labels:           make(map[string]string, len(labels)),
		AssignedUserIDs:  users,
		AssignedGroupIDs: groups,
	}
	for _, label := range labels {
		view.Labels[label.Key] = label.Value
	}
	views := []DeviceView{view}
	if err := s.addDeviceFreshness(ctx, []string{id}, views); err != nil {
		return DeviceView{}, err
	}
	view = views[0]
	return view, nil
}

type normalizedDeviceFilter struct {
	afterID         string
	limit           int32
	status          int32
	onlineSince     time.Time
	labels          []byte
	assignedUserID  *string
	scopeRestricted bool
	scopeGroupIDs   []string
}

func (s *Store) normalizeDeviceFilter(filter DeviceListFilter) (normalizedDeviceFilter, error) {

	if filter.Limit < 0 || filter.Limit > 101 {
		return normalizedDeviceFilter{}, fmt.Errorf("device: list limit must be between 0 and 101")
	}
	if filter.Status < DeviceStatusAny || filter.Status > DeviceStatusOffline {
		return normalizedDeviceFilter{}, fmt.Errorf("device: invalid status filter %d", filter.Status)
	}
	limit := filter.Limit
	if limit == 0 {
		limit = 50
	}
	onlineSince := filter.OnlineSince
	if onlineSince.IsZero() {
		onlineSince = s.clock().Add(-5 * time.Minute)
	}
	labels := filter.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	encodedLabels, err := json.Marshal(labels)
	if err != nil {
		return normalizedDeviceFilter{}, fmt.Errorf("device: encode label filter: %w", err)
	}
	return normalizedDeviceFilter{
		afterID: filter.AfterID, limit: limit, status: int32(filter.Status),
		onlineSince: onlineSince, labels: encodedLabels,
		assignedUserID:  filter.AssignedUserID,
		scopeRestricted: filter.ScopeRestricted, scopeGroupIDs: filter.ScopeGroupIDs,
	}, nil
}

func (s *Store) ListDeviceViews(ctx context.Context, filter DeviceListFilter) ([]DeviceView, error) {
	f, err := s.normalizeDeviceFilter(filter)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListDevices(ctx, generated.ListDevicesParams{
		AfterID: f.afterID, AssignedUserID: f.assignedUserID,
		ScopeRestricted: f.scopeRestricted, ScopeGroupIdsJson: sqlitetype.StringList(f.scopeGroupIDs),
		LabelFilter: f.labels, StatusFilter: f.status,
		OnlineSince: &f.onlineSince, RowLimit: int64(f.limit),
	})
	if err != nil {
		return nil, fmt.Errorf("device: list: %w", err)
	}
	if len(rows) == 0 {
		return []DeviceView{}, nil
	}

	ids := make([]string, len(rows))
	views := make([]DeviceView, len(rows))
	byID := make(map[string]int, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
		byID[row.ID] = i
		views[i] = DeviceView{DeviceRow: row, Labels: map[string]string{}}
	}
	labels, err := s.queries.ListDeviceLabelsBatch(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("device: list labels: %w", err)
	}
	for _, label := range labels {
		i := byID[label.DeviceID]
		views[i].Labels[label.Key] = label.Value
	}
	users, err := s.queries.ListDeviceAssignedUserIDsBatch(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("device: list assigned users: %w", err)
	}
	for _, assignment := range users {
		i := byID[assignment.DeviceID]
		views[i].AssignedUserIDs = append(views[i].AssignedUserIDs, assignment.UserID)
	}
	groups, err := s.queries.ListDeviceAssignedGroupIDsBatch(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("device: list assigned groups: %w", err)
	}
	for _, assignment := range groups {
		i := byID[assignment.DeviceID]
		views[i].AssignedGroupIDs = append(views[i].AssignedGroupIDs, assignment.GroupID)
	}
	if err := s.addDeviceFreshness(ctx, ids, views); err != nil {
		return nil, err
	}
	return views, nil
}

func (s *Store) addDeviceFreshness(ctx context.Context, ids []string, views []DeviceView) error {
	latest, err := s.queries.ListLatestInventoryTimesForDevices(ctx, ids)
	if err != nil {
		return fmt.Errorf("device: list inventory freshness: %w", err)
	}
	byID := make(map[string]int, len(views))
	for i := range views {
		byID[views[i].ID] = i
	}
	for _, row := range latest {
		i, ok := byID[row.DeviceID]
		if !ok {
			continue
		}
		collectedAt := row.CollectedAt
		views[i].LastInventoryAt = &collectedAt
	}
	for i := range views {
		views[i].ResolvedInventoryIntervalMinutes = DefaultInventoryIntervalMinutes
		if views[i].InventoryIntervalMinutes != 0 {
			views[i].ResolvedInventoryIntervalMinutes = int32(views[i].InventoryIntervalMinutes)
		}
	}
	intervals, err := s.queries.ListGroupInventoryIntervalsForDevices(ctx, ids)
	if err != nil {
		return fmt.Errorf("device: list group inventory intervals: %w", err)
	}
	for _, row := range intervals {
		i, ok := byID[row.DeviceID]
		if !ok || row.InventoryIntervalMinutes == 0 || views[i].InventoryIntervalMinutes != 0 {
			continue
		}
		interval := int32(row.InventoryIntervalMinutes)
		if interval < views[i].ResolvedInventoryIntervalMinutes {
			views[i].ResolvedInventoryIntervalMinutes = interval
		}
	}
	return nil
}

func (s *Store) CountDeviceViews(ctx context.Context, filter DeviceListFilter) (int64, error) {
	f, err := s.normalizeDeviceFilter(filter)
	if err != nil {
		return 0, err
	}
	n, err := s.queries.CountDeviceViews(ctx, generated.CountDeviceViewsParams{
		AssignedUserID: f.assignedUserID, ScopeRestricted: f.scopeRestricted,
		ScopeGroupIdsJson: sqlitetype.StringList(f.scopeGroupIDs), LabelFilter: f.labels,
		StatusFilter: f.status, OnlineSince: &f.onlineSince,
	})
	if err != nil {
		return 0, fmt.Errorf("device: count filtered: %w", err)
	}
	return n, nil
}

func (s *Store) ListDeviceGroupIDs(ctx context.Context, deviceID string) ([]string, error) {
	if _, err := s.GetDevice(ctx, deviceID); err != nil {
		return nil, err
	}
	ids, err := s.queries.ListDeviceGroupIDs(ctx, deviceID)
	if err != nil {
		return nil, fmt.Errorf("device: list group ids: %w", err)
	}
	return ids, nil
}

func (s *Store) IsDeviceAssignedToUser(ctx context.Context, deviceID, userID string) (bool, error) {
	assigned, err := s.queries.IsDeviceAssignedToUser(ctx, generated.IsDeviceAssignedToUserParams{
		DeviceID: deviceID,
		UserID:   userID,
	})
	if err != nil {
		return false, fmt.Errorf("device: check user assignment: %w", err)
	}
	return assigned, nil
}

func (s *Store) IsDeviceDirectlyAssignedToUser(ctx context.Context, deviceID, userID string) (bool, error) {
	assigned, err := s.queries.IsDeviceDirectlyAssignedToUser(ctx, generated.IsDeviceDirectlyAssignedToUserParams{
		DeviceID: deviceID,
		UserID:   userID,
	})
	if err != nil {
		return false, fmt.Errorf("device: check direct user assignment: %w", err)
	}
	return assigned, nil
}

func (s *Store) ListDeviceAssignees(ctx context.Context, deviceID string) ([]DeviceAssigneeView, error) {
	if _, err := s.GetDevice(ctx, deviceID); err != nil {
		return nil, err
	}
	rows, err := s.queries.ListDeviceAssignees(ctx, deviceID)
	if err != nil {
		return nil, fmt.Errorf("device: list assignees: %w", err)
	}
	out := make([]DeviceAssigneeView, len(rows))
	for i, row := range rows {
		out[i] = DeviceAssigneeView{ID: row.AssigneeID, Kind: row.AssigneeKind, Name: row.AssigneeName}
	}
	return out, nil
}

func (s *Store) ListDeviceInventory(ctx context.Context, deviceID string, tableNames []string) ([]DeviceInventoryTable, error) {
	rows, err := s.queries.ListDeviceInventory(ctx, generated.ListDeviceInventoryParams{
		DeviceID: deviceID, AllTableNames: len(tableNames) == 0, TableNames: tableNames,
	})
	if err != nil {
		return nil, fmt.Errorf("device: list inventory: %w", err)
	}
	return rows, nil
}

func (s *Store) GetOSQueryResult(ctx context.Context, queryID string) (OSQueryResult, error) {
	row, err := s.queries.GetOSQueryResult(ctx, queryID)
	if err != nil {
		return OSQueryResult{}, fmt.Errorf("osquery: get result: %w", translateNotFound(err))
	}
	return row, nil
}

func (s *Store) GetDeviceLogResult(ctx context.Context, queryID string) (DeviceLogResult, error) {
	row, err := s.queries.GetDeviceLogResult(ctx, queryID)
	if err != nil {
		return DeviceLogResult{}, fmt.Errorf("device logs: get result: %w", translateNotFound(err))
	}
	return row, nil
}

func (s *Store) ListDeviceComplianceResults(ctx context.Context, deviceID string) ([]DeviceComplianceResult, error) {
	rows, err := s.queries.ListDeviceComplianceResults(ctx, deviceID)
	if err != nil {
		return nil, fmt.Errorf("compliance: list device results: %w", err)
	}
	return rows, nil
}

func (s *Store) ListDeviceComplianceEvaluations(ctx context.Context, deviceID string) ([]DeviceComplianceEvaluation, error) {
	rows, err := s.queries.ListDeviceComplianceEvaluations(ctx, deviceID)
	if err != nil {
		return nil, fmt.Errorf("compliance: list device evaluations: %w", err)
	}
	return rows, nil
}

func (s *Store) ListDeviceLpsPasswords(ctx context.Context, deviceID string) ([]LpsPasswordView, []LpsPasswordView, error) {
	currentRows, err := s.queries.ListCurrentLpsPasswords(ctx, deviceID)
	if err != nil {
		return nil, nil, fmt.Errorf("lps passwords: list current: %w", err)
	}
	historyRows, err := s.queries.ListLpsPasswordHistory(ctx, deviceID)
	if err != nil {
		return nil, nil, fmt.Errorf("lps passwords: list history: %w", err)
	}
	current := make([]LpsPasswordView, len(currentRows))
	for i, row := range currentRows {
		current[i] = LpsPasswordView{
			ID: row.ID, DeviceID: row.DeviceID, DeviceHostname: row.DeviceHostname,
			ActionID: row.ActionID, ActionName: row.ActionName, Username: row.Username,
			RotatedAt: row.RotatedAt, RotationReason: row.RotationReason,
		}
	}
	history := make([]LpsPasswordView, len(historyRows))
	for i, row := range historyRows {
		history[i] = LpsPasswordView{
			ID: row.ID, DeviceID: row.DeviceID, DeviceHostname: row.DeviceHostname,
			ActionID: row.ActionID, ActionName: row.ActionName, Username: row.Username,
			RotatedAt: row.RotatedAt, RotationReason: row.RotationReason,
		}
	}
	return current, history, nil
}

func (s *Store) GetLpsPasswordForReveal(ctx context.Context, id string) (LpsPasswordSecret, error) {
	row, err := s.queries.GetLpsPasswordForReveal(ctx, id)
	if err != nil {
		return LpsPasswordSecret{}, fmt.Errorf("lps password: get for reveal: %w", translateNotFound(err))
	}
	return row, nil
}

func (s *Store) GetDeviceSecret(ctx context.Context, id string) (DeviceSecretRow, error) {
	return s.queries.GetDeviceSecret(ctx, id)
}

func (s *Store) ListDeviceLuksKeys(ctx context.Context, deviceID string) ([]LuksKeyView, []LuksKeyView, error) {
	currentRows, err := s.queries.ListCurrentLuksKeys(ctx, deviceID)
	if err != nil {
		return nil, nil, fmt.Errorf("luks keys: list current: %w", err)
	}
	historyRows, err := s.queries.ListLuksKeyHistory(ctx, deviceID)
	if err != nil {
		return nil, nil, fmt.Errorf("luks keys: list history: %w", err)
	}
	current := make([]LuksKeyView, len(currentRows))
	for i, row := range currentRows {
		current[i] = LuksKeyView{
			ID: row.ID, DeviceID: row.DeviceID, DeviceHostname: row.DeviceHostname,
			ActionID: row.ActionID, ActionName: row.ActionName, DevicePath: row.DevicePath,
			RotatedAt: row.RotatedAt, RotationReason: row.RotationReason,
			RevocationStatus: row.RevocationStatus, RevocationError: row.RevocationError,
			RevocationAt: row.RevocationAt,
		}
	}
	history := make([]LuksKeyView, len(historyRows))
	for i, row := range historyRows {
		history[i] = LuksKeyView{
			ID: row.ID, DeviceID: row.DeviceID, DeviceHostname: row.DeviceHostname,
			ActionID: row.ActionID, ActionName: row.ActionName, DevicePath: row.DevicePath,
			RotatedAt: row.RotatedAt, RotationReason: row.RotationReason,
			RevocationStatus: row.RevocationStatus, RevocationError: row.RevocationError,
			RevocationAt: row.RevocationAt,
		}
	}
	return current, history, nil
}

func (s *Store) GetLuksKeyForReveal(ctx context.Context, id string) (LuksKeySecret, error) {
	row, err := s.queries.GetLuksKeyForReveal(ctx, id)
	if err != nil {
		return LuksKeySecret{}, fmt.Errorf("luks key: get for reveal: %w", translateNotFound(err))
	}
	return row, nil
}

func (s *Store) GetLuksRevocationTarget(ctx context.Context, deviceID, actionID string) (LuksRevocationTarget, error) {
	row, err := s.queries.GetLuksRevocationTarget(ctx, generated.GetLuksRevocationTargetParams{
		DeviceID: deviceID, ActionID: actionID,
	})
	if err != nil {
		return LuksRevocationTarget{}, fmt.Errorf("luks revocation target: %w", err)
	}
	return row, nil
}

func (s *Store) GetOpenTerminalSession(ctx context.Context, sessionID string) (OpenTerminalSession, error) {
	row, err := s.queries.GetOpenTerminalSession(ctx, sessionID)
	if err != nil {
		return OpenTerminalSession{}, fmt.Errorf("terminal session: get open: %w", translateNotFound(err))
	}
	return row, nil
}

func (s *Store) GetUser(ctx context.Context, id string) (UserRow, error) {
	row, err := s.queries.GetUser(ctx, id)
	if err != nil {
		return UserRow{}, fmt.Errorf("user: get: %w", translateNotFound(err))
	}
	return row, nil
}

func (s *Store) CountUsers(ctx context.Context) (int64, error) {
	n, err := s.queries.CountUsers(ctx)
	if err != nil {
		return 0, fmt.Errorf("user: count: %w", err)
	}
	return n, nil
}

func (s *Store) GetUserEncryptionKey(ctx context.Context, userID string) (generated.UserEncryptionKey, error) {
	row, err := s.queries.GetUserEncryptionKey(ctx, userID)
	if err != nil {
		return generated.UserEncryptionKey{}, fmt.Errorf("user_encryption_key: get: %w", translateNotFound(err))
	}
	return row, nil
}
