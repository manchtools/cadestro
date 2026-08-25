package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type environmentFixture struct {
	values     map[string]string
	sessionKey ed25519.PrivateKey
}

func newEnvironmentFixture(t *testing.T) environmentFixture {
	t.Helper()
	directory := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(directory, name)
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
		return path
	}
	_, sessionKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	encodedSession, err := x509.MarshalPKCS8PrivateKey(sessionKey)
	require.NoError(t, err)
	sessionPath := write("session.pem",
		string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encodedSession})))
	artifactPath := filepath.Join(directory, "artifacts")
	backupPath := filepath.Join(directory, "backups")
	require.NoError(t, os.Mkdir(artifactPath, 0o700))
	require.NoError(t, os.Mkdir(backupPath, 0o700))
	return environmentFixture{
		sessionKey: sessionKey,
		values: map[string]string{
			"CADESTRO_PUBLIC_BASE_URL":          "https://manage.example",
			"CADESTRO_AGENT_URL":                "https://agents.example",
			"CADESTRO_TERMINAL_URL":             "wss://manage.example/terminal",
			"CADESTRO_WEBHOOK_URL":              "https://hooks.example.test/cadestro?token=secret",
			"CADESTRO_CORS_ORIGINS":             "https://manage.example",
			"CADESTRO_AGENT_PROXY_SOURCES":      "172.30.0.2",
			"CADESTRO_ARTIFACT_PATH":            artifactPath,
			"CADESTRO_BACKUP_PATH":              backupPath,
			"CADESTRO_CA_CERT_FILE":             "/certs/ca.crt",
			"CADESTRO_CA_KEY_FILE":              "/certs/ca.key",
			"CADESTRO_AGENT_TLS_CERT_FILE":      "/certs/control.crt",
			"CADESTRO_AGENT_TLS_KEY_FILE":       "/certs/control.key",
			"CADESTRO_DATABASE_PATH":            filepath.Join(directory, "control.db"),
			"CADESTRO_ENCRYPTION_KEY_FILE":      write("encryption.key", strings.Repeat("02", 32)),
			"CADESTRO_SESSION_SIGNING_KEY_FILE": sessionPath,
		},
	}
}

func setEnvironment(t *testing.T, values map[string]string) {
	t.Helper()
	entries := make([]string, 0, len(values))
	for name, value := range values {
		t.Setenv(name, value)
		entries = append(entries, name+"="+value)
	}
	previous := configEnviron
	configEnviron = func() []string { return entries }
	t.Cleanup(func() { configEnviron = previous })
}

func TestLoadConfigResolvesEveryOptionFromTheEnvironment(t *testing.T) {
	fixture := newEnvironmentFixture(t)
	fixture.values["CADESTRO_CORS_ORIGINS"] = "https://manage.example, https://admin.example"
	fixture.values["CADESTRO_TRUSTED_PROXIES"] = "10.0.0.1 , 10.0.0.2"
	fixture.values["CADESTRO_CORS_ALLOW_ALL"] = "true"
	fixture.values["CADESTRO_HEARTBEAT_INTERVAL"] = "45s"
	fixture.values["CADESTRO_LOG_LEVEL"] = "debug"
	setEnvironment(t, fixture.values)

	cfg, err := loadConfig()
	require.NoError(t, err)

	assert.Equal(t, "https://manage.example", cfg.PublicBaseURL)
	assert.Equal(t, "https://agents.example", cfg.AgentURL)
	assert.Equal(t, "wss://manage.example/terminal", cfg.TerminalURL)
	assert.Equal(t, "https://hooks.example.test/cadestro?token=secret", cfg.WebhookURL)
	assert.Equal(t, fixture.values["CADESTRO_ARTIFACT_PATH"], cfg.ArtifactPath)
	assert.Equal(t, fixture.values["CADESTRO_BACKUP_PATH"], cfg.BackupPath)
	assert.Equal(t, fixture.values["CADESTRO_DATABASE_PATH"], cfg.DatabasePath)
	assert.Equal(t, "/certs/ca.crt", cfg.CACertFile)
	assert.Equal(t, "/certs/ca.key", cfg.CAKeyFile)
	assert.Equal(t, "/certs/control.crt", cfg.AgentTLSCertFile)
	assert.Equal(t, "/certs/control.key", cfg.AgentTLSKeyFile)

	assert.Equal(t, []string{"https://manage.example", "https://admin.example"}, cfg.CORSOrigins)
	assert.Equal(t, []string{"10.0.0.1", "10.0.0.2"}, cfg.TrustedProxies)
	assert.Equal(t, []string{"172.30.0.2"}, cfg.AgentProxySources)
	assert.True(t, cfg.CORSAllowAll)
	assert.Equal(t, []string{"manage.example", "admin.example"}, cfg.TerminalOrigins,
		"an unset terminal origin list follows the CORS origins")

	assert.Equal(t, ":8081", cfg.PublicListen)
	assert.Equal(t, ":8082", cfg.AgentListen)
	assert.Equal(t, "json", cfg.LogFormat)
	assert.Equal(t, 8760*time.Hour, cfg.CertificateValidity)
	assert.Equal(t, 26*time.Hour, cfg.BackupMaxLag)
	assert.Empty(t, cfg.PublicTLSCertFile)
	assert.Empty(t, cfg.PublicTLSKeyFile)

	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, 45*time.Second, cfg.HeartbeatInterval)

	assert.Equal(t, strings.Repeat("02", 32), cfg.EncryptionKey)
	assert.Equal(t, fixture.sessionKey, cfg.SessionSigningKey)
}

