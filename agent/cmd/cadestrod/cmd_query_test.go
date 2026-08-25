package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/manchtools/cadestro/sdk/sys/osquery"
)

func TestIsNotInstalled(t *testing.T) {
	if !isNotInstalled(osquery.ErrNotInstalled) {
		t.Error("bare sentinel must be detected as not-installed")
	}
	if !isNotInstalled(fmt.Errorf("create registry: %w", osquery.ErrNotInstalled)) {
		t.Error("wrapped sentinel must still be detected (errors.Is, not ==)")
	}
	if isNotInstalled(errors.New("permission denied")) {
		t.Error("an unrelated error must not be treated as not-installed")
	}
	if isNotInstalled(nil) {
		t.Error("nil must not be treated as not-installed")
	}
}
