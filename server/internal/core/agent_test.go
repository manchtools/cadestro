package core

import (
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
)

func TestDeterministicULIDIsStableAndChangesWithInput(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	first, err := deterministicULID(at, "action", "revision")
	if err != nil {
		t.Fatal(err)
	}
	second, err := deterministicULID(at, "action", "revision")
	if err != nil {
		t.Fatal(err)
	}
	different, err := deterministicULID(at, "action", "next")
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first == different {
		t.Fatalf("IDs = %q, %q, %q", first, second, different)
	}
	if _, err := ulid.ParseStrict(first); err != nil {
		t.Fatalf("ParseStrict() error = %v", err)
	}
}
