package assignment

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"

	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/types/known/timestamppb"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/authoring"
	"github.com/manchtools/cadestro/server/internal/middleware"
	"github.com/manchtools/cadestro/server/internal/store"
)

const defaultPageSize = int32(50)

// Handlers implements explicit assignment CRUD.
type Handlers struct {
	store  *store.Store
	state  *State
	logger *slog.Logger
}

// New constructs direct assignment handlers.
func New(cfg Config) *Handlers {
	if cfg.Store == nil {
		panic("assignment: handler store is required")
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Handlers{
		store: cfg.Store, state: NewState(cfg), logger: logger,
	}
}

func (h *Handlers) actor(ctx context.Context) (*auth.UserContext, error) {
	actor, ok := auth.UserFromContext(ctx)
	if !ok {
		return nil, rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_NOT_AUTHENTICATED, connect.CodeUnauthenticated, "not authenticated")
	}
	return actor, nil
}

func (h *Handlers) authorize(ctx context.Context, permission, resourceID string) error {
	if !auth.AuthorizeContext(ctx, permission, resourceID) {
		return rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, connect.CodePermissionDenied, "permission denied")
	}
	return nil
}

func (h *Handlers) operation(req connect.AnyRequest, actor *auth.UserContext, procedure, permission string) store.AuditOperation {
	op := store.AuditOperation{
		Class: store.ClassMutation, ActorType: string(actor.Kind), Origin: auth.ControlRPCOrigin,
		RequestDescriptor: procedure, AuthorizationOutcome: store.AuthorizationAllowed,
		AuthorizationDetail: permission, Result: store.ResultSuccess, ResultCode: "OK",
	}
	if actor.CanOwnResources() {
		op.ActorID = actor.ID
	}
	if ip := auth.ClientIP(req); ip != "" {
		op.OriginFingerprint = auth.Fingerprint(ip)
	}
	return op
}

// CreateAssignment creates or idempotently returns one source-target edge.
func (h *Handlers) CreateAssignment(ctx context.Context, req *connect.Request[cadestrov1.CreateAssignmentRequest]) (*connect.Response[cadestrov1.CreateAssignmentResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "CreateAssignment", ""); err != nil {
		return nil, err
	}
	row, err := h.state.Create(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceCreateAssignmentProcedure, "CreateAssignment"), CreateParams{
		SourceType: req.Msg.SourceType, SourceID: req.Msg.SourceId,
		TargetType: req.Msg.TargetType, TargetID: req.Msg.TargetId,
		Mode: req.Msg.Mode, CreatedBy: actor.ID,
	})
	if err != nil {
		return nil, h.mapError(ctx, "create assignment", err)
	}
	return connect.NewResponse(&cadestrov1.CreateAssignmentResponse{Assignment: assignmentToProto(row)}), nil
}

// DeleteAssignment soft-deletes one assignment edge.
func (h *Handlers) DeleteAssignment(ctx context.Context, req *connect.Request[cadestrov1.DeleteAssignmentRequest]) (*connect.Response[cadestrov1.DeleteAssignmentResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "DeleteAssignment", req.Msg.Id); err != nil {
		return nil, err
	}
	if err := h.state.Delete(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceDeleteAssignmentProcedure, "DeleteAssignment"), req.Msg.Id); err != nil {
		return nil, h.mapError(ctx, "delete assignment", err)
	}
	return connect.NewResponse(&cadestrov1.DeleteAssignmentResponse{}), nil
}

