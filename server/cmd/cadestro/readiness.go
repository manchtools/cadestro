package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/manchtools/cadestro/server/internal/backupstatus"
)

type readinessStore interface {
	Ping(context.Context) error
}

func checkReadiness(
	ctx context.Context,
	st readinessStore,
	artifactPath, backupPath string, backupMaxLag time.Duration,
) error {
	if ctx == nil || st == nil {
		return errors.New("readiness dependencies are required")
	}
	if err := st.Ping(ctx); err != nil {
		return fmt.Errorf("database: %w", err)
	}
	if err := validateWritableDirectory("artifact path", artifactPath); err != nil {
		return fmt.Errorf("artifact path: %w", err)
	}

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
