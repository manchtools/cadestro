package core

import (
	"testing"
)

func TestRegistrationTokenUsesRandomOpaqueValueAndDigest(t *testing.T) {
	t.Parallel()
	first, firstDigest, err := registrationToken()
	if err != nil {
		t.Fatal(err)
	}
	second, secondDigest, err := registrationToken()
	if err != nil {
		t.Fatal(err)
	}
	if first == second || firstDigest == secondDigest {
		t.Fatal("registration tokens must be unique")
	}
	if len(first) != 43 || len(firstDigest) != 64 {
		t.Fatalf("token lengths = %d, %d", len(first), len(firstDigest))
	}
}
