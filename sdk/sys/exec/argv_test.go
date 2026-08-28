package exec

import (
	"reflect"
	"testing"
)

func TestSeparatePositionals_InsertsEndOfOptions(t *testing.T) {
	cases := []struct {
		name        string
		flags       []string
		positionals []string
		want        []string
	}{
		{
			name:        "rpm query",
			flags:       []string{"-q"},
			positionals: []string{"bash"},
			want:        []string{"-q", "--", "bash"},
		},
		{
			name:        "rpm erase with flag-shaped name stays an operand",
			flags:       []string{"-e"},
			positionals: []string{"--eval=evil"},
			want:        []string{"-e", "--", "--eval=evil"},
		},
		{
			name:        "package install with multiple names",
			flags:       []string{"install", "-y"},
			positionals: []string{"curl", "jq"},
			want:        []string{"install", "-y", "--", "curl", "jq"},
		},
		{
			name:        "no flags",
			flags:       nil,
			positionals: []string{"x"},
			want:        []string{"--", "x"},
		},
		{
			name:        "no positionals still terminates options",
			flags:       []string{"-q"},
			positionals: nil,
			want:        []string{"-q", "--"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SeparatePositionals(tc.flags, tc.positionals...)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("SeparatePositionals(%v, %v) = %v, want %v", tc.flags, tc.positionals, got, tc.want)
			}

			sep := -1
			for i, a := range got {
				if a == EndOfOptions {
					sep = i
					break
				}
			}
			if sep < 0 {
				t.Fatalf("result %v contains no %q separator", got, EndOfOptions)
			}
			for _, p := range tc.positionals {
				idx := -1
				for i := sep + 1; i < len(got); i++ {
					if got[i] == p {
						idx = i
						break
					}
				}
				if idx < 0 {
					t.Errorf("positional %q does not appear after the %q separator", p, EndOfOptions)
				}
			}
		})
	}
}

func TestSeparatePositionals_DoesNotMutateFlags(t *testing.T) {
	flags := []string{"-q"}
	_ = SeparatePositionals(flags, "bash")
	_ = SeparatePositionals(flags, "curl")
	if !reflect.DeepEqual(flags, []string{"-q"}) {
		t.Fatalf("input flags were mutated: %v", flags)
	}
}
