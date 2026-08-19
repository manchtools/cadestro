package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/manchtools/cadestro/server/internal/backupstatus"
)

const readinessFingerprint = "0000000000000000000000000000000000000000000000000000000000000000"

type readinessStore interface {
	Ping(context.Context) error
}

type readinessRevocationChecker interface {
	IsRevoked(context.Context, string) (bool, error)
}

// checkReadiness verifies the dependencies whose availability can change
// after startup. Schema and key material are validated while control starts.
func checkReadiness(
	ctx context.Context,
	st readinessStore,
	revocations readinessRevocationChecker,
	artifactPath, backupPath string,
	backupMaxLag time.Duration,
) error {
	if ctx == nil || st == nil || revocations == nil {
		return errors.New("readiness dependencies are required")
	}
	if err := st.Ping(ctx); err != nil {
		return fmt.Errorf("database: %w", err)
	}
	if _, err := revocations.IsRevoked(ctx, readinessFingerprint); err != nil {
		return fmt.Errorf("revocation enforcement: %w", err)
	}
	if err := validateWritableDirectory("artifact path", artifactPath); err != nil {
		return fmt.Errorf("artifact path: %w", err)
	}
	// Empty path or non-positive lag is the explicit disabled/unconfigured
	// policy; readiness must not manufacture a backup failure in that mode.
	if backupPath == "" || backupMaxLag <= 0 {
		return nil
	}
	status, err := backupstatus.Read(backupPath, time.Now().UTC(), backupMaxLag)
	if err != nil {
		return fmt.Errorf("backup status: %w", err)
	}
	if status.Stale {
		return errors.New("backup status is stale")
	}
	return nil
}
