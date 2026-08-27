package network

import (
	"fmt"
	"os"
	"path/filepath"
)

func writeCerts(p Profile) error {
	if err := mkdirAll(p.CertDir, 0o750); err != nil {
		return fmt.Errorf("create cert directory: %w", err)
	}
	files := []struct {
		name    string
		content string
		mode    os.FileMode
	}{
		{"ca.pem", p.CACert, 0o640},
		{"client.pem", p.ClientCert, 0o640},
		{"client-key.pem", p.ClientKey.Reveal(), 0o600},
	}
	for _, f := range files {
		if f.content == "" {
			continue
		}
		path := filepath.Join(p.CertDir, f.name)
		if err := writeFile(path, []byte(f.content), f.mode); err != nil {
			return fmt.Errorf("write %s: %w", f.name, err)
		}
	}
	return nil
}

func removeCerts(certDir string) error {
	var firstErr error
	for _, name := range []string{"ca.pem", "client.pem", "client-key.pem"} {
		if err := removeFile(filepath.Join(certDir, name)); err != nil && !os.IsNotExist(err) && firstErr == nil {
			firstErr = fmt.Errorf("remove %s: %w", name, err)
		}
	}
	return firstErr
}

func certsChanged(p Profile) bool {
	files := []struct {
		name    string
		content string
	}{
		{"ca.pem", p.CACert},
		{"client.pem", p.ClientCert},
		{"client-key.pem", p.ClientKey.Reveal()},
	}
	for _, f := range files {
		if f.content == "" {
			continue
		}
		existing, err := readFile(filepath.Join(p.CertDir, f.name))
		if err != nil || string(existing) != f.content {
			return true
		}
	}
	return false
}
