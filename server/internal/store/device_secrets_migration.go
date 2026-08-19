package store

import (
	"context"
	"database/sql"
	"fmt"

	pmcrypto "github.com/manchtools/cadestro/server/internal/crypto"
	"github.com/oklog/ulid/v2"
)

// MigrateDeviceSecretRows copies schema-v1 LUKS/LPS ciphertext into the
// generic device-owned table. Every legacy row is opened and re-encrypted
// inside one transaction; a bad row rolls back both the new rows and the
// metadata-table rebuild.
func (s *Store) MigrateDeviceSecretRows(ctx context.Context, atRest *pmcrypto.Encryptor) error {
	if s == nil || atRest == nil {
		return fmt.Errorf("device secret migration: store and encryption key are required")
	}
	_, err := s.WithAudit(ctx, AuditOperation{
		OperationID: ulid.Make().String(), Class: ClassMutation, ActorType: "system",
		Origin: "manual_cutover", RequestDescriptor: "device-secret-schema-v1",
		AuthorizationOutcome: AuthorizationAllowed, AuthorizationDetail: "offline_migration", Result: ResultSuccess, ResultCode: "OK",
	}, func(ctx context.Context, tx *Tx, rec *AuditRecorder) error {
		type row struct {
			id, device, action, kind, value string
		}
		rows := make([]row, 0)
		for _, source := range []struct{ table, column, kind string }{
			{"luks_keys", "passphrase", "luks"}, {"lps_passwords", "password", "lps"},
		} {
			query := fmt.Sprintf("SELECT id, device_id, action_id, %s FROM %s", source.column, source.table)
			result, err := tx.raw.QueryContext(ctx, query)
			if err != nil {
				return fmt.Errorf("read legacy %s rows: %w", source.kind, err)
			}
			for result.Next() {
				var item row
				if err := result.Scan(&item.id, &item.device, &item.action, &item.value); err != nil {
					_ = result.Close()
					return fmt.Errorf("read legacy %s row: %w", source.kind, err)
				}
				item.kind = source.kind
				rows = append(rows, item)
			}
			if err := result.Err(); err != nil {
				_ = result.Close()
				return fmt.Errorf("read legacy %s rows: %w", source.kind, err)
			}
			_ = result.Close()
		}
		for _, item := range rows {
			plaintext, err := atRest.DecryptWithContext(item.value, pmcrypto.LegacySecretAADForRow(item.device, item.action, item.kind, item.id))
			if err != nil {
				return fmt.Errorf("decrypt legacy %s row %s: %w", item.kind, item.id, err)
			}
			ciphertext, err := atRest.EncryptWithContext(plaintext, pmcrypto.DeviceSecretAAD(item.id, item.device, item.kind, item.action, 1))
			if err != nil {
				return fmt.Errorf("encrypt migrated %s row %s: %w", item.kind, item.id, err)
			}
			if _, err := tx.raw.ExecContext(ctx, `INSERT INTO device_secrets (id, device_id, kind, subject, version, ciphertext) VALUES (?, ?, ?, ?, 1, ?)`, item.id, item.device, item.kind, item.action, ciphertext); err != nil {
				return fmt.Errorf("insert migrated %s row %s: %w", item.kind, item.id, err)
			}
			rec.Effect(AuditEffect{ResourceType: "device_secret", ResourceID: item.id, Action: "MIGRATE", Outcome: EffectApplied})
		}
		// Only after every legacy value has authenticated and every generic row
		// has been written do we remove the NOT NULL legacy secret columns. The
		// rebuild is part of this transaction, so any DDL failure rolls back the
		// inserts and leaves the legacy tables untouched.
		if err := removeLegacySecretColumns(ctx, tx.raw); err != nil {
			return err
		}
		return nil
	})
	return err
}

func removeLegacySecretColumns(ctx context.Context, tx *sql.Tx) error {
	statements := []string{
		`DROP INDEX idx_lps_passwords_device`,
		`DROP INDEX idx_lps_passwords_action_device`,
		`DROP INDEX idx_lps_passwords_username`,
		`ALTER TABLE lps_passwords RENAME TO lps_passwords_legacy`,
		`CREATE TABLE lps_passwords (
            id text PRIMARY KEY REFERENCES device_secrets(id) ON DELETE CASCADE,
            username text NOT NULL,
            rotated_at timestamp NOT NULL,
            rotation_reason text NOT NULL DEFAULT 'scheduled',
            is_current boolean NOT NULL DEFAULT true,
            created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP
        )`,
		`INSERT INTO lps_passwords (id, username, rotated_at, rotation_reason, is_current, created_at)
		 SELECT id, username, rotated_at, rotation_reason, is_current, created_at FROM lps_passwords_legacy`,
		`DROP TABLE lps_passwords_legacy`,
		`DROP INDEX idx_luks_keys_device`,
		`DROP INDEX idx_luks_keys_action_device`,
		`DROP INDEX idx_luks_keys_current`,
		`ALTER TABLE luks_keys RENAME TO luks_keys_legacy`,
		`CREATE TABLE luks_keys (
            id text PRIMARY KEY REFERENCES device_secrets(id) ON DELETE CASCADE,
            device_path text NOT NULL,
            rotated_at timestamp NOT NULL,
            rotation_reason text NOT NULL DEFAULT 'scheduled',
            is_current boolean NOT NULL DEFAULT true,
            created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
            revocation_status text,
            revocation_error text,
            revocation_at timestamp
        )`,
		`INSERT INTO luks_keys (id, device_path, rotated_at, rotation_reason, is_current, created_at, revocation_status, revocation_error, revocation_at)
		 SELECT id, device_path, rotated_at, rotation_reason, is_current, created_at, revocation_status, revocation_error, revocation_at FROM luks_keys_legacy`,
		`DROP TABLE luks_keys_legacy`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("rebuild metadata tables: %w", err)
		}
	}
	return nil
}