func TestLoadConfigAcceptsSecretsSuppliedDirectly(t *testing.T) {
	fixture := newEnvironmentFixture(t)
	delete(fixture.values, "CADESTRO_ENCRYPTION_KEY_FILE")
	fixture.values["CADESTRO_ENCRYPTION_KEY"] = strings.Repeat("03", 32)
	setEnvironment(t, fixture.values)

	cfg, err := loadConfig()
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("03", 32), cfg.EncryptionKey)
}

func TestLoadConfigFailsClosedAndNamesTheOffendingVariable(t *testing.T) {
	tests := map[string]struct {
		mutate   func(*testing.T, environmentFixture)
		expected []string
	}{
		"unrecognized variable": {
			mutate: func(_ *testing.T, fixture environmentFixture) {
				fixture.values["CADESTRO_TYPO"] = "1"
			},
			expected: []string{"CADESTRO_TYPO"},
		},
		"empty list entry": {
			mutate: func(_ *testing.T, fixture environmentFixture) {
				fixture.values["CADESTRO_CORS_ORIGINS"] = "https://a.example,,https://b.example"
			},
			expected: []string{"CADESTRO_CORS_ORIGINS"},
		},
		"trailing list separator": {
			mutate: func(_ *testing.T, fixture environmentFixture) {
				fixture.values["CADESTRO_AGENT_PROXY_SOURCES"] = "172.30.0.2,"
			},
			expected: []string{"CADESTRO_AGENT_PROXY_SOURCES"},
		},
		"blank list entry": {
			mutate: func(_ *testing.T, fixture environmentFixture) {
				fixture.values["CADESTRO_TERMINAL_ORIGINS"] = "manage.example,   ,admin.example"
			},
			expected: []string{"CADESTRO_TERMINAL_ORIGINS"},
		},
		"invalid boolean": {
			mutate: func(_ *testing.T, fixture environmentFixture) {
				fixture.values["CADESTRO_CORS_ALLOW_ALL"] = "yes-please"
			},
			expected: []string{"CADESTRO_CORS_ALLOW_ALL"},
		},
		"invalid duration": {
			mutate: func(_ *testing.T, fixture environmentFixture) {
				fixture.values["CADESTRO_CERTIFICATE_VALIDITY"] = "forever"
			},
			expected: []string{"CADESTRO_CERTIFICATE_VALIDITY"},
		},
		"missing encryption key": {
			mutate: func(_ *testing.T, fixture environmentFixture) {
				delete(fixture.values, "CADESTRO_ENCRYPTION_KEY_FILE")
			},
			expected: []string{"CADESTRO_ENCRYPTION_KEY", "CADESTRO_ENCRYPTION_KEY_FILE"},
		},
		"encryption key supplied twice": {
			mutate: func(_ *testing.T, fixture environmentFixture) {
				fixture.values["CADESTRO_ENCRYPTION_KEY"] = strings.Repeat("05", 32)
			},
			expected: []string{"CADESTRO_ENCRYPTION_KEY", "CADESTRO_ENCRYPTION_KEY_FILE"},
		},
		"missing session signing key": {
			mutate: func(_ *testing.T, fixture environmentFixture) {
				delete(fixture.values, "CADESTRO_SESSION_SIGNING_KEY_FILE")
			},
			expected: []string{"CADESTRO_SESSION_SIGNING_KEY_FILE"},
		},
		"session signing key is not a PEM key": {
			mutate: func(t *testing.T, fixture environmentFixture) {
				require.NoError(t, os.WriteFile(
					fixture.values["CADESTRO_SESSION_SIGNING_KEY_FILE"], []byte("not a key"), 0o600))
			},
			expected: []string{"CADESTRO_SESSION_SIGNING_KEY_FILE"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fixture := newEnvironmentFixture(t)
			test.mutate(t, fixture)
			setEnvironment(t, fixture.values)

			cfg, err := loadConfig()
			require.Error(t, err)
			assert.Nil(t, cfg)
			for _, expected := range test.expected {
				assert.ErrorContains(t, err, expected)
			}
		})
	}
}

