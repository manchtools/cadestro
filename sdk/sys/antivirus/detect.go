package antivirus

import (
	"context"
	osexec "os/exec"
)

var lookPath = osexec.LookPath

// Detect reports the AV backends usable on THIS host: ClamAV when clamscan is on
// PATH. It lists; the consumer picks one and passes it to New.
//
// The ctx is accepted for signature uniformity; the probe is a pure PATH lookup.
func Detect(ctx context.Context) []Backend {
	_ = ctx
	if _, err := lookPath("clamscan"); err != nil {
		return nil
	}
	return []Backend{ClamAV}
}
