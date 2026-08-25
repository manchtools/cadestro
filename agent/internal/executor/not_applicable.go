package executor

import (
	"errors"
	"fmt"

	"github.com/manchtools/cadestro/sdk/pkg"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

var errNotApplicable = errors.New("not applicable to this device")

func notApplicable(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errNotApplicable, fmt.Sprintf(format, args...))
}

func securityOnlyNotApplicable(securityOnly bool, upgradeErr, lastErr error) bool {
	return securityOnly && upgradeErr != nil && lastErr == upgradeErr &&
		(errors.Is(upgradeErr, pkg.ErrUnsupported) ||
			errors.Is(upgradeErr, sysexec.ErrBackendUnavailable))
}
