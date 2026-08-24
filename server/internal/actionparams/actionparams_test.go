package actionparams_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/actionparams"
)

func TestPopulateAction_RejectsMalformedParams(t *testing.T) {
	action := &cadestrov1.Action{}
	err := actionparams.PopulateAction(action, int32(cadestrov1.ActionType_ACTION_TYPE_SHELL), []byte("{not valid json"))
	require.Error(t, err)
	assert.Nil(t, action.Params)
}

func TestPopulateAction_RejectsUnknownParams(t *testing.T) {
	action := &cadestrov1.Action{}
	err := actionparams.PopulateAction(action, int32(cadestrov1.ActionType_ACTION_TYPE_SHELL), []byte(`{"unexpected":true}`))
	require.Error(t, err)
	assert.Nil(t, action.Params)
}

func TestPopulateAction_RejectsUnknownType(t *testing.T) {
	action := &cadestrov1.Action{}
	err := actionparams.PopulateAction(action, 999999, []byte(`{"x":1}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unhandled action type")
	assert.Nil(t, action.Params)
}

func TestPopulateAction_NoParamsTypesRemainEmpty(t *testing.T) {
	for _, actionType := range []cadestrov1.ActionType{
		cadestrov1.ActionType_ACTION_TYPE_UNSPECIFIED,
	} {
		action := &cadestrov1.Action{}
		require.NoError(t, actionparams.PopulateAction(action, int32(actionType), []byte(`{}`)))
		assert.Nil(t, action.Params)
	}
}

func TestPopulateAction_EveryContractTypeIsClassified(t *testing.T) {
	require.NotEmpty(t, cadestrov1.ActionType_name)
	for value, name := range cadestrov1.ActionType_name {
		t.Run(name, func(t *testing.T) {
			action := &cadestrov1.Action{}
			require.NoErrorf(t, actionparams.PopulateAction(action, value, []byte(`{}`)),
				"classify new action type %s", name)
			managed := &cadestrov1.ManagedAction{}
			require.NoErrorf(t, actionparams.PopulateManagedAction(managed, cadestrov1.ActionType(value), []byte(`{}`)),
				"classify new managed action type %s", name)
		})
	}
}

func TestPopulateManagedAction_RejectsMalformedParams(t *testing.T) {
	action := &cadestrov1.ManagedAction{}
	err := actionparams.PopulateManagedAction(action, cadestrov1.ActionType_ACTION_TYPE_FILE, []byte("{bad"))
	require.Error(t, err)
	assert.Nil(t, action.Params)
}
