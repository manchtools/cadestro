package actionparams_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/actionparams"
)

func TestScheduleRoundTrip_PreservesExplicitFalse(t *testing.T) {

	s := &cadestrov1.ActionSchedule{Cron: "0 0 * * *", RunOnAssign: false, SkipIfUnchanged: false}
	raw, err := actionparams.ScheduleToRaw(s)
	require.NoError(t, err)
	require.NotNil(t, raw)

	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	assert.Contains(t, m, "runOnAssign", "explicit run_on_assign=false must be present on the wire, not dropped")
	assert.Contains(t, m, "skipIfUnchanged", "explicit skip_if_unchanged=false must be present on the wire")

	got := actionparams.ScheduleFromJSON(raw)
	require.NotNil(t, got)
	assert.True(t, proto.Equal(s, got), "schedule must round-trip losslessly via protojson")
}

func TestScheduleFromJSON_DistinguishesPresentFromEmpty(t *testing.T) {

	assert.Nil(t, actionparams.ScheduleFromJSON(nil), "nil bytes → no schedule")
	assert.Nil(t, actionparams.ScheduleFromJSON([]byte("")), "empty bytes → no schedule")
	assert.Nil(t, actionparams.ScheduleFromJSON([]byte("{}")), "{} → no schedule")
	assert.Nil(t, actionparams.ScheduleFromJSON([]byte("  {}  ")), "whitespaced {} → no schedule")
	assert.Nil(t, actionparams.ScheduleFromJSON([]byte("null")), "null → no schedule")

	got := actionparams.ScheduleFromJSON([]byte(`{"runOnAssign":true}`))
	require.NotNil(t, got)
	assert.True(t, got.RunOnAssign)

	assert.Nil(t, actionparams.ScheduleFromJSON([]byte("{not json")))
}

func TestParseSchedule_RejectsUnknownFields(t *testing.T) {
	got, err := actionparams.ParseSchedule([]byte(`{"unexpected":true}`))
	require.Error(t, err)
	assert.Nil(t, got)
}

func TestScheduleToRaw_NilOrEmptyIsAbsent(t *testing.T) {
	rawNil, err := actionparams.ScheduleToRaw(nil)
	require.NoError(t, err)
	assert.Nil(t, rawNil, "nil schedule must serialise to nil so the event omits the schedule key")

	rawEmpty, err := actionparams.ScheduleToRaw(&cadestrov1.ActionSchedule{})
	require.NoError(t, err)
	assert.Nil(t, rawEmpty, "an all-default schedule is omitted")

	rawOnFlag, err := actionparams.ScheduleToRaw(&cadestrov1.ActionSchedule{RunOnAssign: true})
	require.NoError(t, err)
	require.NotNil(t, rawOnFlag, "a schedule with run_on_assign set must serialise")
	got := actionparams.ScheduleFromJSON(rawOnFlag)
	require.NotNil(t, got)
	assert.True(t, got.RunOnAssign)
	assert.Equal(t, int32(0), got.IntervalHours, "explicit interval_hours:0 is preserved alongside run_on_assign:true")
}
