package archtest

import (
	"go/ast"
	"strings"
	"testing"
)

var revealSinkAllowlist = map[string]string{
	"sys/user/password.go :: password.Reveal()":    "chpasswd stdin: the sole sink that writes a user password to useradd's helper",
	"sys/encryption/luks.go :: key.Reveal()":       "LUKS key file in /dev/shm: cryptsetup --key-file sink (never argv)",
	"sys/encryption/tpm.go :: key.Reveal()":        "systemd-cryptenroll stdin: the TPM-enrollment passphrase sink (never argv)",
	"sys/network/keyfile.go :: p.PSK.Reveal()":     "NetworkManager keyfile [wifi-security] psk= line (0600 file, never argv)",
	"sys/network/keyfile.go :: psk.Reveal()":       "validatePSK WPA2 length/charset gate before the keyfile write — same secret the psk= sink writes, never logged (errors omit the value) or argv",
	"sys/network/certs.go :: p.ClientKey.Reveal()": "EAP-TLS client-key.pem (0600 file write + on-disk drift compare, never argv)",
}

func TestRevealOnlyFromKnownSinks(t *testing.T) {
	root := moduleRoot(t)

	files := walkGoFiles(t, root, func(rel string) bool {
		if strings.HasPrefix(rel, "gen/") || strings.HasPrefix(rel, "archtest/") {
			return false
		}
		if rel == "sys/exec/secret.go" {
			return false
		}
		return strings.HasPrefix(rel, "sys/") || strings.HasPrefix(rel, "pkg/")
	})
	if len(files) == 0 {
		t.Fatal("matches-zero guard: walked zero capability Go files — detector is mis-scoped")
	}

	allow := newAllowlist(revealSinkAllowlist)
	sawReveal := false

	for _, gf := range files {
		ast.Inspect(gf.ast, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Reveal" || len(call.Args) != 0 {
				return true
			}
			sawReveal = true
			key := gf.rel + " :: " + render(gf.fset, call)
			if allow.exempt(key) {
				return true
			}
			t.Errorf("unsanctioned Secret.Reveal() at %s:%d — %s\n  Reveal() exposes credential plaintext and must appear only at an enumerated OS sink. If this is a real new sink, add a justified, guarded entry to revealSinkAllowlist; otherwise pass the Secret itself (it redacts in logs/format).",
				gf.rel, gf.line(call), render(gf.fset, call))
			return true
		})
	}

	if !sawReveal {
		t.Fatal("matches-zero guard: found no Reveal() calls anywhere — the detector is dead (or every sink was removed); the guard would pass vacuously")
	}
	allow.assertNoStale(t)
}
