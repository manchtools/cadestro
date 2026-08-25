package actionparams

import (
	"fmt"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const paramsOneofName protoreflect.Name = "params"

var paramsFieldByActionType = map[cadestrov1.ActionType]protoreflect.Name{
	cadestrov1.ActionType_ACTION_TYPE_PACKAGE:      "package",
	cadestrov1.ActionType_ACTION_TYPE_APP_IMAGE:    "app",
	cadestrov1.ActionType_ACTION_TYPE_DEB:          "app",
	cadestrov1.ActionType_ACTION_TYPE_RPM:          "app",
	cadestrov1.ActionType_ACTION_TYPE_FLATPAK:      "flatpak",
	cadestrov1.ActionType_ACTION_TYPE_SHELL:        "shell",
	cadestrov1.ActionType_ACTION_TYPE_SCRIPT_RUN:   "shell",
	cadestrov1.ActionType_ACTION_TYPE_SERVICE:      "service",
	cadestrov1.ActionType_ACTION_TYPE_FILE:         "file",
	cadestrov1.ActionType_ACTION_TYPE_UPDATE:       "update",
	cadestrov1.ActionType_ACTION_TYPE_REPOSITORY:   "repository",
	cadestrov1.ActionType_ACTION_TYPE_DIRECTORY:    "directory",
	cadestrov1.ActionType_ACTION_TYPE_USER:         "user",
	cadestrov1.ActionType_ACTION_TYPE_GROUP:        "group",
	cadestrov1.ActionType_ACTION_TYPE_SSH:          "ssh",
	cadestrov1.ActionType_ACTION_TYPE_SSHD:         "sshd",
	cadestrov1.ActionType_ACTION_TYPE_ADMIN_POLICY: "admin_policy",
	cadestrov1.ActionType_ACTION_TYPE_LPS:          "lps",
	cadestrov1.ActionType_ACTION_TYPE_ENCRYPTION:   "encryption",
	cadestrov1.ActionType_ACTION_TYPE_WIFI:         "wifi",
	cadestrov1.ActionType_ACTION_TYPE_AGENT_UPDATE: "agent_update",
}

func populateParamsOneof(msg proto.Message, actionType cadestrov1.ActionType, paramsJSON []byte) error {
	if isNoParamsActionType(actionType) {
		return nil
	}
	fieldName, ok := paramsFieldByActionType[actionType]
	if !ok {
		return fmt.Errorf("actionparams: unhandled action type %d (%s)", int32(actionType), actionType)
	}
	m := msg.ProtoReflect()
	fd := m.Descriptor().Fields().ByName(fieldName)
	if fd == nil || fd.Message() == nil {
		return fmt.Errorf("actionparams: %s has no params message field %q for action type %s",
			m.Descriptor().FullName(), fieldName, actionType)
	}

	sub := m.NewField(fd)
	if err := unmarshalOpts.Unmarshal(paramsJSON, sub.Message().Interface()); err != nil {
		return fmt.Errorf("actionparams: unmarshal %s params: %w", actionType, err)
	}
	m.Set(fd, sub)
	return nil
}

func ExtractParamsMsg(msg proto.Message) proto.Message {
	if msg == nil {
		return nil
	}
	m := msg.ProtoReflect()
	od := m.Descriptor().Oneofs().ByName(paramsOneofName)
	if od == nil {
		return nil
	}
	fd := m.WhichOneof(od)
	if fd == nil || fd.Message() == nil {
		return nil
	}
	return m.Get(fd).Message().Interface()
}

func ParamsMatchType(msg proto.Message, actionType cadestrov1.ActionType) bool {
	want, ok := paramsFieldByActionType[actionType]
	if !ok {
		return false
	}
	if msg == nil {
		return actionType == cadestrov1.ActionType_ACTION_TYPE_UPDATE
	}
	m := msg.ProtoReflect()
	od := m.Descriptor().Oneofs().ByName(paramsOneofName)
	if od == nil {
		return false
	}
	set := m.WhichOneof(od)
	if actionType == cadestrov1.ActionType_ACTION_TYPE_UPDATE && set == nil {
		return true
	}
	return set != nil && set.Name() == want
}

func registryFieldsAreValid(msgs ...proto.Message) (ok bool, detail string) {
	for _, msg := range msgs {
		m := msg.ProtoReflect()
		fields := m.Descriptor().Fields()
		for at, name := range paramsFieldByActionType {
			fd := fields.ByName(name)
			if fd == nil {
				return false, fmt.Sprintf("%s missing params field %q (for %s)", m.Descriptor().FullName(), name, at)
			}
			if fd.Message() == nil {
				return false, fmt.Sprintf("%s field %q is not a message field (for %s)", m.Descriptor().FullName(), name, at)
			}
			if fd.ContainingOneof() == nil || fd.ContainingOneof().Name() != paramsOneofName {
				return false, fmt.Sprintf("%s field %q is not part of the %q oneof (for %s)", m.Descriptor().FullName(), name, paramsOneofName, at)
			}
		}
	}
	return true, ""
}
