package store_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/manchtools/cadestro/server/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/manchtools/cadestro/server/internal/actionparams"
	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/authoring"
	"github.com/manchtools/cadestro/server/internal/compliance"
	"github.com/manchtools/cadestro/server/internal/store"
)

type complianceHandlerFixture struct {
	*actionHandlerFixture
	handlers *compliance.Handlers
}

func newComplianceHandlerFixture(t *testing.T) *complianceHandlerFixture {
	t.Helper()
	actionFixture := newActionHandlerFixture(t)
	return &complianceHandlerFixture{
		actionHandlerFixture: actionFixture,
		handlers: compliance.NewHandlers(compliance.HandlersConfig{
			Store:  actionFixture.store,
			Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			Now:    func() time.Time { return actionFixture.now },
		}),
	}
}

func createPolicyAction(t *testing.T, state *authoring.Service, name string, isCompliance bool) string {
	t.Helper()
	params, err := actionparams.MarshalActionParams(&cadestrov1.ShellParams{
		Interpreter: "/bin/sh", DetectionScript: "exit 0", IsCompliance: isCompliance,
	})
	require.NoError(t, err)
	op := actionOperation()
	action, err := state.CreateAction(context.Background(), op, authoring.CreateActionParams{
		Name: name, CreatedBy: op.ActorID, Type: cadestrov1.ActionType_ACTION_TYPE_SHELL,
		DesiredState: cadestrov1.DesiredState_DESIRED_STATE_PRESENT, Params: params,
	})
	require.NoError(t, err)
	return action.ID
}