// ListAssignments returns a deterministic keyset page.
func (h *Handlers) ListAssignments(ctx context.Context, req *connect.Request[cadestrov1.ListAssignmentsRequest]) (*connect.Response[cadestrov1.ListAssignmentsResponse], error) {
	if _, err := h.actor(ctx); err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "ListAssignments", ""); err != nil {
		return nil, err
	}
	if req.Msg.PageToken != "" {
		if _, err := ulid.ParseStrict(req.Msg.PageToken); err != nil {
			return nil, rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_INVALID_PAGE_TOKEN, connect.CodeInvalidArgument, "invalid page token")
		}
	}
	sourceType, _ := sourceTypeName(req.Msg.SourceType)
	targetType, _ := targetTypeName(req.Msg.TargetType)
	limit := req.Msg.PageSize
	if limit == 0 {
		limit = defaultPageSize
	}
	filter := store.AssignmentListFilter{
		AfterID: req.Msg.PageToken, Limit: limit + 1,
		SourceType: sourceType, SourceID: req.Msg.SourceId,
		TargetType: targetType, TargetID: req.Msg.TargetId,
	}
	rows, err := h.store.ListAssignments(ctx, filter)
	if err != nil {
		return nil, h.internal(ctx, "list assignments", err)
	}
	hasMore := len(rows) > int(limit)
	if hasMore {
		rows = rows[:limit]
	}
	count, err := h.store.CountAssignments(ctx, filter)
	if err != nil {
		return nil, h.internal(ctx, "count assignments", err)
	}
	out := make([]*cadestrov1.Assignment, len(rows))
	for i, row := range rows {
		out[i] = assignmentToProto(row)
	}
	next := ""
	if hasMore {
		next = rows[len(rows)-1].ID
	}
	return connect.NewResponse(&cadestrov1.ListAssignmentsResponse{
		Assignments: out, NextPageToken: next, TotalCount: boundedCount(count),
	}), nil
}

// GetUserAssignments resolves direct and current user-group targets.
func (h *Handlers) GetUserAssignments(ctx context.Context, req *connect.Request[cadestrov1.GetUserAssignmentsRequest]) (*connect.Response[cadestrov1.GetUserAssignmentsResponse], error) {
	if _, err := h.actor(ctx); err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "GetUserAssignments", ""); err != nil {
		return nil, err
	}
	rows, err := h.store.ListAssignmentsForUser(ctx, req.Msg.UserId)
	if err != nil {
		return nil, h.internal(ctx, "get user assignments", err)
	}
	out := make([]*cadestrov1.Assignment, len(rows))
	for i, row := range rows {
		out[i] = assignmentToProto(row)
	}
	return connect.NewResponse(&cadestrov1.GetUserAssignmentsResponse{Assignments: out}), nil
}

