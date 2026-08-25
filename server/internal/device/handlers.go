package device

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"time"

	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/connection"
	"github.com/manchtools/cadestro/server/internal/crypto"
	"github.com/manchtools/cadestro/server/internal/store"
	"github.com/manchtools/cadestro/server/internal/terminal"
)

const (
	defaultPageSize          = int32(50)
	maxLabelFilters          = 64
	maxInventoryTableFilters = 128
	resultTimeout            = 5 * time.Minute
)

// Config supplies the direct SQLite store and process-local seams used by
// the device handlers.
type Config struct {
	Store            *store.Store
	Logger           *slog.Logger
	Now              func() time.Time
	CloseStream      func(deviceID string)
	AgentSender      AgentSender
	Decryptor        *crypto.Encryptor
	TerminalTokens   *terminal.TokenStore
	TerminalSessions *connection.TerminalSessionRegistry
	TerminalURL      string
	IsConnected      func(deviceID string) bool
}

// AgentSender is the only outbound transport capability an instant device
// operation needs. The connection manager satisfies it directly.
type AgentSender interface {
	Send(deviceID string, message *cadestrov1.ServerMessage) error
}

// Handlers implements the device CRUD procedures.
type Handlers struct {
	store            *store.Store
	logger           *slog.Logger
	now              func() time.Time
	closeStream      func(string)
	agentSender      AgentSender
	decryptor        *crypto.Encryptor
	terminalTokens   *terminal.TokenStore
	terminalSessions *connection.TerminalSessionRegistry
	terminalURL      string
	isConnected      func(string) bool
}

