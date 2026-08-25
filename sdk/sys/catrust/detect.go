package catrust

import (
	"context"
	"os"
	osexec "os/exec"
)

var lookPath = osexec.LookPath

var anchorsDirExists = func(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// Detect reports the trust-store backends usable on THIS host: CaCertificates
// when update-ca-certificates is on PATH, P11Kit when update-ca-trust is. It
// lists; the consumer picks one and passes it to New.
//
// The ctx is accepted for signature uniformity; the probe is a pure PATH lookup.
func Detect(ctx context.Context) []Backend {
	_ = ctx
	var out []Backend
	if _, err := lookPath("update-ca-certificates"); err == nil {

		if anchorsDirExists(backends[SuseCaCertificates].anchorsDirs[0]) {
			out = append(out, SuseCaCertificates)
		} else {
			out = append(out, CaCertificates)
		}
	}
	if _, err := lookPath("update-ca-trust"); err == nil {
		out = append(out, P11Kit)
	}
	return out
}
