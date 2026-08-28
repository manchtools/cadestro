package contract_test

import (
	"strings"
	"testing"

	"buf.build/go/protovalidate"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func TestEnrollRequest_RequiresValidCAPin(t *testing.T) {
	t.Parallel()
	v, err := protovalidate.New()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name, pin string
		wantOK    bool
	}{
		{name: "missing", wantOK: false},
		{name: "short", pin: "abcd", wantOK: false},
		{name: "one character short", pin: strings.Repeat("a", 63), wantOK: false},
		{name: "one character long", pin: strings.Repeat("a", 65), wantOK: false},
		{name: "non-hex", pin: strings.Repeat("z", 64), wantOK: false},
		{name: "valid lowercase", pin: strings.Repeat("a", 64), wantOK: true},
		{name: "valid uppercase", pin: strings.Repeat("A", 64), wantOK: true},
		{name: "valid mixed", pin: strings.Repeat("0123abcdABCD", 5) + "0123", wantOK: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ok := v.Validate(&cadestrov1.EnrollRequest{
				ServerUrl: "https://control.example.test", Token: "token", CaFingerprintPin: tc.pin,
			}) == nil
			if ok != tc.wantOK {
				t.Fatalf("pin validation = %v, want %v", ok, tc.wantOK)
			}
		})
	}
}

func TestCreateTokenResponse_RequiresCAPin(t *testing.T) {
	t.Parallel()
	v, err := protovalidate.New()
	if err != nil {
		t.Fatal(err)
	}
	response := &cadestrov1.CreateTokenResponse{Token: &cadestrov1.RegistrationToken{Id: &cadestrov1.RegistrationTokenId{Value: "01J0000000000000000000000A"}}}
	if v.Validate(response) == nil {
		t.Fatal("token creation without the enrollment CA pin passed validation")
	}
	response.CaFingerprintPin = strings.Repeat("a", 64)
	if err := v.Validate(response); err != nil {
		t.Fatalf("valid token creation response rejected: %s", err)
	}
}