// GetDeviceAssignments expands every effective live source that reaches a
// device. EXCLUDED suppresses a source, UNINSTALL forces its resolved actions
// absent, and AVAILABLE contributes only when selected.
func (h *Handlers) GetDeviceAssignments(ctx context.Context, req *connect.Request[cadestrov1.GetDeviceAssignmentsRequest]) (*connect.Response[cadestrov1.GetDeviceAssignmentsResponse], error) {
	deviceID := req.Msg.GetDeviceId().GetValue()
	if _, err := h.actor(ctx); err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "GetDeviceAssignments", ""); err != nil {
		return nil, err
	}
	if _, err := h.store.GetDevice(ctx, deviceID); err != nil {
		if store.IsNotFound(err) {
			return nil, rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_DEVICE_NOT_FOUND, connect.CodeNotFound, "device not found")
		}
		return nil, h.internal(ctx, "read assignment device", err)
	}
	paths, err := h.store.ListResolvedSources(ctx, deviceID)
	if err != nil {
		return nil, h.internal(ctx, "resolve device assignments", err)
	}
	sources, err := EffectiveSources(paths)
	if err != nil {
		return nil, h.internal(ctx, "resolve assignment modes", err)
	}

	response := &cadestrov1.GetDeviceAssignmentsResponse{}
	actionIndex := make(map[string]int)
	for _, source := range sources {
		sourceType, ok := sourceTypeValue(source.Row.SourceType)
		if !ok {
			return nil, h.internal(ctx, "decode assigned source", ErrInvalidInput)
		}
		actions, err := h.previewActions(ctx, sourceType, source.Row.SourceID)
		if err != nil {
			return nil, h.internal(ctx, "expand assigned actions", err)
		}
		appendResolvedActions(&response.Actions, actionIndex, actions, source.ForceAbsent)

		switch sourceType {
		case cadestrov1.AssignmentSourceType_ASSIGNMENT_SOURCE_TYPE_ACTION:
			// The expanded action above is the complete response for a
			// singleton source.
		case cadestrov1.AssignmentSourceType_ASSIGNMENT_SOURCE_TYPE_ACTION_SET:
			row, err := h.store.GetManifestActionSet(ctx, source.Row.SourceID)
			if err != nil {
				return nil, h.internal(ctx, "read assigned action set", err)
			}
			members, err := h.store.ListActionSetMembers(ctx, row.ID)
			if err != nil {
				return nil, h.internal(ctx, "read assigned action set members", err)
			}
			set, err := authoring.ActionSetToProto(row, int64(len(members)))
			if err != nil {
				return nil, h.internal(ctx, "encode assigned action set", err)
			}
			response.ActionSets = append(response.ActionSets, set)
			response.ActionSetDetails = append(response.ActionSetDetails, &cadestrov1.GetActionSetResponse{
				Set: set, Members: authoring.ActionSetMembersToProto(members),
			})
		case cadestrov1.AssignmentSourceType_ASSIGNMENT_SOURCE_TYPE_DEFINITION:
			row, err := h.store.GetManifestDefinition(ctx, source.Row.SourceID)
			if err != nil {
				return nil, h.internal(ctx, "read assigned definition", err)
			}
			members, err := h.store.ListDefinitionMembers(ctx, row.ID)
			if err != nil {
				return nil, h.internal(ctx, "read assigned definition members", err)
			}
			definition, err := authoring.DefinitionToProto(row, int64(len(members)))
			if err != nil {
				return nil, h.internal(ctx, "encode assigned definition", err)
			}
			response.Definitions = append(response.Definitions, definition)
			response.DefinitionDetails = append(response.DefinitionDetails, &cadestrov1.GetDefinitionResponse{
				Definition: definition, Members: authoring.DefinitionMembersToProto(members),
			})
		case cadestrov1.AssignmentSourceType_ASSIGNMENT_SOURCE_TYPE_COMPLIANCE_POLICY:
			row, err := h.store.GetAuthoringCompliancePolicy(ctx, source.Row.SourceID)
			if err != nil {
				return nil, h.internal(ctx, "read assigned compliance policy", err)
			}
			rules, err := h.store.ListCompliancePolicyRules(ctx, row.ID)
			if err != nil {
				return nil, h.internal(ctx, "read assigned compliance rules", err)
			}
			response.CompliancePolicies = append(response.CompliancePolicies, compliancePolicyToProto(row, rules))
		}
	}
	return connect.NewResponse(response), nil
}

// ResolvedSource is one source after all target paths and modes have been
// collapsed. Excluded remains explicit because it suppresses lower authoring
// layers even though it produces no manifest itself.
type ResolvedSource struct {
	Row         store.ResolvedAssignmentSource
	Active      bool
	Excluded    bool
	ForceAbsent bool
}

// ResolveSources collapses assignment modes without discarding exclusions.
func ResolveSources(paths []store.ResolvedAssignmentSource) ([]ResolvedSource, error) {
	type decision struct {
		row                           store.ResolvedAssignmentSource
		required, selected, uninstall bool
		excluded                      bool
	}
	order := make([]string, 0, len(paths))
	bySource := make(map[string]*decision, len(paths))
	for _, path := range paths {
		key := path.SourceType + ":" + path.SourceID
		current := bySource[key]
		if current == nil {
			current = &decision{row: path}
			bySource[key] = current
			order = append(order, key)
		}
		switch cadestrov1.AssignmentMode(path.Mode) {
		case cadestrov1.AssignmentMode_ASSIGNMENT_MODE_REQUIRED:
			current.required = true
		case cadestrov1.AssignmentMode_ASSIGNMENT_MODE_AVAILABLE:
			current.selected = current.selected || path.Selected
		case cadestrov1.AssignmentMode_ASSIGNMENT_MODE_EXCLUDED:
			current.excluded = true
		case cadestrov1.AssignmentMode_ASSIGNMENT_MODE_UNINSTALL:
			current.uninstall = true
		default:
			return nil, ErrInvalidInput
		}
	}
	resolved := make([]ResolvedSource, 0, len(order))
	for _, key := range order {
		decision := bySource[key]
		resolved = append(resolved, ResolvedSource{
			Row: decision.row, Active: !decision.excluded &&
				(decision.required || decision.selected || decision.uninstall),
			Excluded: decision.excluded, ForceAbsent: decision.uninstall && !decision.excluded,
		})
	}
	return resolved, nil
}

