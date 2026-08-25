package store

import (
	"context"
	"strings"
)

const TTYSettingKey = "tty.enabled"

func (s *Store) IsTTYEnabled(ctx context.Context) (bool, error) {
	v, err := s.GetSetting(ctx, TTYSettingKey)
	if err != nil {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "enabled", "yes", "on":
		return true, nil
	}
	return false, nil
}

func (s *Store) SetTTYEnabled(ctx context.Context, enabled bool) error {
	v := "0"
	if enabled {
		v = "1"
	}
	return s.SetSetting(ctx, TTYSettingKey, v)
}