// New constructs the device handlers. A missing store is a boot-time wiring
// defect and is rejected immediately.
func New(cfg Config) *Handlers {
	if cfg.Store == nil {
		panic("device: store is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.CloseStream == nil {
		panic("device: stream closer is required")
	}
	if cfg.AgentSender == nil {
		panic("device: agent sender is required")
	}
	if cfg.Decryptor == nil {
		panic("device: secret decryptor is required")
	}
	if cfg.TerminalTokens == nil || cfg.TerminalSessions == nil || cfg.IsConnected == nil {
		panic("device: terminal transport is required")
	}
	terminalURL := normalizeTerminalURL(cfg.TerminalURL)
	if terminalURL == "" {
		panic("device: secure terminal URL is required")
	}
	return &Handlers{
		store: cfg.Store, logger: cfg.Logger, now: cfg.Now,
		closeStream: cfg.CloseStream, agentSender: cfg.AgentSender, decryptor: cfg.Decryptor,
		terminalTokens: cfg.TerminalTokens, terminalSessions: cfg.TerminalSessions,
		terminalURL: terminalURL, isConnected: cfg.IsConnected,
	}
}

func (h *Handlers) actor(ctx context.Context) (*auth.UserContext, error) {
	actor, ok := auth.UserFromContext(ctx)
	if !ok {
		return nil, rpcError(ctx, errNotAuthenticated, connect.CodeUnauthenticated, "not authenticated")
	}
	return actor, nil
}

func (h *Handlers) authorize(ctx context.Context, permission, resourceID string) error {
	if !auth.AuthorizeContext(ctx, permission, resourceID) {
		return rpcError(ctx, errPermissionDenied, connect.CodePermissionDenied, "permission denied")
	}
	return nil
}

func (h *Handlers) internal(ctx context.Context, operation string, err error) *connect.Error {
	h.logger.Error("device RPC failed", "operation", operation, "error", err)
	return rpcError(ctx, errInternal, connect.CodeInternal, "internal error")
}

type scopeResolver struct{ store *store.Store }

func (r scopeResolver) DeviceGroupsForDevice(ctx context.Context, deviceID string) ([]string, error) {
	ids, err := r.store.ListDeviceGroupIDs(ctx, deviceID)
	if store.IsNotFound(err) {
		// Scope checks run before existence lookups. Unknown and out-of-scope
		// identifiers must therefore both reduce to "not in an allowed group".
		return nil, nil
	}
	return ids, err
}

func (scopeResolver) UserGroupsForUser(context.Context, string) ([]string, error) {
	return nil, fmt.Errorf("user scope resolution is unavailable in the device domain")
}

func (h *Handlers) enforceDeviceScope(ctx context.Context, permission, deviceID string) error {
	err := auth.EnforceDeviceScopeOnBaseTier(ctx, scopeResolver{h.store}, permission, deviceID)
	if err == nil {
		return nil
	}
	if connect.CodeOf(err) == connect.CodeInternal {
		return h.internal(ctx, "resolve device scope", err)
	}
	return notFound(ctx, errDeviceNotFound, "device not found")
}

func (h *Handlers) readDevice(ctx context.Context, permission, deviceID string) (store.DeviceView, error) {
	if err := h.authorize(ctx, permission, deviceID); err != nil {
		return store.DeviceView{}, err
	}
	view, err := h.store.GetDeviceView(ctx, deviceID)
	if err != nil {
		if store.IsNotFound(err) {
			return store.DeviceView{}, notFound(ctx, errDeviceNotFound, "device not found")
		}
		return store.DeviceView{}, h.internal(ctx, "read device", err)
	}
	if auth.HasPermission(ctx, permission) {
		if err := h.enforceDeviceScope(ctx, permission, deviceID); err != nil {
			return store.DeviceView{}, err
		}
		return view, nil
	}
	actor, _ := auth.UserFromContext(ctx)
	assigned, err := h.store.IsDeviceAssignedToUser(ctx, deviceID, actor.ID)
	if err != nil {
		return store.DeviceView{}, h.internal(ctx, "check device assignment", err)
	}
	if !assigned {
		return store.DeviceView{}, notFound(ctx, errDeviceNotFound, "device not found")
	}
	return view, nil
}

func (h *Handlers) mutationDevice(ctx context.Context, permission, deviceID string) (store.DeviceView, error) {
	if err := h.authorize(ctx, permission, deviceID); err != nil {
		return store.DeviceView{}, err
	}
	view, err := h.store.GetDeviceView(ctx, deviceID)
	if err != nil {
		if store.IsNotFound(err) {
			return store.DeviceView{}, notFound(ctx, errDeviceNotFound, "device not found")
		}
		return store.DeviceView{}, h.internal(ctx, "read mutation target", err)
	}
	if err := h.enforceDeviceScope(ctx, permission, deviceID); err != nil {
		return store.DeviceView{}, err
	}
	return view, nil
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

func (h *Handlers) recordSensitiveRead(
	ctx context.Context,
	req connect.AnyRequest,
	actor *auth.UserContext,
	procedure, permission, resourceType, resourceID string,
) error {
	op := h.operation(req, actor, procedure, permission)
	op.Class = store.ClassSensitiveRead
	if resourceID == "" {
		if _, err := h.store.RecordOperation(ctx, op); err != nil {
			return h.internal(ctx, "record sensitive read", err)
		}
		return nil
	}
	if _, err := h.store.RecordOperation(ctx, op, store.AuditEffect{
		ResourceType: resourceType, ResourceID: resourceID,
		Action: "READ", Outcome: store.EffectApplied,
	}); err != nil {
		return h.internal(ctx, "record sensitive read", err)
	}
	return nil
}

// ListDevices returns a keyset page narrowed in SQL by assignment, device
// scope, status, and exact label matches.
func (h *Handlers) ListDevices(ctx context.Context, req *connect.Request[cadestrov1.ListDevicesRequest]) (*connect.Response[cadestrov1.ListDevicesResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "ListDevices", ""); err != nil {
		return nil, err
	}
	if len(req.Msg.LabelFilter) > maxLabelFilters {
		return nil, rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument, "too many label filters")
	}
	if req.Msg.PageToken != "" {
		if _, err := ulid.ParseStrict(req.Msg.PageToken); err != nil {
			return nil, rpcError(ctx, errInvalidPageToken, connect.CodeInvalidArgument, "invalid page token")
		}
	}
	limit := req.Msg.PageSize
	if limit == 0 {
		limit = defaultPageSize
	}
	filter := store.DeviceListFilter{
		AfterID: req.Msg.PageToken, Limit: limit + 1,
		Status: store.DeviceStatusFilter(req.Msg.StatusFilter), Labels: req.Msg.LabelFilter,
		OnlineSince: h.now().Add(-onlineWindow),
	}
	if req.Msg.MyDevicesOnly || !auth.HasPermission(ctx, "ListDevices") {
		filter.AssignedUserID = &actor.ID
	}
	filter.ScopeGroupIDs, filter.ScopeRestricted = auth.DeviceScopeListFilter(ctx, "ListDevices")
	views, err := h.store.ListDeviceViews(ctx, filter)
	if err != nil {
		return nil, h.internal(ctx, "list devices", err)
	}
	hasMore := len(views) > int(limit)
	if hasMore {
		views = views[:limit]
	}
	countFilter := filter
	countFilter.AfterID = ""
	countFilter.Limit = 0
	total, err := h.store.CountDeviceViews(ctx, countFilter)
	if err != nil {
		return nil, h.internal(ctx, "count devices", err)
	}
	devices := make([]*cadestrov1.Device, len(views))
	for i := range views {
		devices[i] = h.toProto(views[i])
	}
	next := ""
	if hasMore {
		next = views[len(views)-1].ID
	}
	if total > math.MaxInt32 {
		total = math.MaxInt32
	}
	return connect.NewResponse(&cadestrov1.ListDevicesResponse{
		Devices: devices, NextPageToken: next, TotalCount: int32(total),
	}), nil
}

// GetDevice returns one visible device without revealing hidden device IDs.
func (h *Handlers) GetDevice(ctx context.Context, req *connect.Request[cadestrov1.GetDeviceRequest]) (*connect.Response[cadestrov1.GetDeviceResponse], error) {
	if _, err := h.actor(ctx); err != nil {
		return nil, err
	}
	view, err := h.readDevice(ctx, "GetDevice", req.Msg.Id)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.GetDeviceResponse{Device: h.toProto(view)}), nil
}