// EffectiveSources returns only sources that currently contribute actions.
func EffectiveSources(paths []store.ResolvedAssignmentSource) ([]ResolvedSource, error) {
	resolved, err := ResolveSources(paths)
	if err != nil {
		return nil, err
	}
	effective := make([]ResolvedSource, 0, len(resolved))
	for _, source := range resolved {
		if source.Active {
			effective = append(effective, source)
		}
	}
	return effective, nil
}

func appendResolvedActions(dst *[]*cadestrov1.ManagedAction, index map[string]int, actions []*cadestrov1.ManagedAction, forceAbsent bool) {
	for _, action := range actions {
		if existing, ok := index[action.Id]; ok {
			if forceAbsent {
				(*dst)[existing].DesiredState = cadestrov1.DesiredState_DESIRED_STATE_ABSENT
			}
			continue
		}
		if forceAbsent {
			action.DesiredState = cadestrov1.DesiredState_DESIRED_STATE_ABSENT
		}
		index[action.Id] = len(*dst)
		*dst = append(*dst, action)
	}
}

func compliancePolicyToProto(row store.CompliancePolicyRow, rules []store.CompliancePolicyRuleView) *cadestrov1.CompliancePolicy {
	policy := &cadestrov1.CompliancePolicy{
		Id: row.ID, Name: row.Name, Description: row.Description,
		RuleCount: boundedCount(int64(len(rules))), CreatedBy: row.CreatedBy,
		Rules: make([]*cadestrov1.CompliancePolicyRule, len(rules)),
	}
	if row.CreatedAt != nil {
		policy.CreatedAt = timestamppb.New(*row.CreatedAt)
	}
	for i, rule := range rules {
		policy.Rules[i] = &cadestrov1.CompliancePolicyRule{
			ActionId: &cadestrov1.ActionId{Value: rule.ActionID}, ActionName: rule.ActionName,
			GracePeriodHours: rule.GracePeriodHours,
		}
	}
	return policy
}

// SetUserSelection persists one optional source choice for an accessible
// device through the audited mutation primitive.
func (h *Handlers) SetUserSelection(ctx context.Context, req *connect.Request[cadestrov1.SetUserSelectionRequest]) (*connect.Response[cadestrov1.SetUserSelectionResponse], error) {
	deviceID := req.Msg.GetDeviceId().GetValue()
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "SetUserSelection", ""); err != nil {
		return nil, err
	}
	if err := h.requireDeviceAccess(ctx, actor, deviceID); err != nil {
		return nil, err
	}
	row, err := h.state.SetUserSelection(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceSetUserSelectionProcedure, "SetUserSelection"),
		deviceID, req.Msg.SourceType, req.Msg.SourceId, req.Msg.Selected, actor.ID)
	if err != nil {
		return nil, h.mapError(ctx, "set user selection", err)
	}
	return connect.NewResponse(&cadestrov1.SetUserSelectionResponse{Selection: userSelectionToProto(row)}), nil
}

