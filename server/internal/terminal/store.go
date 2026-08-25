package terminal

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
)

const DefaultTokenTTL = 60 * time.Second

var (
	ErrTokenNotFound = errors.New("terminal: session token not found")

	ErrTokenMismatch = errors.New("terminal: session token mismatch")
)

type Session struct {
	SessionID string `json:"session_id"`

	UserID string `json:"user_id"`

	DeviceID string `json:"device_id"`

	TtyUser string `json:"tty_user"`

	Cols uint32 `json:"cols"`
	Rows uint32 `json:"rows"`

	CreatedAt time.Time `json:"created_at"`

	ExpiresAt time.Time `json:"expires_at"`

	TokenHash string `json:"token_hash"`
}

type SessionBackend interface {
	Set(ctx context.Context, sessionID string, payload []byte, ttl time.Duration) error

	Get(ctx context.Context, sessionID string) ([]byte, error)

	Delete(ctx context.Context, sessionID string) error

	GetAndDelete(ctx context.Context, sessionID string) ([]byte, error)
}

type TokenStore struct {
	backend SessionBackend
	ttl     time.Duration
	now     func() time.Time
}

type TokenStoreOption func(*TokenStore)

func WithTTL(ttl time.Duration) TokenStoreOption {
	return func(s *TokenStore) {
		if ttl > 0 {
			s.ttl = ttl
		}
	}
}

func WithClock(now func() time.Time) TokenStoreOption {
	return func(s *TokenStore) {
		if now != nil {
			s.now = now
		}
	}
}

func NewTokenStore(backend SessionBackend, opts ...TokenStoreOption) *TokenStore {
	if backend == nil {
		panic("terminal: NewTokenStore requires a non-nil SessionBackend")
	}
	s := &TokenStore{
		backend: backend,
		ttl:     DefaultTokenTTL,
		now:     time.Now,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

type MintParams struct {
	UserID   string
	DeviceID string
	TtyUser  string
	Cols     uint32
	Rows     uint32
}

type MintResult struct {
	SessionID string
	Token     string
	ExpiresAt time.Time
}

func (s *TokenStore) Mint(ctx context.Context, params MintParams) (*MintResult, error) {
	return s.MintWithID(ctx, ulid.Make().String(), params)
}

func (s *TokenStore) MintWithID(ctx context.Context, sessionID string, params MintParams) (*MintResult, error) {
	if sessionID == "" {
		return nil, errors.New("terminal: session_id is required")
	}
	if params.UserID == "" || params.DeviceID == "" || params.TtyUser == "" {
		return nil, errors.New("terminal: mint requires user_id, device_id, and tty_user")
	}

	token, err := generateOpaqueToken(32)
	if err != nil {
		return nil, fmt.Errorf("terminal: generate token: %w", err)
	}

	now := s.now()
	expiresAt := now.Add(s.ttl)
	session := Session{
		SessionID: sessionID,
		UserID:    params.UserID,
		DeviceID:  params.DeviceID,
		TtyUser:   params.TtyUser,
		Cols:      params.Cols,
		Rows:      params.Rows,
		CreatedAt: now,
		ExpiresAt: expiresAt,
		TokenHash: hashToken(token),
	}
	payload, err := json.Marshal(session)
	if err != nil {
		return nil, fmt.Errorf("terminal: marshal session: %w", err)
	}
	if err := s.backend.Set(ctx, sessionID, payload, s.ttl); err != nil {
		return nil, fmt.Errorf("terminal: persist session: %w", err)
	}
	return &MintResult{
		SessionID: sessionID,
		Token:     token,
		ExpiresAt: expiresAt,
	}, nil
}

func (s *TokenStore) Lookup(ctx context.Context, sessionID string) (*Session, error) {
	payload, err := s.backend.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	var session Session
	if err := json.Unmarshal(payload, &session); err != nil {
		return nil, fmt.Errorf("terminal: decode session %s: %w", sessionID, err)
	}
	return &session, nil
}

func (s *TokenStore) Validate(ctx context.Context, sessionID, bearerToken string) (*Session, error) {
	payload, err := s.backend.GetAndDelete(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	var session Session
	if err := json.Unmarshal(payload, &session); err != nil {
		return nil, fmt.Errorf("terminal: decode session %s: %w", sessionID, err)
	}
	if subtle.ConstantTimeCompare([]byte(session.TokenHash), []byte(hashToken(bearerToken))) != 1 {

		remaining := session.ExpiresAt.Sub(s.now())
		if remaining > 0 {
			if restoreErr := s.backend.Set(ctx, sessionID, payload, remaining); restoreErr != nil {

				return nil, ErrTokenMismatch
			}
		}
		return nil, ErrTokenMismatch
	}
	return &session, nil
}

func (s *TokenStore) Revoke(ctx context.Context, sessionID string) error {
	return s.backend.Delete(ctx, sessionID)
}

func generateOpaqueToken(numBytes int) (string, error) {
	buf := make([]byte, numBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
