package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/oklog/ulid/v2"
)

type TokenType string

const (
	TokenTypeAccess   TokenType = "access"
	TokenTypeRefresh  TokenType = "refresh"
	TokenTypeAPIToken TokenType = "api_token"
)

var SigningAlgorithm = jwt.SigningMethodEdDSA

var ErrNoSigningKey = errors.New("auth: no Ed25519 session signing key")

type Claims struct {
	jwt.RegisteredClaims
	UserID         string        `json:"uid"`
	Email          string        `json:"email"`
	Permissions    []string      `json:"perms,omitempty"`
	ScopedGrants   []ScopedGrant `json:"sgrants,omitempty"`
	TokenType      TokenType     `json:"type"`
	SessionVersion int32         `json:"sv,omitempty"`
}

type JWTConfig struct {
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey

	AccessTokenExpiry  time.Duration
	RefreshTokenExpiry time.Duration
	Issuer             string

	Now func() time.Time
}

type JWTManager struct {
	config JWTConfig
}

const (
	DefaultAccessTokenExpiry  = 5 * time.Minute
	DefaultRefreshTokenExpiry = 7 * 24 * time.Hour

	DefaultIssuer = "cadestro"
)

func NewJWTManager(config JWTConfig) (*JWTManager, error) {
	if len(config.PrivateKey) != ed25519.PrivateKeySize {
		return nil, ErrNoSigningKey
	}
	if config.PublicKey == nil {
		pub, ok := config.PrivateKey.Public().(ed25519.PublicKey)
		if !ok {
			return nil, ErrNoSigningKey
		}
		config.PublicKey = pub
	}
	if config.AccessTokenExpiry == 0 {
		config.AccessTokenExpiry = DefaultAccessTokenExpiry
	}
	if config.RefreshTokenExpiry == 0 {
		config.RefreshTokenExpiry = DefaultRefreshTokenExpiry
	}
	if config.Issuer == "" {
		config.Issuer = DefaultIssuer
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &JWTManager{config: config}, nil
}

func GenerateSessionKey() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate session signing key: %w", err)
	}
	return pub, priv, nil
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

func (m *JWTManager) AccessTokenTTL() time.Duration { return m.config.AccessTokenExpiry }

func (m *JWTManager) GenerateAPIToken(userID, email string, permissions []string, scopedGrants []ScopedGrant, sessionVersion int32, expiresAt time.Time) (string, string, error) {
	now := m.config.Now().UTC()
	if !expiresAt.After(now) {
		return "", "", errors.New("API token expiry must be in the future")
	}
	jti, err := ulid.New(ulid.Timestamp(now), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("mint API token id: %w", err)
	}
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID: jti.String(), Issuer: m.config.Issuer, Subject: userID,
			ExpiresAt: jwt.NewNumericDate(expiresAt.UTC()), IssuedAt: jwt.NewNumericDate(now),
		},
		UserID: userID, Email: email, Permissions: permissions, ScopedGrants: scopedGrants,
		TokenType: TokenTypeAPIToken, SessionVersion: sessionVersion,
	}
	token, err := jwt.NewWithClaims(SigningAlgorithm, claims).SignedString(m.config.PrivateKey)
	if err != nil {
		return "", "", fmt.Errorf("sign API token: %w", err)
	}
	return jti.String(), token, nil
}

func (m *JWTManager) GenerateTokens(userID, email string, permissions []string, scopedGrants []ScopedGrant, sessionVersion int32) (*TokenPair, error) {
	now := m.config.Now()
	accessExpiry := now.Add(m.config.AccessTokenExpiry)
	entropy := ulid.Monotonic(rand.Reader, 0)

	accessJTI, err := ulid.New(ulid.Timestamp(now), entropy)
	if err != nil {
		return nil, fmt.Errorf("mint access token id: %w", err)
	}
	accessClaims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        accessJTI.String(),
			Issuer:    m.config.Issuer,
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(accessExpiry),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		UserID:         userID,
		Email:          email,
		Permissions:    permissions,
		ScopedGrants:   scopedGrants,
		TokenType:      TokenTypeAccess,
		SessionVersion: sessionVersion,
	}
	accessToken, err := jwt.NewWithClaims(SigningAlgorithm, accessClaims).SignedString(m.config.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("sign access token: %w", err)
	}

	refreshJTI, err := ulid.New(ulid.Timestamp(now), entropy)
	if err != nil {
		return nil, fmt.Errorf("mint refresh token id: %w", err)
	}
	refreshClaims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        refreshJTI.String(),
			Issuer:    m.config.Issuer,
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(now.Add(m.config.RefreshTokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		UserID:         userID,
		Email:          email,
		TokenType:      TokenTypeRefresh,
		SessionVersion: sessionVersion,
	}
	refreshToken, err := jwt.NewWithClaims(SigningAlgorithm, refreshClaims).SignedString(m.config.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("sign refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    accessExpiry,
	}, nil
}

func (m *JWTManager) ValidateToken(tokenString string, expectedType TokenType) (*Claims, error) {
	claims, err := m.parseToken(tokenString)
	if err != nil {
		return nil, err
	}
	if claims.TokenType != expectedType {
		return nil, fmt.Errorf("unexpected token type: expected %s, got %s", expectedType, claims.TokenType)
	}
	if expectedType == TokenTypeAPIToken && claims.ID == "" {
		return nil, errors.New("API token carries no id")
	}
	return claims, nil
}

func (m *JWTManager) ValidateBearerToken(tokenString string) (*Claims, error) {
	claims, err := m.parseToken(tokenString)
	if err != nil {
		return nil, err
	}
	if claims.TokenType != TokenTypeAccess && claims.TokenType != TokenTypeAPIToken {
		return nil, fmt.Errorf("unexpected bearer token type %s", claims.TokenType)
	}
	if claims.TokenType == TokenTypeAPIToken && claims.ID == "" {
		return nil, errors.New("API token carries no id")
	}
	return claims, nil
}

func (m *JWTManager) parseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(
		tokenString,
		&Claims{},
		func(t *jwt.Token) (any, error) {
			if t.Method != jwt.SigningMethod(SigningAlgorithm) {
				return nil, fmt.Errorf("unexpected signing method %q", t.Method.Alg())
			}
			return m.config.PublicKey, nil
		},
		jwt.WithValidMethods([]string{SigningAlgorithm.Alg()}),
		jwt.WithIssuer(m.config.Issuer),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(m.config.Now),
	)
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	if claims.UserID == "" {
		return nil, errors.New("token carries no subject")
	}
	return claims, nil
}

type RefreshResult struct {
	Claims *Claims
	OldJTI string
	OldExp time.Time
}

func (m *JWTManager) ValidateRefreshToken(refreshTokenString string, isRevoked func(string) (bool, error)) (*RefreshResult, error) {
	claims, err := m.ValidateToken(refreshTokenString, TokenTypeRefresh)
	if err != nil {
		return nil, fmt.Errorf("validate refresh token: %w", err)
	}
	if claims.ID == "" {
		return nil, errors.New("refresh token carries no id")
	}
	if isRevoked != nil {
		revoked, err := isRevoked(claims.ID)
		if err != nil {
			return nil, fmt.Errorf("check token revocation: %w", err)
		}
		if revoked {
			return nil, errors.New("refresh token has been revoked")
		}
	}
	var exp time.Time
	if claims.ExpiresAt != nil {
		exp = claims.ExpiresAt.Time
	}
	return &RefreshResult{Claims: claims, OldJTI: claims.ID, OldExp: exp}, nil
}