// ListAvailableActions returns each live AVAILABLE source once with its
// current device selection and a complete action preview.
func (h *Handlers) ListAvailableActions(ctx context.Context, req *connect.Request[cadestrov1.ListAvailableActionsRequest]) (*connect.Response[cadestrov1.ListAvailableActionsResponse], error) {
	deviceID := req.Msg.GetDeviceId().GetValue()
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "ListAvailableActions", ""); err != nil {
		return nil, err
	}
	if err := h.requireDeviceAccess(ctx, actor, deviceID); err != nil {
		return nil, err
	}
	rows, err := h.store.ListAvailableSources(ctx, deviceID)
	if err != nil {
		return nil, h.internal(ctx, "list available actions", err)
	}
	items := make([]*cadestrov1.AvailableItem, len(rows))
	for i, row := range rows {
		sourceType, ok := sourceTypeValue(row.SourceType)
		if !ok {
			return nil, h.internal(ctx, "decode available source", ErrInvalidInput)
		}
		actions, err := h.previewActions(ctx, sourceType, row.SourceID)
		if err != nil {
			return nil, h.internal(ctx, "build available preview", err)
		}
		items[i] = &cadestrov1.AvailableItem{
			SourceType: sourceType, SourceId: row.SourceID,
			SourceName: row.SourceName, SourceDescription: row.SourceDescription,
			Selected: row.Selected, Actions: actions,
		}
	}
	return connect.NewResponse(&cadestrov1.ListAvailableActionsResponse{Items: items}), nil
}

type assignmentScopeResolver struct{ store *store.Store }

func (r assignmentScopeResolver) DeviceGroupsForDevice(ctx context.Context, deviceID string) ([]string, error) {
	return r.store.ListDeviceGroupIDs(ctx, deviceID)
}

func (r assignmentScopeResolver) UserGroupsForUser(ctx context.Context, userID string) ([]string, error) {
	return r.store.ListUserGroupIDsForUser(ctx, userID)
}

func (h *Handlers) requireDeviceAccess(ctx context.Context, actor *auth.UserContext, deviceID string) error {
	if !auth.AuthorizeContext(ctx, "ListDevices", deviceID) {
		return rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_DEVICE_NOT_FOUND, connect.CodeNotFound, "device not found")
	}
	if _, err := h.store.GetDevice(ctx, deviceID); err != nil {
		if store.IsNotFound(err) {
			return rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_DEVICE_NOT_FOUND, connect.CodeNotFound, "device not found")
		}
		return h.internal(ctx, "read selection device", err)
	}
	if auth.HasPermission(ctx, "ListDevices") {
		if err := auth.EnforceDeviceScopeOnBaseTier(ctx, assignmentScopeResolver{store: h.store}, "ListDevices", deviceID); err != nil {
			if connect.CodeOf(err) == connect.CodeInternal {
				return h.internal(ctx, "resolve selection device scope", err)
			}
			return rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_DEVICE_NOT_FOUND, connect.CodeNotFound, "device not found")
		}
		return nil
	}
	assigned, err := h.store.IsDeviceAssignedToUser(ctx, deviceID, actor.ID)
	if err != nil {
		return h.internal(ctx, "check selection device assignment", err)
	}
	if !assigned {
		return rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_DEVICE_NOT_FOUND, connect.CodeNotFound, "device not found")
	}
	return nil
}

func (h *Handlers) previewActions(ctx context.Context, sourceType cadestrov1.AssignmentSourceType, sourceID string) ([]*cadestrov1.ManagedAction, error) {
	var rows []store.ActionRow
	switch sourceType {
	case cadestrov1.AssignmentSourceType_ASSIGNMENT_SOURCE_TYPE_ACTION:
		row, err := h.store.GetManifestAction(ctx, sourceID)
		if err != nil {
			return nil, err
		}
		rows = []store.ActionRow{row}
	case cadestrov1.AssignmentSourceType_ASSIGNMENT_SOURCE_TYPE_ACTION_SET:
		var err error
		rows, err = h.store.ListManifestActionSetActions(ctx, sourceID)
		if err != nil {
			return nil, err
		}
	case cadestrov1.AssignmentSourceType_ASSIGNMENT_SOURCE_TYPE_DEFINITION:
		definitionRows, err := h.store.ListManifestDefinitionActions(ctx, sourceID)
		if err != nil {
			return nil, err
		}
		rows = make([]store.ActionRow, len(definitionRows))
		for i, row := range definitionRows {
			rows[i] = row.Action
		}
	case cadestrov1.AssignmentSourceType_ASSIGNMENT_SOURCE_TYPE_COMPLIANCE_POLICY:
		rules, err := h.store.ListCompliancePolicyRules(ctx, sourceID)
		if err != nil {
			return nil, err
		}
		rows = make([]store.ActionRow, len(rules))
		for i, rule := range rules {
			rows[i], err = h.store.GetManifestAction(ctx, rule.ActionID)
			if err != nil {
				return nil, err
			}
		}
	default:
		return nil, ErrInvalidInput
	}

	actions := make([]*cadestrov1.ManagedAction, len(rows))
	for i, row := range rows {
		var err error
		actions[i], err = authoring.ActionToProto(row)
		if err != nil {
			return nil, err
		}
	}
	return actions, nil
}

