package contract

import (
	"testing"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func TestAdminAccessLevel_WireNumbersAreStable(t *testing.T) {
	cases := []struct {
		name string
		got  int32
		want int32
	}{
		{"UNSPECIFIED", int32(cadestrov1.AdminAccessLevel_ADMIN_ACCESS_LEVEL_UNSPECIFIED), 0},
		{"FULL", int32(cadestrov1.AdminAccessLevel_ADMIN_ACCESS_LEVEL_FULL), 1},
		{"LIMITED", int32(cadestrov1.AdminAccessLevel_ADMIN_ACCESS_LEVEL_LIMITED), 2},
		{"CUSTOM", int32(cadestrov1.AdminAccessLevel_ADMIN_ACCESS_LEVEL_CUSTOM), 3},
		{"TERMINAL_ADMIN_LIMITED", int32(cadestrov1.AdminAccessLevel_ADMIN_ACCESS_LEVEL_TERMINAL_ADMIN_LIMITED), 4},
		{"TERMINAL_ADMIN_FULL", int32(cadestrov1.AdminAccessLevel_ADMIN_ACCESS_LEVEL_TERMINAL_ADMIN_FULL), 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("AdminAccessLevel %s = %d; want %d (wire-compat break)", tc.name, tc.got, tc.want)
			}
		})
	}
}

func TestAdminAccessLevel_TerminalAdminValuesAreDistinct(t *testing.T) {
	tlim := cadestrov1.AdminAccessLevel_ADMIN_ACCESS_LEVEL_TERMINAL_ADMIN_LIMITED
	tfull := cadestrov1.AdminAccessLevel_ADMIN_ACCESS_LEVEL_TERMINAL_ADMIN_FULL

	olds := []cadestrov1.AdminAccessLevel{
		cadestrov1.AdminAccessLevel_ADMIN_ACCESS_LEVEL_UNSPECIFIED,
		cadestrov1.AdminAccessLevel_ADMIN_ACCESS_LEVEL_FULL,
		cadestrov1.AdminAccessLevel_ADMIN_ACCESS_LEVEL_LIMITED,
		cadestrov1.AdminAccessLevel_ADMIN_ACCESS_LEVEL_CUSTOM,
	}
	for _, o := range olds {
		if tlim == o {
			t.Fatalf("TERMINAL_ADMIN_LIMITED (%d) collides with pre-existing value %s (%d)", tlim, o, o)
		}
		if tfull == o {
			t.Fatalf("TERMINAL_ADMIN_FULL (%d) collides with pre-existing value %s (%d)", tfull, o, o)
		}
	}
	if tlim == tfull {
		t.Fatalf("TERMINAL_ADMIN_LIMITED and TERMINAL_ADMIN_FULL collide on %d", tlim)
	}
}

func TestAdminAccessLevel_TerminalAdminValuesHaveStringNames(t *testing.T) {
	if got := cadestrov1.AdminAccessLevel_ADMIN_ACCESS_LEVEL_TERMINAL_ADMIN_LIMITED.String(); got != "ADMIN_ACCESS_LEVEL_TERMINAL_ADMIN_LIMITED" {
		t.Fatalf("TERMINAL_ADMIN_LIMITED.String() = %q; want %q", got, "ADMIN_ACCESS_LEVEL_TERMINAL_ADMIN_LIMITED")
	}
	if got := cadestrov1.AdminAccessLevel_ADMIN_ACCESS_LEVEL_TERMINAL_ADMIN_FULL.String(); got != "ADMIN_ACCESS_LEVEL_TERMINAL_ADMIN_FULL" {
		t.Fatalf("TERMINAL_ADMIN_FULL.String() = %q; want %q", got, "ADMIN_ACCESS_LEVEL_TERMINAL_ADMIN_FULL")
	}
}