func TestLoadConfigKeepsExistingValidationSemantics(t *testing.T) {
	tests := map[string]struct {
		mutate   func(environmentFixture)
		expected string
	}{
		"listeners must differ": {
			mutate: func(fixture environmentFixture) {
				fixture.values["CADESTRO_PUBLIC_LISTEN"] = ":9000"
				fixture.values["CADESTRO_AGENT_LISTEN"] = ":9000"
			},
			expected: "must be distinct",
		},
		"agent proxy sources are required": {
			mutate: func(fixture environmentFixture) {
				delete(fixture.values, "CADESTRO_AGENT_PROXY_SOURCES")
			},
			expected: "isolated reverse proxy network",
		},
		"agent proxy sources must be addresses": {
			mutate: func(fixture environmentFixture) {
				fixture.values["CADESTRO_AGENT_PROXY_SOURCES"] = "not-an-address"
			},
			expected: "invalid address",
		},
		"public base URL must be https": {
			mutate: func(fixture environmentFixture) {
				fixture.values["CADESTRO_PUBLIC_BASE_URL"] = "http://manage.example"
			},
			expected: "public_base_url",
		},
		"terminal URL must end at /terminal": {
			mutate: func(fixture environmentFixture) {
				fixture.values["CADESTRO_TERMINAL_URL"] = "wss://manage.example/other"
			},
			expected: "terminal_url",
		},
		"CORS origins must be bare origins": {
			mutate: func(fixture environmentFixture) {
				fixture.values["CADESTRO_CORS_ORIGINS"] = "https://manage.example/app"
			},
			expected: "invalid CORS origin",
		},
		"public TLS certificate and key travel together": {
			mutate: func(fixture environmentFixture) {
				fixture.values["CADESTRO_PUBLIC_TLS_CERT_FILE"] = "/certs/public.crt"
			},
			expected: "must be set together",
		},
		"CA certificate is required": {
			mutate: func(fixture environmentFixture) {
				delete(fixture.values, "CADESTRO_CA_CERT_FILE")
			},
			expected: "ca_cert_file is required",
		},
		"artifact path must exist": {
			mutate: func(fixture environmentFixture) {
				fixture.values["CADESTRO_ARTIFACT_PATH"] = "/nonexistent/cadestro-artifacts"
			},
			expected: "artifact_path",
		},
		"database path must be absolute": {
			mutate: func(fixture environmentFixture) {
				fixture.values["CADESTRO_DATABASE_PATH"] = "control.db"
			},
			expected: "database_path must be an absolute file path",
		},
		"webhook URL must be https": {
			mutate: func(fixture environmentFixture) {
				fixture.values["CADESTRO_WEBHOOK_URL"] = "http://hooks.example.test"
			},
			expected: "webhook_url",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fixture := newEnvironmentFixture(t)
			test.mutate(fixture)
			setEnvironment(t, fixture.values)

			cfg, err := loadConfig()
			require.Error(t, err)
			assert.Nil(t, cfg)
			assert.ErrorContains(t, err, test.expected)
		})
	}
}

