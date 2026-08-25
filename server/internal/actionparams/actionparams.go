package actionparams

import (
	"fmt"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var unmarshalOpts = protojson.UnmarshalOptions{}

var marshalOptions = protojson.MarshalOptions{
	EmitUnpopulated: true,
	UseProtoNames:   false,
}

func MarshalActionParams(msg proto.Message) ([]byte, error) {
	if msg == nil {
		return nil, fmt.Errorf("actionparams.MarshalActionParams: nil message")
	}
	return marshalOptions.Marshal(msg)
}

func UnmarshalActionParams(raw []byte, msg proto.Message) error {
	if msg == nil {
		return fmt.Errorf("actionparams.UnmarshalActionParams: nil message")
	}
	return unmarshalOpts.Unmarshal(raw, msg)
}

func isNoParamsActionType(t cadestrov1.ActionType) bool {
	switch t {
	case cadestrov1.ActionType_ACTION_TYPE_UNSPECIFIED:
		return true
	default:
		return false
	}
}

func PopulateAction(action *cadestrov1.Action, actionType int32, paramsJSON []byte) error {
	return populateParamsOneof(action, cadestrov1.ActionType(actionType), paramsJSON)
}

func PopulateManagedAction(action *cadestrov1.ManagedAction, actionType cadestrov1.ActionType, paramsJSON []byte) error {
	return populateParamsOneof(action, actionType, paramsJSON)
}

func PopulateUpdateActionParams(request *cadestrov1.UpdateActionParamsRequest, actionType cadestrov1.ActionType, paramsJSON []byte) error {
	return populateParamsOneof(request, actionType, paramsJSON)
}