// GetDeviceInventory returns the latest directly stored osquery tables for a
// visible device.
func (h *Handlers) GetDeviceInventory(ctx context.Context, req *connect.Request[cadestrov1.GetDeviceInventoryRequest]) (*connect.Response[cadestrov1.GetDeviceInventoryResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if len(req.Msg.TableNames) > maxInventoryTableFilters {
		return nil, rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument, "too many inventory table filters")
	}
	if _, err := h.readDevice(ctx, "GetDeviceInventory", req.Msg.DeviceId); err != nil {
		return nil, err
	}
	rows, err := h.store.ListDeviceInventory(ctx, req.Msg.DeviceId, req.Msg.TableNames)
	if err != nil {
		return nil, h.internal(ctx, "list device inventory", err)
	}
	tables := make([]*cadestrov1.InventoryTableResult, len(rows))
	for i, row := range rows {
		var values []map[string]string
		if err := json.Unmarshal(row.Rows, &values); err != nil {
			return nil, h.internal(ctx, "decode device inventory", err)
		}
		protoRows := make([]*cadestrov1.OSQueryRow, len(values))
		for j, value := range values {
			protoRows[j] = &cadestrov1.OSQueryRow{Data: value}
		}
		tables[i] = &cadestrov1.InventoryTableResult{
			TableName: row.TableName, Rows: protoRows,
			CollectedAt: timestamppb.New(row.CollectedAt),
		}
	}
	if err := h.recordSensitiveRead(ctx, req, actor,
		cadestrov1connect.ControlServiceGetDeviceInventoryProcedure, "GetDeviceInventory",
		"device_inventory", req.Msg.DeviceId); err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.GetDeviceInventoryResponse{Tables: tables}), nil
}

// GetOSQueryResult returns one directly stored on-demand query result.
func (h *Handlers) GetOSQueryResult(ctx context.Context, req *connect.Request[cadestrov1.GetOSQueryResultRequest]) (*connect.Response[cadestrov1.GetOSQueryResultResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "GetOSQueryResult", ""); err != nil {
		return nil, err
	}
	result, err := h.store.GetOSQueryResult(ctx, req.Msg.QueryId)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, errQueryResultMissing, "query result not found")
		}
		return nil, h.internal(ctx, "read osquery result", err)
	}
	if _, err := h.readDevice(ctx, "GetOSQueryResult", result.DeviceID); err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			return nil, notFound(ctx, errQueryResultMissing, "query result not found")
		}
		return nil, err
	}

	response := &cadestrov1.GetOSQueryResultResponse{
		QueryId: result.QueryID, Completed: result.Completed,
		Success: result.Success, Error: result.Error,
	}
	if !result.Completed && h.now().Sub(result.CreatedAt) > resultTimeout {
		response.Completed = true
		response.Success = false
		response.Error = "query timed out: device did not respond within 5 minutes"
	} else if result.Completed && result.Success {
		var values []map[string]string
		if err := json.Unmarshal(result.Rows, &values); err != nil {
			return nil, h.internal(ctx, "decode osquery result", err)
		}
		response.Rows = make([]*cadestrov1.OSQueryRow, len(values))
		for i, value := range values {
			response.Rows[i] = &cadestrov1.OSQueryRow{Data: value}
		}
	}
	if err := h.recordSensitiveRead(ctx, req, actor,
		cadestrov1connect.ControlServiceGetOSQueryResultProcedure, "GetOSQueryResult",
		"osquery_result", result.QueryID); err != nil {
		return nil, err
	}
	return connect.NewResponse(response), nil
}

