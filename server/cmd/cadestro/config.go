package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/manchtools/cadestro/contract"
)

type Config struct {
	PublicListen          string
	AgentListen           string
	PublicBaseURL         string
	AgentURL              string
	CORSOrigins           []string
	LogLevel              string
	LogFormat             string
	CertificateValidity   time.Duration
	HeartbeatInterval     time.Duration
	CACertFile            string
	CAKeyFile             string
	AgentTLSCertFile      string
	AgentTLSKeyFile       string
	PublicTLSCertFile     string
	PublicTLSKeyFile      string
	DatabasePath          string
	SessionSigningKey     ed25519.PrivateKey
	BootstrapOIDCName     string
	BootstrapOIDCSlug     string
	BootstrapOIDCClientID string
	BootstrapOIDCIssuer   string
	BootstrapOIDCScopes   []string
}

func environment(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func durationEnvironment(name string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return parsed, nil
}

func listEnvironment(name string, fallback []string) []string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func readPrivateFile(path string) ([]byte, error) {
	if path == "" {
		return nil, errors.New("private file path is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("private file %q must be regular and not group/world accessible", path)
	}
	value, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(value) > 64<<10 {
		return nil, fmt.Errorf("private file %q is too large", path)
	}
	return value, nil
}

func loadSessionKey(path string) (ed25519.PrivateKey, error) {
	value, err := readPrivateFile(path)
	if err != nil {
		return nil, err
	}
	block, rest := pem.Decode(value)
	if block == nil || len(strings.TrimSpace(string(rest))) != 0 {
		return nil, errors.New("session signing key must contain one PEM block")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse session signing key: %w", err)
	}
	privateKey, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("session signing key must be Ed25519")
	}
	return privateKey, nil
}

func validateHTTPS(name, value string) error {
	if err := contract.ValidateHTTPSURL(value); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func loadConfig() (*Config, error) {
	certificateValidity, err := durationEnvironment("CADESTRO_CERTIFICATE_VALIDITY", 365*24*time.Hour)
	if err != nil {
		return nil, err
	}
	heartbeatInterval, err := durationEnvironment("CADESTRO_HEARTBEAT_INTERVAL", 30*time.Second)
	if err != nil {
		return nil, err
	}
	sessionKey, err := loadSessionKey(environment("CADESTRO_SESSION_SIGNING_KEY_FILE", ""))
	if err != nil {
		return nil, fmt.Errorf("CADESTRO_SESSION_SIGNING_KEY_FILE: %w", err)
	}
	config := &Config{
		PublicListen: environment("CADESTRO_PUBLIC_LISTEN", ":8081"), AgentListen: environment("CADESTRO_AGENT_LISTEN", ":8082"),
		PublicBaseURL: environment("CADESTRO_PUBLIC_BASE_URL", ""), AgentURL: environment("CADESTRO_AGENT_URL", ""),
		CORSOrigins: listEnvironment("CADESTRO_CORS_ORIGINS", nil), LogLevel: environment("CADESTRO_LOG_LEVEL", "info"), LogFormat: environment("CADESTRO_LOG_FORMAT", "json"),
		CertificateValidity: certificateValidity, HeartbeatInterval: heartbeatInterval,
		CACertFile: environment("CADESTRO_CA_CERT_FILE", ""), CAKeyFile: environment("CADESTRO_CA_KEY_FILE", ""),
		AgentTLSCertFile: environment("CADESTRO_AGENT_TLS_CERT_FILE", ""), AgentTLSKeyFile: environment("CADESTRO_AGENT_TLS_KEY_FILE", ""),
		PublicTLSCertFile: environment("CADESTRO_PUBLIC_TLS_CERT_FILE", ""), PublicTLSKeyFile: environment("CADESTRO_PUBLIC_TLS_KEY_FILE", ""),
		DatabasePath: environment("CADESTRO_DATABASE_PATH", "/var/lib/cadestro/control.db"), SessionSigningKey: sessionKey,
		BootstrapOIDCName: environment("CADESTRO_OIDC_NAME", "Company SSO"), BootstrapOIDCSlug: environment("CADESTRO_OIDC_SLUG", "sso"),
		BootstrapOIDCClientID: environment("CADESTRO_OIDC_CLIENT_ID", ""),
		BootstrapOIDCIssuer:   environment("CADESTRO_OIDC_ISSUER_URL", ""), BootstrapOIDCScopes: listEnvironment("CADESTRO_OIDC_SCOPES", []string{"openid", "profile", "email"}),
	}
	if config.PublicListen == config.AgentListen {
		return nil, errors.New("public and agent listeners must be distinct")
	}
	if err := validateHTTPS("CADESTRO_PUBLIC_BASE_URL", config.PublicBaseURL); err != nil {
		return nil, err
	}
	if err := validateHTTPS("CADESTRO_AGENT_URL", config.AgentURL); err != nil {
		return nil, err
	}
	for _, origin := range config.CORSOrigins {
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, fmt.Errorf("invalid CORS origin %q", origin)
		}
	}
	if !filepath.IsAbs(config.DatabasePath) {
		return nil, errors.New("database path must be absolute")
	}
	for name, path := range map[string]string{
		"CADESTRO_CA_CERT_FILE": config.CACertFile, "CADESTRO_CA_KEY_FILE": config.CAKeyFile,
		"CADESTRO_AGENT_TLS_CERT_FILE": config.AgentTLSCertFile, "CADESTRO_AGENT_TLS_KEY_FILE": config.AgentTLSKeyFile,
		"CADESTRO_PUBLIC_TLS_CERT_FILE": config.PublicTLSCertFile, "CADESTRO_PUBLIC_TLS_KEY_FILE": config.PublicTLSKeyFile,
	} {
		if path == "" {
			return nil, fmt.Errorf("%s is required", name)
		}
	}
	return config, nil
}
