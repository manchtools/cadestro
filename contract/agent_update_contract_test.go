package contract_test

import (
	"testing"

	"buf.build/go/protovalidate"
	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func TestAgentUpdateArchRequiresSignedChecksumManifest(t *testing.T) {
	t.Parallel()
	descriptor := (&cadestrov1.AgentUpdateArch{}).ProtoReflect().Descriptor()
	if descriptor.Fields().ByName("expected_sha256") != nil || descriptor.Fields().ByNumber(3) != nil {
		t.Fatal("expected_sha256 remains in the public agent-update contract")
	}
	if descriptor.ReservedNames().Has("expected_sha256") || descriptor.ReservedRanges().Has(3) {
		t.Fatal("expected_sha256 remains as reserved pre-alpha contract history")
	}

	validator, err := protovalidate.New()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		checksumURL string
		wantOK      bool
	}{
		{name: "missing", wantOK: false},
		{name: "non-HTTPS", checksumURL: "http://releases.example/SHA256SUMS", wantOK: false},
		{name: "HTTPS", checksumURL: "https://releases.example/SHA256SUMS", wantOK: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ok := validator.Validate(&cadestrov1.AgentUpdateArch{
				BinaryUrl:   "https://releases.example/cadestrod-linux-amd64",
				ChecksumUrl: tc.checksumURL,
			}) == nil
			if ok != tc.wantOK {
				t.Fatalf("validation = %v, want %v", ok, tc.wantOK)
			}
		})
	}
}
