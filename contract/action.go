package contract

import (
	"crypto/sha256"
	"errors"
	"fmt"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"google.golang.org/protobuf/proto"
)

// ActionDigest identifies the exact action definition executed by an agent.
func ActionDigest(action *cadestrov1.Action) ([]byte, error) {
	if action == nil {
		return nil, errors.New("action digest: action is required")
	}
	payload, err := (proto.MarshalOptions{Deterministic: true}).Marshal(action)
	if err != nil {
		return nil, fmt.Errorf("action digest: marshal action: %w", err)
	}
	digest := sha256.Sum256(payload)
	return digest[:], nil
}
