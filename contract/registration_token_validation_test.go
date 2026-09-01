package contract_test

import (
	"strings"
	"testing"

	"buf.build/go/protovalidate"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

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