func TestCompliancePolicyHandlers_ValidateBeforeAuthentication(t *testing.T) {
	f := newComplianceHandlerFixture(t)
	_, err := validated(f.handlers.GetCompliancePolicy)(context.Background(), connect.NewRequest(&cadestrov1.GetCompliancePolicyRequest{Id: &cadestrov1.CompliancePolicyId{Value: "bad"}}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	_, err = validated(f.handlers.GetCompliancePolicy)(context.Background(), connect.NewRequest(&cadestrov1.GetCompliancePolicyRequest{Id: &cadestrov1.CompliancePolicyId{Value: newID()}}))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

func TestCompliancePolicyHandlers_CRUDRulesAndAudit(t *testing.T) {
	f := newComplianceHandlerFixture(t)
	ctx := f.actor(
		"CreateCompliancePolicy", "GetCompliancePolicy", "ListCompliancePolicies",
		"RenameCompliancePolicy", "UpdateCompliancePolicyDescription", "DeleteCompliancePolicy",
		"AddCompliancePolicyRule", "RemoveCompliancePolicyRule", "UpdateCompliancePolicyRule",
	)
	actionState := authoring.New(authoring.Config{Store: f.store, Now: func() time.Time { return f.now }})
	actionID := createPolicyAction(t, actionState, "detect drift", true)

	created, err := f.handlers.CreateCompliancePolicy(ctx, connect.NewRequest(&cadestrov1.CreateCompliancePolicyRequest{
		Name: "baseline", Description: "required state",
	}))
	require.NoError(t, err)
	policyID := created.Msg.Policy.GetId().GetValue()
	assert.Equal(t, int32(0), created.Msg.Policy.RuleCount)
	assert.True(t, created.Msg.Policy.CreatedAt.AsTime().Equal(f.now))

	added, err := f.handlers.AddCompliancePolicyRule(ctx, connect.NewRequest(&cadestrov1.AddCompliancePolicyRuleRequest{
		PolicyId: &cadestrov1.CompliancePolicyId{Value: policyID}, ActionId: &cadestrov1.ActionId{Value: actionID}, GracePeriodHours: 24,
	}))
	require.NoError(t, err)
	require.Len(t, added.Msg.Policy.Rules, 1)
	assert.Equal(t, int32(1), added.Msg.Policy.RuleCount)
	assert.Equal(t, "detect drift", added.Msg.Policy.Rules[0].ActionName)
	rows, total, err := f.store.Search(context.Background(), store.SearchParams{
		Scope: "actions", Query: actionID, Limit: 50,
		TagFilters: map[string][]string{"is_compliance": {"true"}},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	assert.Equal(t, actionID, rows[0].ID)
	_, err = f.handlers.AddCompliancePolicyRule(ctx, connect.NewRequest(&cadestrov1.AddCompliancePolicyRuleRequest{
		PolicyId: &cadestrov1.CompliancePolicyId{Value: policyID}, ActionId: &cadestrov1.ActionId{Value: actionID},
	}))
	assert.Equal(t, connect.CodeAlreadyExists, connect.CodeOf(err))

	updatedRule, err := f.handlers.UpdateCompliancePolicyRule(ctx, connect.NewRequest(&cadestrov1.UpdateCompliancePolicyRuleRequest{
		PolicyId: &cadestrov1.CompliancePolicyId{Value: policyID}, ActionId: &cadestrov1.ActionId{Value: actionID}, GracePeriodHours: 48,
	}))
	require.NoError(t, err)
	require.Len(t, updatedRule.Msg.Policy.Rules, 1)
	assert.Equal(t, int32(48), updatedRule.Msg.Policy.Rules[0].GracePeriodHours)

	renamed, err := f.handlers.RenameCompliancePolicy(ctx, connect.NewRequest(&cadestrov1.RenameCompliancePolicyRequest{
		Id: &cadestrov1.CompliancePolicyId{Value: policyID}, Name: "renamed",
	}))
	require.NoError(t, err)
	assert.Equal(t, "renamed", renamed.Msg.Policy.Name)
	described, err := f.handlers.UpdateCompliancePolicyDescription(ctx, connect.NewRequest(&cadestrov1.UpdateCompliancePolicyDescriptionRequest{
		Id: &cadestrov1.CompliancePolicyId{Value: policyID}, Description: "direct state",
	}))
	require.NoError(t, err)
	assert.Equal(t, "direct state", described.Msg.Policy.Description)

	got, err := f.handlers.GetCompliancePolicy(ctx, connect.NewRequest(&cadestrov1.GetCompliancePolicyRequest{Id: &cadestrov1.CompliancePolicyId{Value: policyID}}))
	require.NoError(t, err)
	require.Len(t, got.Msg.Policy.Rules, 1)
	assert.Equal(t, int32(1), got.Msg.Policy.RuleCount)
	listed, err := f.handlers.ListCompliancePolicies(ctx, connect.NewRequest(&cadestrov1.ListCompliancePoliciesRequest{}))
	require.NoError(t, err)
	require.Len(t, listed.Msg.Policies, 1)
	assert.Equal(t, int32(1), listed.Msg.Policies[0].RuleCount)

	removed, err := f.handlers.RemoveCompliancePolicyRule(ctx, connect.NewRequest(&cadestrov1.RemoveCompliancePolicyRuleRequest{
		PolicyId: &cadestrov1.CompliancePolicyId{Value: policyID}, ActionId: &cadestrov1.ActionId{Value: actionID},
	}))
	require.NoError(t, err)
	assert.Empty(t, removed.Msg.Policy.Rules)
	assert.Equal(t, int32(0), removed.Msg.Policy.RuleCount)
	rows, total, err = f.store.Search(context.Background(), store.SearchParams{
		Scope: "actions", Query: actionID, Limit: 50,
		TagFilters: map[string][]string{"is_compliance": {"false"}},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	assert.Equal(t, actionID, rows[0].ID)
	_, err = f.handlers.RemoveCompliancePolicyRule(ctx, connect.NewRequest(&cadestrov1.RemoveCompliancePolicyRuleRequest{
		PolicyId: &cadestrov1.CompliancePolicyId{Value: policyID}, ActionId: &cadestrov1.ActionId{Value: actionID},
	}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))

	_, err = f.handlers.DeleteCompliancePolicy(ctx, connect.NewRequest(&cadestrov1.DeleteCompliancePolicyRequest{Id: &cadestrov1.CompliancePolicyId{Value: policyID}}))
	require.NoError(t, err)
	_, err = f.handlers.GetCompliancePolicy(ctx, connect.NewRequest(&cadestrov1.GetCompliancePolicyRequest{Id: &cadestrov1.CompliancePolicyId{Value: policyID}}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))

	for _, procedure := range compliance.MutationProcedures() {
		operation, err := latestOperationFor(t, f.store, f.raw, procedure)
		require.NoError(t, err, procedure)
		effects, err := f.store.ListAuditEffects(context.Background(), operation.OperationID)
		require.NoError(t, err, procedure)
		assert.NotEmpty(t, effects, procedure)
	}
}

func TestCompliancePolicyHandlers_RejectsOrdinaryAndOutOfScopeActions(t *testing.T) {
	f := newComplianceHandlerFixture(t)
	actionState := authoring.New(authoring.Config{Store: f.store})
	nonCompliance := createPolicyAction(t, actionState, "ordinary shell", false)
	inScope := createPolicyAction(t, actionState, "visible compliance", true)
	outOfScope := createPolicyAction(t, actionState, "hidden compliance", true)
	policyState := compliance.NewState(compliance.StateConfig{Store: f.store})
	op := actionOperation()
	policy, err := policyState.Create(context.Background(), op, compliance.CreateParams{
		Name: "target", CreatedBy: op.ActorID,
	})
	require.NoError(t, err)

	global := f.actor("AddCompliancePolicyRule")
	_, err = f.handlers.AddCompliancePolicyRule(global, connect.NewRequest(&cadestrov1.AddCompliancePolicyRuleRequest{
		PolicyId: &cadestrov1.CompliancePolicyId{Value: policy.ID}, ActionId: &cadestrov1.ActionId{Value: nonCompliance},
	}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	groupA, groupB := newID(), newID()
	_, err = f.raw.Exec(context.Background(),
		`INSERT INTO device_groups (id, name) VALUES ($1, 'A'), ($2, 'B')`, groupA, groupB)
	require.NoError(t, err)
	_, err = f.raw.Exec(context.Background(), `
		INSERT INTO assignments (id, source_type, source_id, target_type, target_id, created_at, created_by)
		VALUES ($1, 'compliance_policy', $2, 'device_group', $3, CURRENT_TIMESTAMP, $4),
		       ($5, 'action', $6, 'device_group', $3, CURRENT_TIMESTAMP, $4),
		       ($7, 'action', $8, 'device_group', $9, CURRENT_TIMESTAMP, $4)`,
		newID(), policy.ID, groupA, f.actorID,
		newID(), inScope, newID(), outOfScope, groupB)
	require.NoError(t, err)
	scoped := auth.WithUser(context.Background(), &auth.UserContext{
		ID: f.actorID, Kind: auth.PrincipalUser, Permissions: []string{"AddCompliancePolicyRule"},
		ScopedGrants: []auth.ScopedGrant{{
			Permission: "ListDevices", ScopeKind: auth.ScopeKindDeviceGroup, ScopeID: groupA,
		}},
	})
	_, err = f.handlers.AddCompliancePolicyRule(scoped, connect.NewRequest(&cadestrov1.AddCompliancePolicyRuleRequest{
		PolicyId: &cadestrov1.CompliancePolicyId{Value: policy.ID}, ActionId: &cadestrov1.ActionId{Value: outOfScope},
	}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	_, err = f.handlers.AddCompliancePolicyRule(scoped, connect.NewRequest(&cadestrov1.AddCompliancePolicyRuleRequest{
		PolicyId: &cadestrov1.CompliancePolicyId{Value: policy.ID}, ActionId: &cadestrov1.ActionId{Value: inScope},
	}))
	require.NoError(t, err)
}

// insertDetectionlessComplianceAction stores a compliance-classified shell
// action whose detection script is absent or blank. It bypasses authoring so
// the attachment guard is proven on its own rather than through the authoring
// guard that also refuses this shape.
func insertDetectionlessComplianceAction(t *testing.T, raw *testdb.DB, name, detection string) string {
	t.Helper()
	params, err := actionparams.MarshalActionParams(&cadestrov1.ShellParams{
		Interpreter: "/bin/sh", Script: "echo remediate",
		DetectionScript: detection, IsCompliance: true,
	})
	require.NoError(t, err)
	id := newID()
	_, err = raw.Exec(context.Background(), `
		INSERT INTO actions
			(id, name, action_type, desired_state, params, timeout_seconds, created_at)
		VALUES ($1, $2, $3, 1, $4, 60, CURRENT_TIMESTAMP)
	`, id, name, int32(cadestrov1.ActionType_ACTION_TYPE_SHELL), string(params))
	require.NoError(t, err)
	return id
}

// A compliance action is detection-only, so attaching one without a detection
// script must fail closed. The positive control — attaching a compliance action
// that has a detection script — is covered by
// TestCompliancePolicyHandlers_CRUDRulesAndAudit and
// TestCompliancePolicyHandlers_RejectsOrdinaryAndOutOfScopeActions.
func TestCompliancePolicyHandlers_RefuseComplianceActionWithoutDetectionScript(t *testing.T) {
	f := newComplianceHandlerFixture(t)
	policyState := compliance.NewState(compliance.StateConfig{Store: f.store})
	op := actionOperation()
	policy, err := policyState.Create(context.Background(), op, compliance.CreateParams{
		Name: "baseline", CreatedBy: op.ActorID,
	})
	require.NoError(t, err)
	ctx := f.actor("AddCompliancePolicyRule")

	for _, tc := range []struct{ name, detection string }{
		{"empty", ""},
		{"blank", " \t\n "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actionID := insertDetectionlessComplianceAction(t, f.raw, tc.name+" detection", tc.detection)

			stateErr := policyState.AddRule(context.Background(), actionOperation(), policy.ID, actionID, 0)
			require.Error(t, stateErr, "a detection-less compliance action is not attachable")
			require.ErrorIs(t, stateErr, compliance.ErrComplianceActionNeedsDetection)
			require.NotErrorIs(t, stateErr, compliance.ErrActionNotCompliance,
				"the refusal names the missing detection script, not the classification")

			_, rpcErr := f.handlers.AddCompliancePolicyRule(ctx, connect.NewRequest(&cadestrov1.AddCompliancePolicyRuleRequest{
				PolicyId: &cadestrov1.CompliancePolicyId{Value: policy.ID}, ActionId: &cadestrov1.ActionId{Value: actionID},
			}))
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(rpcErr))

			rules, err := f.store.ListCompliancePolicyRules(context.Background(), policy.ID)
			require.NoError(t, err)
			assert.Empty(t, rules)
		})
	}
}

func TestCompliancePolicyHandlers_KeysetAndDirectScope(t *testing.T) {
	f := newComplianceHandlerFixture(t)
	state := compliance.NewState(compliance.StateConfig{Store: f.store, Now: func() time.Time { return f.now }})
	create := func(name string) string {
		op := actionOperation()
		row, err := state.Create(context.Background(), op, compliance.CreateParams{Name: name, CreatedBy: op.ActorID})
		require.NoError(t, err)
		return row.ID
	}
	directID, outsideID, unassignedID := create("direct"), create("outside"), create("unassigned")
	groupA, groupB := newID(), newID()
	_, err := f.raw.Exec(context.Background(),
		`INSERT INTO device_groups (id, name) VALUES ($1, 'A'), ($2, 'B')`, groupA, groupB)
	require.NoError(t, err)
	_, err = f.raw.Exec(context.Background(), `
		INSERT INTO assignments (id, source_type, source_id, target_type, target_id, created_at, created_by)
		VALUES ($1, 'compliance_policy', $2, 'device_group', $3, $4, $5),
		       ($6, 'compliance_policy', $7, 'device_group', $8, $4, $5)`,
		newID(), directID, groupA, f.now, f.actorID,
		newID(), outsideID, groupB)
	require.NoError(t, err)

	scoped := auth.WithUser(context.Background(), &auth.UserContext{
		ID: f.actorID, Kind: auth.PrincipalUser,
		Permissions: []string{"GetCompliancePolicy", "ListCompliancePolicies", "RenameCompliancePolicy"},
		ScopedGrants: []auth.ScopedGrant{{
			Permission: "ListDevices", ScopeKind: auth.ScopeKindDeviceGroup, ScopeID: groupA,
		}},
	})
	_, err = f.handlers.GetCompliancePolicy(scoped, connect.NewRequest(&cadestrov1.GetCompliancePolicyRequest{Id: &cadestrov1.CompliancePolicyId{Value: directID}}))
	require.NoError(t, err)
	for _, id := range []string{outsideID, unassignedID} {
		_, err = f.handlers.GetCompliancePolicy(scoped, connect.NewRequest(&cadestrov1.GetCompliancePolicyRequest{Id: &cadestrov1.CompliancePolicyId{Value: id}}))
		assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), id)
	}
	_, err = f.handlers.RenameCompliancePolicy(scoped, connect.NewRequest(&cadestrov1.RenameCompliancePolicyRequest{Id: &cadestrov1.CompliancePolicyId{Value: outsideID}, Name: "denied"}))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	_, err = f.handlers.RenameCompliancePolicy(scoped, connect.NewRequest(&cadestrov1.RenameCompliancePolicyRequest{Id: &cadestrov1.CompliancePolicyId{Value: directID}, Name: "allowed"}))
	require.NoError(t, err)

	list, err := f.handlers.ListCompliancePolicies(scoped, connect.NewRequest(&cadestrov1.ListCompliancePoliciesRequest{}))
	require.NoError(t, err)
	require.Len(t, list.Msg.Policies, 1)
	assert.Equal(t, directID, list.Msg.Policies[0].GetId().GetValue())
	assert.Equal(t, int32(1), list.Msg.TotalCount)

	global := f.actor("ListCompliancePolicies")
	page, err := f.handlers.ListCompliancePolicies(global, connect.NewRequest(&cadestrov1.ListCompliancePoliciesRequest{PageSize: 1}))
	require.NoError(t, err)
	require.Len(t, page.Msg.Policies, 1)
	assert.NotEmpty(t, page.Msg.NextPageToken)
	assert.Equal(t, int32(3), page.Msg.TotalCount)
	all, err := f.handlers.ListCompliancePolicies(global, connect.NewRequest(&cadestrov1.ListCompliancePoliciesRequest{}))
	require.NoError(t, err)
	ids := []string{all.Msg.Policies[0].GetId().GetValue(), all.Msg.Policies[1].GetId().GetValue(), all.Msg.Policies[2].GetId().GetValue()}
	sort.Strings(ids)
	want := []string{directID, outsideID, unassignedID}
	sort.Strings(want)
	assert.Equal(t, want, ids)
}

func TestCompliancePolicyRules_FollowLiveActionsAndActionDeletion(t *testing.T) {
	f := newComplianceHandlerFixture(t)
	actionState := authoring.New(authoring.Config{Store: f.store})
	actionID := createPolicyAction(t, actionState, "temporary", true)
	policyState := compliance.NewState(compliance.StateConfig{Store: f.store})
	policyOp := actionOperation()
	policy, err := policyState.Create(context.Background(), policyOp, compliance.CreateParams{
		Name: "policy", CreatedBy: policyOp.ActorID,
	})
	require.NoError(t, err)
	require.NoError(t, policyState.AddRule(context.Background(), actionOperation(), policy.ID, actionID, 0))

	require.NoError(t, actionState.DeleteAction(context.Background(), actionOperation(), actionID, false))
	rules, err := f.store.ListCompliancePolicyRules(context.Background(), policy.ID)
	require.NoError(t, err)
	assert.Empty(t, rules)
}

func TestCompliancePolicyRules_ProvideTransitiveActionReadScope(t *testing.T) {
	f := newComplianceHandlerFixture(t)
	actionState := authoring.New(authoring.Config{Store: f.store})
	actionID := createPolicyAction(t, actionState, "transitive", true)
	policyState := compliance.NewState(compliance.StateConfig{Store: f.store})
	policyOp := actionOperation()
	policy, err := policyState.Create(context.Background(), policyOp, compliance.CreateParams{
		Name: "assigned policy", CreatedBy: policyOp.ActorID,
	})
	require.NoError(t, err)
	require.NoError(t, policyState.AddRule(context.Background(), actionOperation(), policy.ID, actionID, 0))
	groupID := newID()
	_, err = f.raw.Exec(context.Background(), `INSERT INTO device_groups (id, name) VALUES ($1, 'A')`, groupID)
	require.NoError(t, err)
	_, err = f.raw.Exec(context.Background(), `
		INSERT INTO assignments (id, source_type, source_id, target_type, target_id, created_at, created_by)
		VALUES ($1, 'compliance_policy', $2, 'device_group', $3, CURRENT_TIMESTAMP, $4)`,
		newID(), policy.ID, groupID, f.actorID)
	require.NoError(t, err)

	scoped := auth.WithUser(context.Background(), &auth.UserContext{
		ID: f.actorID, Kind: auth.PrincipalUser,
		Permissions: []string{"GetAction", "ListActions"},
		ScopedGrants: []auth.ScopedGrant{{
			Permission: "ListDevices", ScopeKind: auth.ScopeKindDeviceGroup, ScopeID: groupID,
		}},
	})
	_, err = f.actionHandlerFixture.handlers.GetAction(scoped, connect.NewRequest(&cadestrov1.GetActionRequest{Id: &cadestrov1.ActionId{Value: actionID}}))
	require.NoError(t, err)
	listed, err := f.actionHandlerFixture.handlers.ListActions(scoped, connect.NewRequest(&cadestrov1.ListActionsRequest{}))
	require.NoError(t, err)
	require.Len(t, listed.Msg.Actions, 1)
	assert.Equal(t, actionID, listed.Msg.Actions[0].GetId().GetValue())
}

func TestCompliancePolicyHandlers_MountsExactCRUDSurface(t *testing.T) {
	f := newComplianceHandlerFixture(t)
	assert.Equal(t, []string{
		cadestrov1connect.ControlServiceCreateCompliancePolicyProcedure,
		cadestrov1connect.ControlServiceGetCompliancePolicyProcedure,
		cadestrov1connect.ControlServiceListCompliancePoliciesProcedure,
		cadestrov1connect.ControlServiceRenameCompliancePolicyProcedure,
		cadestrov1connect.ControlServiceUpdateCompliancePolicyDescriptionProcedure,
		cadestrov1connect.ControlServiceDeleteCompliancePolicyProcedure,
		cadestrov1connect.ControlServiceAddCompliancePolicyRuleProcedure,
		cadestrov1connect.ControlServiceRemoveCompliancePolicyRuleProcedure,
		cadestrov1connect.ControlServiceUpdateCompliancePolicyRuleProcedure,
	}, f.handlers.MountPolicies(http.NewServeMux()))
}