func TestLoadConfigRejectsRetiredSealingConfiguration(t *testing.T) {
	fixture := newEnvironmentFixture(t)
	fixture.values["CADESTRO_SEALING_KEY"] = "retired"
	setEnvironment(t, fixture.values)

	cfg, err := loadConfig()
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.ErrorContains(t, err, "CADESTRO_SEALING_KEY")
}

func TestEveryConfigOptionDeclaresItsVariable(t *testing.T) {
	fields := reflect.VisibleFields(reflect.TypeOf(configEnvironment{}))
	require.NotEmpty(t, fields, "the option table must declare at least one option")

	declared := declaredOptions()
	require.Len(t, declared, len(fields), "every option needs its own distinct variable")
	for _, field := range fields {
		expected := optionPrefix + upperSnake(field.Name)
		assert.Equal(t, expected, field.Tag.Get("env"),
			"option %s must declare the variable derived from its name", field.Name)
		assert.Contains(t, declared, expected)
	}
}

func upperSnake(name string) string {
	runes := []rune(name)
	var builder strings.Builder
	for index, current := range runes {
		if index > 0 && unicode.IsUpper(current) {
			var next rune
			if index+1 < len(runes) {
				next = runes[index+1]
			}
			if unicode.IsLower(runes[index-1]) || unicode.IsLower(next) {
				builder.WriteRune('_')
			}
		}
		builder.WriteRune(unicode.ToUpper(current))
	}
	return builder.String()
}

func TestParseCommandAcceptsSubcommandsAndRejectsEverythingElse(t *testing.T) {
	for name, args := range map[string][]string{
		"serve":                 {},
		"bootstrap-admin":       {"bootstrap-admin"},
		"bootstrap-admin-token": {"bootstrap-admin", "--output", "token"},
		"backup-status":         {"backup-status"},
	} {
		command, err := parseCommand(args)
		require.NoError(t, err)
		assert.Equal(t, name, command)
	}

	const hint = " (accepted commands: bootstrap-admin, backup-status)"
	for message, args := range map[string][]string{
		"unexpected arguments: -config /etc/cadestro/control.json" + hint: {"-config", "/etc/cadestro/control.json"},
		"unexpected arguments: --help" + hint:                             {"--help"},
		"unexpected arguments: serve" + hint:                              {"serve"},
		"unexpected arguments: extra":                                     {"bootstrap-admin", "extra"},
		"unexpected arguments: -config /etc/cadestro/control.json":        {"backup-status", "-config", "/etc/cadestro/control.json"},
	} {
		command, err := parseCommand(args)
		assert.EqualError(t, err, message, "%v must be rejected", args)
		assert.Empty(t, command)
	}
}

func TestValidateConfigRequiresWritableDataDirectories(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))

	err := validateWritableDirectory("artifact_path", file)
	assert.ErrorContains(t, err, "must be a directory")
	assert.ErrorContains(t, validateWritableDirectory("backup_path", ""), "is required")
}

func TestValidateDatabasePathRequiresAnAbsoluteFileInAWritableDirectory(t *testing.T) {
	directory := t.TempDir()
	assert.NoError(t, validateDatabasePath(filepath.Join(directory, "control.db")))
	assert.ErrorContains(t, validateDatabasePath("control.db"), "absolute")
	assert.ErrorContains(t, validateDatabasePath(directory), "regular file")
}

func TestSecretFileReaderRejectsLooseFiles(t *testing.T) {
	directory := t.TempDir()
	loose := filepath.Join(directory, "loose.key")
	require.NoError(t, os.WriteFile(loose, []byte("secret"), 0o644))
	_, err := readSecretFile(loose)
	assert.ErrorContains(t, err, "group/world accessible")

	_, err = readSecretFile("")
	assert.ErrorContains(t, err, "required")

	_, err = readSecretFile(directory)
	assert.ErrorContains(t, err, "small regular file")
}
