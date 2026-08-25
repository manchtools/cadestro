package actionparams

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"google.golang.org/protobuf/proto"
)

func ScheduleToRaw(s *cadestrov1.ActionSchedule) (json.RawMessage, error) {
	if s == nil || proto.Equal(s, &cadestrov1.ActionSchedule{}) {
		return nil, nil
	}
	b, err := marshalOptions.Marshal(s)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

func ScheduleFromJSON(data []byte) *cadestrov1.ActionSchedule {
	s, err := ParseSchedule(data)
	if err != nil {
		if len(bytes.TrimSpace(data)) > 0 {
			slog.Warn("actionparams: schedule JSON malformed; treating as no schedule",
				"bytes", len(data), "error", err)
		}
		return nil
	}
	return s
}

func ParseSchedule(data []byte) (*cadestrov1.ActionSchedule, error) {

	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		if len(bytes.TrimSpace(data)) == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("decode schedule JSON: %w", err)
	}
	if len(probe) == 0 {

		return nil, nil
	}
	var s cadestrov1.ActionSchedule
	if err := unmarshalOpts.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("decode schedule fields: %w", err)
	}
	return &s, nil
}
