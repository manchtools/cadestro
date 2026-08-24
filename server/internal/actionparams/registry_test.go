package actionparams

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func TestEveryActionTypeUsesOneParamsRegistry(t *testing.T) {
	require.NotEmpty(t, cadestrov1.ActionType_name)
	ok, detail := registryFieldsAreValid(
		&cadestrov1.Action{}, &cadestrov1.ManagedAction{}, &cadestrov1.CreateActionRequest{}, &cadestrov1.UpdateActionParamsRequest{},
	)
	require.Truef(t, ok, "registry inconsistent with the contract: %s", detail)

	for value, name := range cadestrov1.ActionType_name {
		actionType := cadestrov1.ActionType(value)
		t.Run(name, func(t *testing.T) {
			action := &cadestrov1.Action{}
			require.NoError(t, PopulateAction(action, value, []byte(`{}`)))
			managed := &cadestrov1.ManagedAction{}
			require.NoError(t, PopulateManagedAction(managed, actionType, []byte(`{}`)))

			_, registered := paramsFieldByActionType[actionType]
			if isNoParamsActionType(actionType) {
				assert.False(t, registered)
				assert.Nil(t, action.Params)
				assert.Nil(t, managed.Params)
				assert.Nil(t, ExtractParamsMsg(action))
				assert.False(t, ParamsMatchType(action, actionType))
				return
			}
			require.Truef(t, registered, "classify new action type %s", name)
			actionParams := ExtractParamsMsg(action)
			managedParams := ExtractParamsMsg(managed)
			require.NotNil(t, actionParams)
			require.NotNil(t, managedParams)
			if actionType == cadestrov1.ActionType_ACTION_TYPE_ENCRYPTION || actionType == cadestrov1.ActionType_ACTION_TYPE_WIFI {
				assert.NotEqual(t, proto.MessageName(actionParams), proto.MessageName(managedParams))
			} else {
				assert.Equal(t, proto.MessageName(actionParams), proto.MessageName(managedParams))
			}
			assert.True(t, ParamsMatchType(action, actionType))
		})
	}
}

func TestParamsMatchType_RejectsMismatchedOneof(t *testing.T) {
	mismatch := &cadestrov1.Action{}
	require.NoError(t, PopulateAction(mismatch, int32(cadestrov1.ActionType_ACTION_TYPE_SSH), []byte(`{}`)))
	assert.False(t, ParamsMatchType(mismatch, cadestrov1.ActionType_ACTION_TYPE_USER))
	assert.True(t, ParamsMatchType(mismatch, cadestrov1.ActionType_ACTION_TYPE_SSH))

	empty := &cadestrov1.Action{}
	assert.True(t, ParamsMatchType(empty, cadestrov1.ActionType_ACTION_TYPE_UPDATE))
	assert.False(t, ParamsMatchType(empty, cadestrov1.ActionType_ACTION_TYPE_PACKAGE))
}
