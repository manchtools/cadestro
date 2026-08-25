package credentials

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"golang.org/x/crypto/argon2"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
	sdkfs "github.com/manchtools/cadestro/sdk/sys/fs"
)

const (
	argonTime    = 1
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32

	saltLen  = 32
	nonceLen = 12

	credentialsFile = "credentials.enc"
	saltFile        = "salt"

	DefaultDataDir = "/var/lib/cadestro"

	credentialsMagicV1 = "cadestrocred:v1:"
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

	ControlAddr string `json:"control_addr,omitempty"`
}

type Store struct {
	dataDir string
	fs      sdkfs.Manager
	fsErr   error
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

func (s *Store) writeFile(path string, data []byte) error {
	if s.fsErr != nil {
		return s.fsErr
	}
	return s.fs.WriteFile(context.Background(), path, data, sdkfs.WriteOptions{Mode: 0600})
}

func (s *Store) Exists() bool {
	_, err := os.Stat(filepath.Join(s.dataDir, credentialsFile))
	return err == nil
}

func requireOwnerOnlyDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("stat store directory %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("store path %s is not a directory", dir)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("store directory %s is group/world-writable (%#o); it must be owner-only-writable (0700)", dir, info.Mode().Perm())
	}
	return nil
}

func (s *Store) Save(creds *Credentials) error {

	if err := os.MkdirAll(s.dataDir, 0700); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	if err := os.Chmod(s.dataDir, 0700); err != nil {
		return fmt.Errorf("secure data directory: %w", err)
	}
	if err := requireOwnerOnlyDir(s.dataDir); err != nil {
		return err
	}

	salt, err := s.loadOrCreateSalt()
	if err != nil {
		return fmt.Errorf("load/create salt: %w", err)
	}

	key, err := s.deriveKey(salt)
	if err != nil {
		return fmt.Errorf("derive key: %w", err)
	}

	plaintext, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}

	ciphertext, err := encrypt(key, plaintext)
	if err != nil {
		return fmt.Errorf("encrypt credentials: %w", err)
	}
	ciphertext = append([]byte(credentialsMagicV1), ciphertext...)

	credPath := filepath.Join(s.dataDir, credentialsFile)
	if err := s.writeFile(credPath, ciphertext); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}

	return nil
}

func (s *Store) Load() (*Credentials, error) {

	if err := requireOwnerOnlyDir(s.dataDir); err != nil {
		return nil, err
	}

	saltPath := filepath.Join(s.dataDir, saltFile)
	salt, err := os.ReadFile(saltPath)
	if err != nil {
		return nil, fmt.Errorf("read salt: %w", err)
	}

	key, err := s.deriveKey(salt)
	if err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}

	credPath := filepath.Join(s.dataDir, credentialsFile)
	ciphertext, err := os.ReadFile(credPath)
	if err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	}

	if !bytes.HasPrefix(ciphertext, []byte(credentialsMagicV1)) {
		return nil, errors.New("unsupported credentials format, please re-enroll the agent (delete credentials.enc and use a fresh registration token)")
	}
	ciphertext = ciphertext[len(credentialsMagicV1):]

	plaintext, err := decrypt(key, ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypt credentials: %w", err)
	}

	var creds Credentials
	if err := json.Unmarshal(plaintext, &creds); err != nil {
		return nil, fmt.Errorf("unmarshal credentials: %w", err)
	}

	return &creds, nil
}

func (s *Store) Delete() error {
	credPath := filepath.Join(s.dataDir, credentialsFile)
	saltPath := filepath.Join(s.dataDir, saltFile)

	if err := os.Remove(credPath); err != nil && !os.IsNotExist(err) {
		slog.Warn("failed to remove credentials file", "path", credPath, "error", err)
	}
	if err := os.Remove(saltPath); err != nil && !os.IsNotExist(err) {
		slog.Warn("failed to remove salt file", "path", saltPath, "error", err)
	}

	return nil
}

func (s *Store) DataDir() string {
	return s.dataDir
}

func (s *Store) loadOrCreateSalt() ([]byte, error) {
	saltPath := filepath.Join(s.dataDir, saltFile)

	salt, err := os.ReadFile(saltPath)
	if err == nil && len(salt) == saltLen {
		return salt, nil
	}

	if err == nil {
		return nil, fmt.Errorf("salt file %s is corrupt (%d bytes, want %d) — refusing to regenerate; delete it together with %s to re-enroll", saltPath, len(salt), saltLen, credentialsFile)
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read salt: %w", err)
	}

	salt = make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}

	if err := s.writeFile(saltPath, salt); err != nil {
		return nil, fmt.Errorf("write salt: %w", err)
	}

	return salt, nil
}

func (s *Store) deriveKey(salt []byte) ([]byte, error) {
	machineID, err := getMachineID()
	if err != nil {
		return nil, fmt.Errorf("get machine ID: %w", err)
	}

	key := argon2.IDKey(machineID, salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return key, nil
}

var getMachineID = func() ([]byte, error) {

	id, err := os.ReadFile("/etc/machine-id")
	if err == nil && len(id) > 0 {
		return id, nil
	}

	id, err = os.ReadFile("/var/lib/dbus/machine-id")
	if err == nil && len(id) > 0 {
		return id, nil
	}

	return nil, errors.New("machine ID not found")
}

func MachineIDAvailable() bool {
	_, err := getMachineID()
	return err == nil
}

func encrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

func decrypt(key, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < nonceLen {
		return nil, errors.New("ciphertext too short")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := ciphertext[:nonceLen]
	ciphertext = ciphertext[nonceLen:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}
