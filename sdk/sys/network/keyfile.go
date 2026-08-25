package network

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/manchtools/cadestro/sdk/sys/exec"
)

var nmKeyfileDir = "/etc/NetworkManager/system-connections"

func keyfilePath(name string) string {
	safe := strings.ReplaceAll(name, string(filepath.Separator), "_")
	return filepath.Join(nmKeyfileDir, safe+".nmconnection")
}

func validatePSK(psk exec.Secret) error {
	v := psk.Reveal()
	n := len(v)

	if n == 64 && isHex(v) {
		return nil
	}
	if n < 8 || n > 63 {
		return fmt.Errorf("invalid PSK: a WPA pre-shared key must be 8–63 characters (or a 64-hex-digit raw PMK)")
	}
	for i := 0; i < n; i++ {
		c := v[i]
		if c < 0x20 || c > 0x7e {
			return fmt.Errorf("invalid PSK: must contain only printable ASCII characters")
		}
	}
	return nil
}

func isHex(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		isDigit := c >= '0' && c <= '9'
		isLower := c >= 'a' && c <= 'f'
		isUpper := c >= 'A' && c <= 'F'
		if !isDigit && !isLower && !isUpper {
			return false
		}
	}
	return true
}

func buildPSKKeyfile(p Profile) []byte {
	var b strings.Builder

	b.WriteString("# Managed by cadestrod — do not edit by hand.\n")
	b.WriteString("[connection]\n")
	fmt.Fprintf(&b, "id=%s\n", p.Name)
	b.WriteString("type=wifi\n")
	if p.AutoConnect {
		b.WriteString("autoconnect=true\n")
	} else {
		b.WriteString("autoconnect=false\n")
	}
	fmt.Fprintf(&b, "autoconnect-priority=%d\n", p.Priority)
	b.WriteString("\n")

	b.WriteString("[wifi]\n")
	fmt.Fprintf(&b, "ssid=%s\n", p.SSID)
	b.WriteString("mode=infrastructure\n")
	if p.Hidden {
		b.WriteString("hidden=true\n")
	}
	b.WriteString("\n")

	b.WriteString("[wifi-security]\n")
	b.WriteString("key-mgmt=wpa-psk\n")
	fmt.Fprintf(&b, "psk=%s\n", p.PSK.Reveal())
	b.WriteString("\n")

	b.WriteString("[ipv4]\n")
	b.WriteString("method=auto\n")
	b.WriteString("\n")
	b.WriteString("[ipv6]\n")
	b.WriteString("method=auto\n")

	return []byte(b.String())
}

func writeKeyfile(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := mkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create keyfile dir %q: %w", dir, err)
	}
	tmp, err := createTemp(dir, ".cadestro-keyfile-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp keyfile in %q: %w", dir, err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return cleanupTempKeyfile(tmpPath, fmt.Errorf("write keyfile: %w", err))
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return cleanupTempKeyfile(tmpPath, fmt.Errorf("chmod keyfile: %w", err))
	}
	if err := tmp.Close(); err != nil {
		return cleanupTempKeyfile(tmpPath, fmt.Errorf("close keyfile: %w", err))
	}
	if err := renameFile(tmpPath, path); err != nil {
		return cleanupTempKeyfile(tmpPath, fmt.Errorf("rename keyfile to %q: %w", path, err))
	}
	return nil
}

func cleanupTempKeyfile(tmpPath string, cause error) error {
	if rmErr := removeFile(tmpPath); rmErr != nil && !os.IsNotExist(rmErr) {
		return fmt.Errorf("%w (temp keyfile cleanup failed, plaintext secret may remain at %s: %v)", cause, tmpPath, rmErr)
	}
	return cause
}
