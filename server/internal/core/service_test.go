package core

import "testing"

func TestNewRejectsMissingDependencies(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("New returned nil error for missing dependencies")
	}
}