// GetDeviceLogResult returns one directly stored remote log query result.
func (h *Handlers) GetDeviceLogResult(ctx context.Context, req *connect.Request[cadestrov1.GetDeviceLogResultRequest]) (*connect.Response[cadestrov1.GetDeviceLogResultResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "GetDeviceLogResult", ""); err != nil {
		return nil, err
	}
	result, err := h.store.GetDeviceLogResult(ctx, req.Msg.QueryId)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, errQueryResultMissing, "log query result not found")
		}
		return nil, h.internal(ctx, "read device log result", err)
	}
	if _, err := h.readDevice(ctx, "GetDeviceLogResult", result.DeviceID); err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			return nil, notFound(ctx, errQueryResultMissing, "log query result not found")
		}
		return nil, err
	}

	response := &cadestrov1.GetDeviceLogResultResponse{
		QueryId: result.QueryID, Completed: result.Completed,
		Success: result.Success, Error: result.Error, Logs: result.Logs,
	}
	if !result.Completed && h.now().Sub(result.CreatedAt) > resultTimeout {
		response.Completed = true
		response.Success = false
		response.Error = "log query timed out: device did not respond within 5 minutes"
		response.Logs = ""
	}
	if err := h.recordSensitiveRead(ctx, req, actor,
		cadestrov1connect.ControlServiceGetDeviceLogResultProcedure, "GetDeviceLogResult",
		"device_log_result", result.QueryID); err != nil {
		return nil, err
	}
	return connect.NewResponse(response), nil
}

// GetDeviceCompliance returns the current direct compliance rows for one
// visible device.
func (h *Handlers) GetDeviceCompliance(ctx context.Context, req *connect.Request[cadestrov1.GetDeviceComplianceRequest]) (*connect.Response[cadestrov1.GetDeviceComplianceResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	view, err := h.readDevice(ctx, "GetDeviceCompliance", req.Msg.DeviceId)
	if err != nil {
		return nil, err
	}
	if !validComplianceStatus(view.ComplianceStatus) {
		return nil, h.internal(ctx, "decode device compliance status", fmt.Errorf("unknown status %d", view.ComplianceStatus))
	}
	rows, err := h.store.ListDeviceComplianceResults(ctx, req.Msg.DeviceId)
	if err != nil {
		return nil, h.internal(ctx, "list device compliance", err)
	}
	checks := make([]*cadestrov1.ComplianceCheckResult, len(rows))
	for i, row := range rows {
		output, err := decodeCommandOutput(row.DetectionOutput)
		if err != nil {
			return nil, h.internal(ctx, "decode compliance output", err)
		}
		checks[i] = &cadestrov1.ComplianceCheckResult{
			ActionId: row.ActionID, ActionName: row.ActionName,
			Compliant: row.Compliant, DetectionOutput: output,
			CheckedAt: timestamppb.New(row.CheckedAt),
		}
	}
	if err := h.recordSensitiveRead(ctx, req, actor,
		cadestrov1connect.ControlServiceGetDeviceComplianceProcedure, "GetDeviceCompliance",
		"device_compliance", req.Msg.DeviceId); err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.GetDeviceComplianceResponse{
		Status: cadestrov1.ComplianceStatus(view.ComplianceStatus), Checks: checks,
	}), nil
}

