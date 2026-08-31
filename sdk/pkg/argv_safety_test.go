package pkg

import (
	"context"
	"reflect"
	"slices"
	"testing"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

func TestEveryManagerMethodNeutralizesFlagShapedOperands(t *testing.T) {
	const flag = "-rf"
	const safe = "coreutils"

	stubLookPath(t, "apt", "apt-get", "dnf", "pacman", "zypper")

	mt := reflect.TypeOf((*Manager)(nil)).Elem()
	if mt.NumMethod() == 0 {
		t.Fatal("matches-zero guard: Manager has no methods")
	}
	ctxType := reflect.TypeOf((*context.Context)(nil)).Elem()
	backends := []Backend{Apt, Dnf, Dnf5, Pacman, Zypper}
	checked := 0

	isStringOperand := func(ft reflect.Type, p int) bool {
		pt := ft.In(p)
		if pt.Kind() == reflect.String {
			return true
		}
		return ft.IsVariadic() && p == ft.NumIn()-1 && pt.Elem().Kind() == reflect.String
	}

	for _, b := range backends {
		probe, _ := mustNew(t, b)
		for i := 0; i < mt.NumMethod(); i++ {
			name := mt.Method(i).Name
			ft := reflect.ValueOf(probe).MethodByName(name).Type()

			var targets []int
			for p := 0; p < ft.NumIn(); p++ {
				if isStringOperand(ft, p) {
					targets = append(targets, p)
				}
			}

			for _, target := range targets {
				f := newFake()
				m, err := New(b, f)
				if err != nil {
					t.Fatalf("New(%v): %v", b, err)
				}
				fn := reflect.ValueOf(m).MethodByName(name)

				args := make([]reflect.Value, ft.NumIn())
				for p := 0; p < ft.NumIn(); p++ {
					pt := ft.In(p)
					val := safe
					if p == target {
						val = flag
					}
					switch {
					case pt == ctxType:
						args[p] = reflect.ValueOf(context.Background())
					case ft.IsVariadic() && p == ft.NumIn()-1 && pt.Elem().Kind() == reflect.String:
						args[p] = reflect.ValueOf([]string{val})
					case pt.Kind() == reflect.String:
						args[p] = reflect.ValueOf(val)
					default:
						args[p] = reflect.Zero(pt)
					}
				}
				if ft.IsVariadic() {
					fn.CallSlice(args)
				} else {
					fn.Call(args)
				}

				for _, c := range f.Calls() {
					idx := slices.Index(c.Args, flag)
					if idx < 0 {
						continue
					}
					sep := slices.Index(c.Args, sysexec.EndOfOptions)
					if sep < 0 || sep > idx {
						t.Errorf("%s.%s (operand #%d): %q reaches argv as an OPTION — no %q separator before it: %s %v",
							b, name, target, flag, sysexec.EndOfOptions, c.Name, c.Args)
					}
				}
				checked++
			}
		}
	}
	if checked == 0 {
		t.Fatal("matches-zero guard: no operand-taking Manager methods were exercised")
	}
}

func TestSearch_RejectsFlagShapedQuery(t *testing.T) {
	ctx := context.Background()
	for _, b := range []Backend{Apt, Dnf, Dnf5, Pacman, Zypper} {
		t.Run(b.String(), func(t *testing.T) {
			stubLookPath(t, "apt", "apt-get", "dnf", "pacman", "zypper")
			m, f := mustNew(t, b)
			if _, err := m.Search(ctx, "-rf"); err == nil {
				t.Errorf("Search(%q) = nil error, want a validation error", "-rf")
			}
			if n := len(f.Calls()); n != 0 {
				t.Errorf("Search ran %d command(s) on a flag-shaped query; want 0", n)
			}

			ok(f, "")
			if _, err := m.Search(ctx, "vim"); err != nil {
				t.Errorf("Search(vim) = %v, want nil", err)
			}
			if n := len(f.Calls()); n != 1 {
				t.Errorf("Search(vim) ran %d command(s); want 1", n)
			}
		})
	}
}

func TestValidateSearchQuery(t *testing.T) {
	for _, q := range []string{"vim", "lib-foo", "c++", "gtk3.0", "", "x86_64"} {
		if err := ValidateSearchQuery(q); err != nil {
			t.Errorf("ValidateSearchQuery(%q) = %v, want nil", q, err)
		}
	}
	for _, q := range []string{"-rf", "--installed", "-x", "vim\nrm", "a\x00b"} {
		if err := ValidateSearchQuery(q); err == nil {
			t.Errorf("ValidateSearchQuery(%q) = nil, want an error", q)
		}
	}
}
