package main

import (
	"context"
	"fmt"
	"os"

	pmcrypto "github.com/manchtools/cadestro/server/internal/crypto"
	"github.com/manchtools/cadestro/server/internal/store"
)

// runDeviceSecretMigration is deliberately a separate, stopped-control
// command. The live server never creates device_secrets or rewrites legacy
// tables implicitly; operators validate the result before starting it again.
func runDeviceSecretMigration(ctx context.Context) int {
	databasePath := os.Getenv("CADESTRO_DATABASE_PATH")
	keyHex, _, keyErr := loadSecret(
		"CADESTRO_ENCRYPTION_KEY", os.Getenv("CADESTRO_ENCRYPTION_KEY"),
		"CADESTRO_ENCRYPTION_KEY_FILE", os.Getenv("CADESTRO_ENCRYPTION_KEY_FILE"))
	if databasePath == "" || keyErr != nil {
		if keyErr != nil {
			fmt.Fprintln(os.Stderr, "cadestro: encryption key:", keyErr)
		} else {
			fmt.Fprintln(os.Stderr, "cadestro: migrate-device-secrets requires CADESTRO_DATABASE_PATH")
		}
		return 2
	}
	legacy, err := pmcrypto.NewEncryptor(keyHex)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cadestro: invalid encryption key:", err)
		return 2
	}
	target, err := pmcrypto.NewEncryptor(keyHex)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cadestro: invalid encryption key:", err)
		return 2
	}
	st, err := store.NewWithoutMigrations(ctx, databasePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cadestro: open database:", err)
		return 1
	}
	defer st.Close()
	if err := st.MigrateDeviceSecretRows(ctx, legacy, target); err != nil {
		fmt.Fprintln(os.Stderr, "cadestro: migrate device secrets:", err)
		return 1
	}
	fmt.Fprintln(os.Stdout, "cadestro: device secret migration completed; validate before starting control")
	return 0
}
