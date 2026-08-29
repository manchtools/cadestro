package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/oklog/ulid/v2"
)

type TokenType string

const (
	TokenTypeAccess  TokenType = "access"
	TokenTypeRefresh TokenType = "refresh"
)

type Claims struct {
	jwt.RegisteredClaims
	UserID         string                  `json:"uid"`
	Email          string                  `json:"email"`
	TokenType      TokenType               `json:"type"`
	SessionVersion int32                   `json:"sv"`
	Permissions    []cadestrov1.Permission `json:"permissions,omitempty"`
}

type JWTConfig struct {
	PrivateKey         ed25519.PrivateKey
	AccessTokenExpiry  time.Duration
	RefreshTokenExpiry time.Duration
	Issuer             string
	Now                func() time.Time
}

type JWTManager struct {
	config JWTConfig
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

func NewJWTManager(config JWTConfig) (*JWTManager, error) {
	if len(config.PrivateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("auth: Ed25519 session signing key is required")
	}
	if config.AccessTokenExpiry == 0 {
		config.AccessTokenExpiry = 5 * time.Minute
	}
	if config.RefreshTokenExpiry == 0 {
		config.RefreshTokenExpiry = 7 * 24 * time.Hour
	}
	if config.Issuer == "" {
		config.Issuer = "cadestro"
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &JWTManager{config: config}, nil
}

func GenerateSessionKey() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

func (manager *JWTManager) AccessTokenTTL() time.Duration { return manager.config.AccessTokenExpiry }

func (manager *JWTManager) GenerateTokens(userID, email string, sessionVersion int32, permissions []cadestrov1.Permission) (*TokenPair, error) {
	now := manager.config.Now().UTC()
	accessExpiry := now.Add(manager.config.AccessTokenExpiry)
	access, err := manager.sign(userID, email, sessionVersion, permissions, TokenTypeAccess, accessExpiry)
	if err != nil {
		return nil, err
	}
	refresh, err := manager.sign(userID, email, sessionVersion, nil, TokenTypeRefresh, now.Add(manager.config.RefreshTokenExpiry))
	if err != nil {
		return nil, err
	}
	return &TokenPair{AccessToken: access, RefreshToken: refresh, ExpiresAt: accessExpiry}, nil
}

func (manager *JWTManager) sign(userID, email string, sessionVersion int32, permissions []cadestrov1.Permission, tokenType TokenType, expires time.Time) (string, error) {
	now := manager.config.Now().UTC()
	id, err := ulid.New(ulid.Timestamp(now), rand.Reader)
	if err != nil {
		return "", fmt.Errorf("mint token id: %w", err)
	}
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID: id.String(), Issuer: manager.config.Issuer, Subject: userID,
			IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(expires),
		},
		UserID: userID, Email: email, TokenType: tokenType, SessionVersion: sessionVersion, Permissions: permissions,
	}
	value, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).SignedString(manager.config.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return value, nil
}

func (manager *JWTManager) ValidateToken(value string, tokenType TokenType) (*Claims, error) {
	publicKey := manager.config.PrivateKey.Public().(ed25519.PublicKey)
	token, err := jwt.ParseWithClaims(value, &Claims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodEdDSA {
			return nil, errors.New("unexpected signing method")
		}
		return publicKey, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}), jwt.WithIssuer(manager.config.Issuer), jwt.WithExpirationRequired(), jwt.WithTimeFunc(manager.config.Now))
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid || claims.UserID == "" || claims.TokenType != tokenType {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