// GetDeviceCompliancePolicyStatus returns the current direct policy-rule
// evaluations for one visible device.
func (h *Handlers) GetDeviceCompliancePolicyStatus(ctx context.Context, req *connect.Request[cadestrov1.GetDeviceCompliancePolicyStatusRequest]) (*connect.Response[cadestrov1.GetDeviceCompliancePolicyStatusResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	view, err := h.readDevice(ctx, "GetDeviceCompliancePolicyStatus", req.Msg.DeviceId)
	if err != nil {
		return nil, err
	}
	if !validComplianceStatus(view.ComplianceStatus) {
		return nil, h.internal(ctx, "decode device compliance status", fmt.Errorf("unknown status %d", view.ComplianceStatus))
	}
	rows, err := h.store.ListDeviceComplianceEvaluations(ctx, req.Msg.DeviceId)
	if err != nil {
		return nil, h.internal(ctx, "list device compliance policies", err)
	}

	policies := make([]*cadestrov1.DevicePolicyEvaluation, 0)
	var policy *cadestrov1.DevicePolicyEvaluation
	for _, row := range rows {
		if !validComplianceStatus(row.Status) {
			return nil, h.internal(ctx, "decode policy compliance status", fmt.Errorf("unknown status %d", row.Status))
		}
		if policy == nil || policy.PolicyId != row.PolicyID {
			policy = &cadestrov1.DevicePolicyEvaluation{
				PolicyId: row.PolicyID, PolicyName: row.PolicyName,
				Status: cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_COMPLIANT,
			}
			policies = append(policies, policy)
		}
		output, err := decodeCommandOutput(row.DetectionOutput)
		if err != nil {
			return nil, h.internal(ctx, "decode policy compliance output", err)
		}
		rule := &cadestrov1.DevicePolicyRuleEvaluation{
			ActionId: row.ActionID, ActionName: row.ActionName,
			Status: cadestrov1.ComplianceStatus(row.Status), Compliant: row.Compliant,
			GracePeriodHours: row.GracePeriodHours, DetectionOutput: output,
		}
		if row.CheckedAt != nil {
			rule.CheckedAt = timestamppb.New(*row.CheckedAt)
		}
		if row.FirstFailedAt != nil {
			rule.FirstFailedAt = timestamppb.New(*row.FirstFailedAt)
			if row.GracePeriodHours > 0 {
				rule.GraceExpiresAt = timestamppb.New(row.FirstFailedAt.Add(
					time.Duration(row.GracePeriodHours) * time.Hour,
				))
			}
		}
		policy.Rules = append(policy.Rules, rule)
		policy.Status = worseComplianceStatus(policy.Status, rule.Status)
	}
	if err := h.recordSensitiveRead(ctx, req, actor,
		cadestrov1connect.ControlServiceGetDeviceCompliancePolicyStatusProcedure,
		"GetDeviceCompliancePolicyStatus", "device_compliance_policy_status", req.Msg.DeviceId); err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.GetDeviceCompliancePolicyStatusResponse{
		OverallStatus: cadestrov1.ComplianceStatus(view.ComplianceStatus), Policies: policies,
	}), nil
}

func decodeCommandOutput(raw []byte) (*cadestrov1.CommandOutput, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	output := &cadestrov1.CommandOutput{}
	if err := protojson.Unmarshal(raw, output); err != nil {
		return nil, err
	}
	return output, nil
}

func validComplianceStatus(status int32) bool {
	switch cadestrov1.ComplianceStatus(status) {
	case cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_UNKNOWN,
		cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_COMPLIANT,
		cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_NON_COMPLIANT,
		cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_IN_GRACE_PERIOD:
		return true
	default:
		return false
	}
}

func worseComplianceStatus(left, right cadestrov1.ComplianceStatus) cadestrov1.ComplianceStatus {
	priority := func(status cadestrov1.ComplianceStatus) int {
		switch status {
		case cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_NON_COMPLIANT:
			return 3
		case cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_IN_GRACE_PERIOD:
			return 2
		case cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_UNKNOWN:
			return 1
		default:
			return 0
		}
	}
	if priority(right) > priority(left) {
		return right
	}
	return left
}

// ListDeviceAssignees returns the live users and groups assigned to a device.
func (h *Handlers) ListDeviceAssignees(ctx context.Context, req *connect.Request[cadestrov1.ListDeviceAssigneesRequest]) (*connect.Response[cadestrov1.ListDeviceAssigneesResponse], error) {
	if _, err := h.actor(ctx); err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "ListDeviceAssignees", req.Msg.DeviceId); err != nil {
		return nil, err
	}
	rows, err := h.store.ListDeviceAssignees(ctx, req.Msg.DeviceId)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, errDeviceNotFound, "device not found")
		}
		return nil, h.internal(ctx, "list device assignees", err)
	}
	assignees := make([]*cadestrov1.DeviceAssignee, len(rows))
	for i, row := range rows {
		var kind cadestrov1.AssignmentTargetType
		switch row.Kind {
		case "user":
			kind = cadestrov1.AssignmentTargetType_ASSIGNMENT_TARGET_TYPE_USER
		case "user_group":
			kind = cadestrov1.AssignmentTargetType_ASSIGNMENT_TARGET_TYPE_USER_GROUP
		default:
			return nil, h.internal(ctx, "list device assignees", fmt.Errorf("unknown assignee kind"))
		}
		assignees[i] = &cadestrov1.DeviceAssignee{Id: row.ID, Type: kind, Name: row.Name}
	}
	return connect.NewResponse(&cadestrov1.ListDeviceAssigneesResponse{Assignees: assignees}), nil
}
