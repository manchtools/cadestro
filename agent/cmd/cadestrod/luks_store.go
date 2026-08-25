package main

import (
	"context"
	"fmt"

	"github.com/manchtools/cadestro/agent/internal/executor"
	sdk "github.com/manchtools/cadestro/contract"
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

type clientLuksKeyStore struct {
	client   *sdk.Client
	executor *executor.Executor
}

func (s *clientLuksKeyStore) GetKey(ctx context.Context, actionID string) (string, error) {
	if s.client == nil {
		return "", fmt.Errorf("luks key store: no SDK client wired (programmer error)")
	}
	value, err := s.client.GetLuksKey(ctx, actionID)
	if err != nil {
		return "", err
	}
	return string(value), nil
}

func (s *clientLuksKeyStore) StoreKey(ctx context.Context, actionID, devicePath, passphrase string, reason pb.RotationReason) error {
	if s.client == nil {
		return fmt.Errorf("luks key store: no SDK client wired (programmer error)")
	}
	return s.client.StoreLuksKey(ctx, actionID, devicePath, []byte(passphrase), reason)
}

func (s *clientLuksKeyStore) GetLuksKey(ctx context.Context, actionID string) (string, error) {
	return s.GetKey(ctx, actionID)
}

func (s *clientLuksKeyStore) ValidateLuksToken(ctx context.Context, token string) (*sdk.ValidateLuksTokenResult, error) {
	return s.client.ValidateLuksToken(ctx, token)
}

type clientLpsPasswordStore struct {
	client *sdk.Client
}

func (s *clientLpsPasswordStore) StorePasswords(ctx context.Context, actionID string, rotations []*pb.LpsPasswordRotation) error {
	if s.client == nil {
		return fmt.Errorf("lps password store: no SDK client wired (programmer error)")
	}
	return s.client.StoreLpsPasswords(ctx, actionID, rotations)
}
