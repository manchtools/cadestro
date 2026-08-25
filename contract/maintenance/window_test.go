package maintenance_test

import (
	"errors"
	"testing"
	"time"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/maintenance"
)

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		w       *cadestrov1.MaintenanceWindow
		wantErr bool
	}{
		{"nil window", nil, false},
		{"empty schedule", &cadestrov1.MaintenanceWindow{}, false},
		{
			"valid same-day",
			&cadestrov1.MaintenanceWindow{Schedule: []*cadestrov1.MaintenanceWindowEntry{
				{Days: []string{"mon", "tue"}, Allow: "09:00-17:00"},
			}},
			false,
		},
		{
			"valid crosses midnight",
			&cadestrov1.MaintenanceWindow{Schedule: []*cadestrov1.MaintenanceWindowEntry{
				{Days: []string{"fri"}, Allow: "22:00-06:00"},
			}},
			false,
		},
		{
			"bad day",
			&cadestrov1.MaintenanceWindow{Schedule: []*cadestrov1.MaintenanceWindowEntry{
				{Days: []string{"funday"}, Allow: "00:00-23:59"},
			}},
			true,
		},
		{

			"uppercase day rejected",
			&cadestrov1.MaintenanceWindow{Schedule: []*cadestrov1.MaintenanceWindowEntry{
				{Days: []string{"MON"}, Allow: "09:00-17:00"},
			}},
			true,
		},
		{
			"mixed-case day rejected",
			&cadestrov1.MaintenanceWindow{Schedule: []*cadestrov1.MaintenanceWindowEntry{
				{Days: []string{"Mon"}, Allow: "09:00-17:00"},
			}},
			true,
		},
		{
			"duplicate day",
			&cadestrov1.MaintenanceWindow{Schedule: []*cadestrov1.MaintenanceWindowEntry{
				{Days: []string{"mon", "mon"}, Allow: "09:00-17:00"},
			}},
			true,
		},
		{
			"empty days",
			&cadestrov1.MaintenanceWindow{Schedule: []*cadestrov1.MaintenanceWindowEntry{
				{Days: nil, Allow: "09:00-17:00"},
			}},
			true,
		},
		{
			"bad clock",
			&cadestrov1.MaintenanceWindow{Schedule: []*cadestrov1.MaintenanceWindowEntry{
				{Days: []string{"mon"}, Allow: "25:00-26:00"},
			}},
			true,
		},
		{
			"signed hour rejected",
			&cadestrov1.MaintenanceWindow{Schedule: []*cadestrov1.MaintenanceWindowEntry{
				{Days: []string{"mon"}, Allow: "+9:00-17:00"},
			}},
			true,
		},
		{
			"missing dash",
			&cadestrov1.MaintenanceWindow{Schedule: []*cadestrov1.MaintenanceWindowEntry{
				{Days: []string{"mon"}, Allow: "09:0017:00X"},
			}},
			true,
		},
		{
			"zero-length range",
			&cadestrov1.MaintenanceWindow{Schedule: []*cadestrov1.MaintenanceWindowEntry{
				{Days: []string{"mon"}, Allow: "09:00-09:00"},
			}},
			true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := maintenance.Validate(tc.w)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got nil")
				}
				if !errors.Is(err, maintenance.ErrInvalidEntry) && tc.w != nil {
					t.Fatalf("want ErrInvalidEntry chain, got %v", err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestIsAllowedSameDay(t *testing.T) {
	w := &cadestrov1.MaintenanceWindow{Schedule: []*cadestrov1.MaintenanceWindowEntry{
		{Days: []string{"mon", "tue", "wed", "thu", "fri"}, Allow: "09:00-17:00"},
	}}

	mondayNoon := time.Date(2026, time.May, 4, 12, 0, 0, 0, time.UTC)
	if !maintenance.IsAllowed(w, mondayNoon) {
		t.Fatalf("want allowed at Mon noon")
	}
	mondayStart := time.Date(2026, time.May, 4, 9, 0, 0, 0, time.UTC)
	if !maintenance.IsAllowed(w, mondayStart) {
		t.Fatalf("want allowed at Mon 09:00 (start is inclusive)")
	}
	mondayEarly := time.Date(2026, time.May, 4, 8, 59, 0, 0, time.UTC)
	if maintenance.IsAllowed(w, mondayEarly) {
		t.Fatalf("want denied at Mon 08:59 (one minute before window)")
	}
	mondayEnd := time.Date(2026, time.May, 4, 17, 0, 0, 0, time.UTC)
	if maintenance.IsAllowed(w, mondayEnd) {
		t.Fatalf("want denied at Mon 17:00 (end is exclusive)")
	}
	saturday := time.Date(2026, time.May, 9, 12, 0, 0, 0, time.UTC)
	if maintenance.IsAllowed(w, saturday) {
		t.Fatalf("want denied on Saturday")
	}
}

func TestIsAllowedCrossesMidnight(t *testing.T) {
	w := &cadestrov1.MaintenanceWindow{Schedule: []*cadestrov1.MaintenanceWindowEntry{
		{Days: []string{"mon", "tue", "wed", "thu", "fri"}, Allow: "22:00-06:00"},
	}}

	if !maintenance.IsAllowed(w, time.Date(2026, time.May, 4, 22, 0, 0, 0, time.UTC)) {
		t.Fatalf("want allowed at Mon 22:00 (cross-midnight start is inclusive)")
	}

	if !maintenance.IsAllowed(w, time.Date(2026, time.May, 4, 23, 0, 0, 0, time.UTC)) {
		t.Fatalf("want allowed at Mon 23:00")
	}

	if !maintenance.IsAllowed(w, time.Date(2026, time.May, 5, 2, 0, 0, 0, time.UTC)) {
		t.Fatalf("want allowed at Tue 02:00 (tail of Mon's window)")
	}

	if maintenance.IsAllowed(w, time.Date(2026, time.May, 5, 6, 0, 0, 0, time.UTC)) {
		t.Fatalf("want denied at Tue 06:00")
	}

	if !maintenance.IsAllowed(w, time.Date(2026, time.May, 9, 2, 0, 0, 0, time.UTC)) {
		t.Fatalf("want allowed at Sat 02:00 (tail of Fri's window)")
	}

	if maintenance.IsAllowed(w, time.Date(2026, time.May, 10, 2, 0, 0, 0, time.UTC)) {
		t.Fatalf("want denied at Sun 02:00 (Saturday not listed)")
	}
}

func TestUnion(t *testing.T) {
	weekdays := &cadestrov1.MaintenanceWindow{Schedule: []*cadestrov1.MaintenanceWindowEntry{
		{Days: []string{"mon", "tue", "wed", "thu", "fri"}, Allow: "22:00-06:00"},
	}}
	weekends := &cadestrov1.MaintenanceWindow{Schedule: []*cadestrov1.MaintenanceWindowEntry{
		{Days: []string{"sat", "sun"}, Allow: "00:00-23:59"},
	}}
	empty := &cadestrov1.MaintenanceWindow{}

	got := maintenance.Union(weekdays, weekends)
	if len(got.GetSchedule()) != 2 {
		t.Fatalf("want 2 entries in concatenation, got %d", len(got.GetSchedule()))
	}

	satAfternoon := time.Date(2026, time.May, 9, 14, 0, 0, 0, time.UTC)
	if !maintenance.IsAllowed(got, satAfternoon) {
		t.Fatalf("want allowed Sat 14:00 in union")
	}

	collapsed := maintenance.Union(weekdays, empty)
	if len(collapsed.GetSchedule()) != 0 {
		t.Fatalf("want empty union when any input is empty, got %d entries", len(collapsed.GetSchedule()))
	}
	if !maintenance.IsAllowed(collapsed, satAfternoon) {
		t.Fatalf("empty union must allow any moment")
	}

	collapsedNil := maintenance.Union(weekdays, nil)
	if len(collapsedNil.GetSchedule()) != 0 {
		t.Fatalf("nil input should collapse union, got %d entries", len(collapsedNil.GetSchedule()))
	}
}

func TestIsAllowedNilOrEmpty(t *testing.T) {
	now := time.Now()
	if !maintenance.IsAllowed(nil, now) {
		t.Fatalf("nil window must allow")
	}
	if !maintenance.IsAllowed(&cadestrov1.MaintenanceWindow{}, now) {
		t.Fatalf("empty schedule must allow")
	}
}
