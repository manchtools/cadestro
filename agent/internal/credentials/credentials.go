package credentials

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
	sdkfs "github.com/manchtools/cadestro/sdk/sys/fs"
)

const (
	credentialsFile = "credentials.json"

	DefaultDataDir = "/var/lib/cadestro"
)

type Credentials struct {
	DeviceID    string `json:"device_id"`
	CACert      []byte `json:"ca_cert"`
	Certificate []byte `json:"certificate"`

	PendingCertificate []byte `json:"pending_certificate,omitempty"`

	PendingPrivateKey []byte `json:"pending_private_key,omitempty"`
	PendingCSR        []byte `json:"pending_csr,omitempty"`
	PrivateKey        []byte `json:"private_key"`

	AgentAddr string `json:"agent_addr"`
}

type Store struct {
	dataDir string
	fs      *sdkfs.Manager
	fsErr   error
}

func (c *Credentials) Ready() bool {
	return c != nil && c.DeviceID != "" && len(c.CACert) > 0 && len(c.Certificate) > 0 && len(c.PrivateKey) > 0 && c.AgentAddr != "" && len(c.PendingCSR) == 0 && len(c.PendingPrivateKey) == 0
}

func NewStore(dataDir string) *Store {
	if dataDir == "" {
		dataDir = DefaultDataDir
	}
	s := &Store{dataDir: dataDir}
	r, err := sysexec.NewRunner(sysexec.Direct)
	if err != nil {
		s.fsErr = fmt.Errorf("credentials: build direct runner: %w", err)
		return s
	}
	m, err := sdkfs.New(r)
	if err != nil {
		s.fsErr = fmt.Errorf("credentials: build fs manager: %w", err)
		return s
	}
	s.fs = m
	return s
}

func (s *Store) writeFile(ctx context.Context, path string, data []byte) error {
	if s.fsErr != nil {
		return s.fsErr
	}
	return s.fs.WriteFile(ctx, path, data, sdkfs.WriteOptions{Mode: 0600})
}

func requireOwnerOnlyDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("stat store directory %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("store path %s is not a directory", dir)
	}
	if info.Mode().Perm() != 0o700 {
		return fmt.Errorf("store directory %s must have owner-only permissions (0700), got %#o", dir, info.Mode().Perm())
	}
	return nil
}

func (s *Store) Save(ctx context.Context, creds *Credentials) error {

	if err := os.MkdirAll(s.dataDir, 0700); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	if err := os.Chmod(s.dataDir, 0700); err != nil {
		return fmt.Errorf("secure data directory: %w", err)
	}
	if err := requireOwnerOnlyDir(s.dataDir); err != nil {
		return err
	}

	plaintext, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}

	credPath := filepath.Join(s.dataDir, credentialsFile)
	if err := s.writeFile(ctx, credPath, plaintext); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}

	return nil
}

func (s *Store) Load() (*Credentials, error) {

	if err := requireOwnerOnlyDir(s.dataDir); err != nil {
		return nil, err
	}

	credPath := filepath.Join(s.dataDir, credentialsFile)
	info, err := os.Lstat(credPath)
	if err != nil {
		return nil, fmt.Errorf("stat credentials: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return nil, fmt.Errorf("credentials file %s must be a regular owner-only file (0600)", credPath)
	}
	plaintext, err := os.ReadFile(credPath)
	if err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	}

	var creds Credentials
	if err := json.Unmarshal(plaintext, &creds); err != nil {
		return nil, fmt.Errorf("unmarshal credentials: %w", err)
	}

	return &creds, nil
}
