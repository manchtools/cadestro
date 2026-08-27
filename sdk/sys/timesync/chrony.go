package timesync

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/manchtools/cadestro/sdk/sys/exec"
)

type chronyManager struct {
	r exec.Runner
}

// Status reads chrony's tracking report.
func (m *chronyManager) Status(ctx context.Context) (Status, error) {
	out, err := runRead(ctx, m.r, "chronyc", "-c", "tracking")
	if err != nil {
		return Status{}, err
	}
	return parseChronyTracking(out)
}

const chronyTrackingFields = 14

func parseChronyTracking(out string) (Status, error) {
	line := strings.TrimSpace(out)
	if line == "" {
		return Status{}, fmt.Errorf("timesync: empty chronyc tracking output")
	}
	f := strings.Split(line, ",")

	if len(f) < chronyTrackingFields {
		return Status{}, fmt.Errorf("timesync: unexpected chronyc tracking format (%d fields, want >= %d)", len(f), chronyTrackingFields)
	}
	st := Status{
		Enabled: true,
		Source:  f[1],
	}

	switch strings.TrimSpace(f[13]) {
	case "Not synchronised":
		st.Synchronized = false
	case "Normal", "Insert second", "Delete second":
		st.Synchronized = true
	default:
		return Status{}, fmt.Errorf("timesync: unrecognised chronyc leap status %q (CSV schema drift?)", strings.TrimSpace(f[13]))
	}
	if off, err := strconv.ParseFloat(f[4], 64); err == nil {
		st.OffsetSeconds = off
	}
	return st, nil
}
