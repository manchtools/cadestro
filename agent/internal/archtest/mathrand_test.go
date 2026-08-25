package archtest

import (
	"testing"
)

func TestNoMathRandForCrypto(t *testing.T) {
	root := moduleRoot(t)
	files := walkGoFiles(t, root, func(string) bool { return true })
	if len(files) == 0 {
		t.Fatal("matches-zero guard: walked zero production Go files — detector is mis-scoped")
	}

	allow := newAllowlist(map[string]string{
		"cmd/cadestrod/backend.go :: math/rand/v2": "connection backoff jitter (rand.Int64N) — non-cryptographic; IDs use ulidx and secret material uses crypto/rand",
	})

	for _, gf := range files {
		for _, imp := range gf.ast.Imports {
			p := unquoteLit(imp.Path)
			if p != "math/rand" && p != "math/rand/v2" {
				continue
			}
			if allow.exempt(gf.rel + " :: " + p) {
				continue
			}
			t.Errorf("%s imports %s at %s:%d — math/rand is not cryptographically secure; use crypto/rand for nonces/keys/challenges and ulidx for IDs. If this is a non-crypto use (jitter/backoff/load-balancing), allowlist the file with a justification.",
				gf.rel, p, gf.rel, gf.line(imp))
		}
	}
	allow.assertNoStale(t)
}