func userSelectionToProto(row store.UserSelectionRow) *cadestrov1.UserSelection {
	value, _ := sourceTypeValue(row.SourceType)
	return &cadestrov1.UserSelection{
		Id: row.ID, DeviceId: &cadestrov1.DeviceId{Value: row.DeviceID}, SourceType: value,
		SourceId: row.SourceID, Selected: row.Selected,
		UpdatedAt: timestamppb.New(row.UpdatedAt),
	}
}

func (h *Handlers) mapError(ctx context.Context, operation string, err error) error {
	switch {
	case errors.Is(err, ErrInvalidInput):
		return rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_VALIDATION_FAILED, connect.CodeInvalidArgument, "invalid assignment")
	case errors.Is(err, ErrSourceNotFound):
		return rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_ASSIGNMENT_SOURCE_NOT_FOUND, connect.CodeNotFound, "assignment source not found")
	case errors.Is(err, ErrTargetNotFound):
		return rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_ASSIGNMENT_TARGET_NOT_FOUND, connect.CodeNotFound, "assignment target not found")
	case errors.Is(err, ErrNotFound), store.IsNotFound(err):
		return rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_ASSIGNMENT_NOT_FOUND, connect.CodeNotFound, "assignment not found")
	case errors.Is(err, ErrSystemAction):
		return rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_CANNOT_MODIFY_SYSTEM_ACTION, connect.CodeFailedPrecondition, "system action cannot be assigned directly")
	case errors.Is(err, ErrNoAvailableAssignment):
		return rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_NO_ASSIGNMENT_FOUND, connect.CodeNotFound, "no available assignment found")
	default:
		return h.internal(ctx, operation, err)
	}
}

func (h *Handlers) internal(ctx context.Context, operation string, err error) *connect.Error {
	h.logger.Error("assignment RPC failed", "operation", operation, "error", err)
	return rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_INTERNAL_ERROR, connect.CodeInternal, "internal error")
}

func assignmentToProto(row store.AssignmentView) *cadestrov1.Assignment {
	sourceType, _ := sourceTypeValue(row.SourceType)
	targetType, _ := targetTypeValue(row.TargetType)
	out := &cadestrov1.Assignment{
		Id: row.ID, SourceType: sourceType, SourceId: row.SourceID,
		TargetType: targetType, TargetId: row.TargetID, Mode: cadestrov1.AssignmentMode(row.Mode),
		CreatedBy: row.CreatedBy, SourceName: row.SourceName, TargetName: row.TargetName,
	}
	if row.CreatedAt != nil {
		out.CreatedAt = timestamppb.New(*row.CreatedAt)
	}
	return out
}

