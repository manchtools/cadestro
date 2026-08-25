package authoring

import (
	"errors"
	"strings"
	"testing"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

func TestValidateActionSafetyRequiresSignedAgentUpdateManifest(t *testing.T) {
	t.Parallel()
	validBinaryURL := "https://releases.example/cadestrod-linux-amd64"
	validChecksumURL := "https://releases.example/SHA256SUMS"

	tests := []struct {
		name string
		arch *cadestrov1.AgentUpdateArch
		ok   bool
	}{
		{name: "valid", arch: &cadestrov1.AgentUpdateArch{BinaryUrl: validBinaryURL, ChecksumUrl: validChecksumURL}, ok: true},
		{name: "missing checksum URL", arch: &cadestrov1.AgentUpdateArch{BinaryUrl: validBinaryURL}},
		{name: "non-HTTPS checksum URL", arch: &cadestrov1.AgentUpdateArch{BinaryUrl: validBinaryURL, ChecksumUrl: "http://releases.example/SHA256SUMS"}},
		{name: "hostless binary URL", arch: &cadestrov1.AgentUpdateArch{BinaryUrl: "https://", ChecksumUrl: validChecksumURL}},
		{name: "hostless checksum URL", arch: &cadestrov1.AgentUpdateArch{BinaryUrl: validBinaryURL, ChecksumUrl: "https://"}},
	}

	legacyWire := protowire.AppendTag(nil, 1, protowire.BytesType)
	legacyWire = protowire.AppendString(legacyWire, validBinaryURL)
	legacyWire = protowire.AppendTag(legacyWire, 3, protowire.BytesType)
	legacyWire = protowire.AppendString(legacyWire, strings.Repeat("a", 64))
	legacyArch := &cadestrov1.AgentUpdateArch{}
	if err := proto.Unmarshal(legacyWire, legacyArch); err != nil {
		t.Fatalf("unmarshal legacy agent-update field: %v", err)
	}
	tests = append(tests, struct {
		name string
		arch *cadestrov1.AgentUpdateArch
		ok   bool
	}{name: "removed expected SHA field", arch: legacyArch})

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateActionSafety(&cadestrov1.AgentUpdateParams{Amd64: tc.arch})
			if tc.ok && err != nil {
				t.Fatalf("valid signed update source rejected: %v", err)
			}
			if !tc.ok && !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("unsafe update error = %v, want ErrInvalidInput", err)
			}
		})
	}
}
