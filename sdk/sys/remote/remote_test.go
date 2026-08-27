package remote

import (
	"context"
	"errors"
	"testing"
)

func TestErrorsAreNonNil(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"ErrInvalidConfig", ErrInvalidConfig},
		{"ErrUnsafeDestination", ErrUnsafeDestination},
		{"ErrIntegrity", ErrIntegrity},
		{"ErrToolMissing", ErrToolMissing},
		{"ErrBackendNotFound", ErrBackendNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Fatalf("%s is nil", tt.name)
			}
			if tt.err.Error() == "" {
				t.Fatalf("%s has empty Error() string", tt.name)
			}

			wrapped := errors.Join(errors.New("ctx"), tt.err)
			if !errors.Is(wrapped, tt.err) {
				t.Fatalf("%s loses identity through errors.Join", tt.name)
			}
		})
	}
}

type stubSource struct{}

func (stubSource) Fetch(ctx context.Context, dest string) (Result, error) {
	return Result{}, nil
}
func (stubSource) Wipe(ctx context.Context, dest string) error { return nil }
func (stubSource) String() string                              { return "stub" }

func TestSourceInterfaceCompiles(t *testing.T) {
	var _ Source = (*stubSource)(nil)
}