func sourceTypeValue(value string) (cadestrov1.AssignmentSourceType, bool) {
	switch value {
	case "action":
		return cadestrov1.AssignmentSourceType_ASSIGNMENT_SOURCE_TYPE_ACTION, true
	case "action_set":
		return cadestrov1.AssignmentSourceType_ASSIGNMENT_SOURCE_TYPE_ACTION_SET, true
	case "definition":
		return cadestrov1.AssignmentSourceType_ASSIGNMENT_SOURCE_TYPE_DEFINITION, true
	case "compliance_policy":
		return cadestrov1.AssignmentSourceType_ASSIGNMENT_SOURCE_TYPE_COMPLIANCE_POLICY, true
	default:
		return cadestrov1.AssignmentSourceType_ASSIGNMENT_SOURCE_TYPE_UNSPECIFIED, false
	}
}

func targetTypeValue(value string) (cadestrov1.AssignmentTargetType, bool) {
	switch value {
	case "device":
		return cadestrov1.AssignmentTargetType_ASSIGNMENT_TARGET_TYPE_DEVICE, true
	case "device_group":
		return cadestrov1.AssignmentTargetType_ASSIGNMENT_TARGET_TYPE_DEVICE_GROUP, true
	case "user":
		return cadestrov1.AssignmentTargetType_ASSIGNMENT_TARGET_TYPE_USER, true
	case "user_group":
		return cadestrov1.AssignmentTargetType_ASSIGNMENT_TARGET_TYPE_USER_GROUP, true
	default:
		return cadestrov1.AssignmentTargetType_ASSIGNMENT_TARGET_TYPE_UNSPECIFIED, false
	}
}

func boundedCount(value int64) int32 {
	if value > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(value)
}

func rpcError(ctx context.Context, code cadestrov1.ErrorCode, connectCode connect.Code, message string) *connect.Error {
	err := connect.NewError(connectCode, errors.New(message))
	detail, detailErr := connect.NewErrorDetail(&cadestrov1.ErrorDetail{
		Code: code, RequestId: middleware.RequestIDFromContext(ctx),
	})
	if detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

// Mount registers exactly the implemented assignment CRUD procedures.
func (h *Handlers) Mount(mux *http.ServeMux, opts ...connect.HandlerOption) []string {
	if mux == nil {
		panic("assignment: mux is required")
	}
	mounted := make([]string, 0, 7)
	register := func(procedure string, handler http.Handler) {
		mux.Handle(procedure, handler)
		mounted = append(mounted, procedure)
	}
	register(cadestrov1connect.ControlServiceCreateAssignmentProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceCreateAssignmentProcedure, h.CreateAssignment, opts...))
	register(cadestrov1connect.ControlServiceDeleteAssignmentProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceDeleteAssignmentProcedure, h.DeleteAssignment, opts...))
	register(cadestrov1connect.ControlServiceListAssignmentsProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceListAssignmentsProcedure, h.ListAssignments, opts...))
	register(cadestrov1connect.ControlServiceGetUserAssignmentsProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceGetUserAssignmentsProcedure, h.GetUserAssignments, opts...))
	register(cadestrov1connect.ControlServiceSetUserSelectionProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceSetUserSelectionProcedure, h.SetUserSelection, opts...))
	register(cadestrov1connect.ControlServiceListAvailableActionsProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceListAvailableActionsProcedure, h.ListAvailableActions, opts...))
	register(cadestrov1connect.ControlServiceGetDeviceAssignmentsProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceGetDeviceAssignmentsProcedure, h.GetDeviceAssignments, opts...))
	return mounted
}

// MutationProcedures is the exact audited assignment mutation surface.
func MutationProcedures() []string {
	return []string{
		cadestrov1connect.ControlServiceCreateAssignmentProcedure,
		cadestrov1connect.ControlServiceDeleteAssignmentProcedure,
		cadestrov1connect.ControlServiceSetUserSelectionProcedure,
	}
}

// ReadProcedures is the exact non-mutating assignment CRUD surface.
func ReadProcedures() []string {
	return []string{
		cadestrov1connect.ControlServiceListAssignmentsProcedure,
		cadestrov1connect.ControlServiceGetUserAssignmentsProcedure,
		cadestrov1connect.ControlServiceListAvailableActionsProcedure,
		cadestrov1connect.ControlServiceGetDeviceAssignmentsProcedure,
	}
}
