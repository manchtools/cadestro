package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func TestManifestScheduleClampsForwardClockJumpToOneInterval(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	interval := 8 * time.Hour
	lastExecuted := now.Add(10 * 24 * time.Hour)

	got := calculateNextExecuteFromSchedule(
		&pb.ActionSchedule{IntervalHours: 8},
		&lastExecuted,
		false,
		now,
	)

	require.Equal(t, now.Add(interval), got)
}

func TestManifestScheduleWithoutExplicitCadenceUsesDriftDefault(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	lastExecuted := now

	got := calculateNextExecuteFromSchedule(nil, &lastExecuted, false, now)

	require.Equal(t, now.Add(nilScheduleDrift), got)
}
