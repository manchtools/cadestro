package main

import (
	"errors"
	"math/rand/v2"
	"time"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

func randomBackoff() time.Duration {
	span := int64(maxInitialBackoff - minInitialBackoff)
	if span <= 0 {
		return minInitialBackoff
	}
	return minInitialBackoff + time.Duration(rand.Int64N(span))
}

func rootBackend(euid int) (sysexec.PrivilegeBackend, error) {
	if euid != 0 {
		return sysexec.Direct, errors.New("root is required")
	}
	return sysexec.Direct, nil
}
