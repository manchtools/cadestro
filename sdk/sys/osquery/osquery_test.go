package osquery

import (
	"context"
	"errors"
	"testing"

	"github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/exec/exectest"
)

func TestFindOsqueryBinary_DiscoveryOrder(t *testing.T) {
	cases := []struct {
		name      string
		installed map[string]string
		want      string
	}{
		{
			name:      "nothing installed",
			installed: nil,
			want:      "",
		},
		{
			name: "first canonical path wins",
			installed: map[string]string{
				"/usr/bin/osqueryi":         "/usr/bin/osqueryi",
				"/usr/local/bin/osqueryi":   "/usr/local/bin/osqueryi",
				"/opt/osquery/bin/osqueryi": "/opt/osquery/bin/osqueryi",
			},
			want: "/usr/bin/osqueryi",
		},
		{
			name: "second canonical path when first missing",
			installed: map[string]string{
				"/usr/local/bin/osqueryi": "/usr/local/bin/osqueryi",
			},
			want: "/usr/local/bin/osqueryi",
		},
		{
			name: "third canonical path when first two missing",
			installed: map[string]string{
				"/opt/osquery/bin/osqueryi": "/opt/osquery/bin/osqueryi",
			},
			want: "/opt/osquery/bin/osqueryi",
		},
		{

			name: "PATH fallback when no canonical path matches",
			installed: map[string]string{
				"osqueryi": "/home/linuxbrew/.linuxbrew/bin/osqueryi",
			},
			want: "/home/linuxbrew/.linuxbrew/bin/osqueryi",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			restore := lookPath
			defer func() { lookPath = restore }()
			lookPath = func(name string) (string, error) {
				resolved, ok := tc.installed[name]
				if !ok {
					return "", errors.New("not found")
				}
				return resolved, nil
			}

			got := findOsqueryBinary()
			if got != tc.want {
				t.Errorf("findOsqueryBinary() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNew_NotInstalled(t *testing.T) {
	restore := lookPath
	defer func() { lookPath = restore }()
	lookPath = func(string) (string, error) {
		return "", errors.New("not found")
	}

	q, err := New(exectest.New(exec.Direct))
	if !errors.Is(err, ErrNotInstalled) {
		t.Errorf("New: want ErrNotInstalled, got %v", err)
	}
	if q != nil {
		t.Errorf("New: want nil Querier on failure, got %+v", q)
	}
}

func TestNew_NilRunner(t *testing.T) {
	if _, err := New(nil); !errors.Is(err, exec.ErrRunnerRequired) {
		t.Errorf("New(_, nil) error = %v, want ErrRunnerRequired", err)
	}
}

func TestNew_Success(t *testing.T) {
	restore := lookPath
	defer func() { lookPath = restore }()
	lookPath = func(name string) (string, error) {
		if name == "/usr/bin/osqueryi" {
			return name, nil
		}
		return "", errors.New("not found")
	}
	q, err := New(exectest.New(exec.Direct))
	if err != nil || q == nil {
		t.Fatalf("New = (%v,%v), want a Querier", q, err)
	}
	c, ok := q.(*client)
	if !ok {
		t.Fatalf("New returned %T, want *client", q)
	}
	if c.binaryPath != "/usr/bin/osqueryi" {
		t.Errorf("binaryPath = %q", c.binaryPath)
	}
}

func TestIsInstalled(t *testing.T) {
	restore := lookPath
	defer func() { lookPath = restore }()

	c := &client{binaryPath: "/usr/bin/osqueryi", r: exectest.New(exec.Direct)}
	ctx := context.Background()

	lookPath = func(string) (string, error) {
		return "", errors.New("not found")
	}
	if c.IsInstalled(ctx) {
		t.Errorf("IsInstalled() = true with no installed paths (removal not detected)")
	}

	lookPath = func(name string) (string, error) {
		if name == "/usr/bin/osqueryi" {
			return name, nil
		}
		return "", errors.New("not found")
	}
	if !c.IsInstalled(ctx) {
		t.Errorf("IsInstalled() = false with /usr/bin/osqueryi installed")
	}
}
