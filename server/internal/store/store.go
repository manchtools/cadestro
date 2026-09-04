package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pressly/goose/v3"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/manchtools/cadestro/server/internal/store/generated"
	"github.com/manchtools/cadestro/server/internal/store/migrations"
)

type Store struct {
	db      *sql.DB
	queries *generated.Queries
}

func New(ctx context.Context, path string) (*Store, error) {
	if path == "" || !filepath.IsAbs(path) || strings.ContainsAny(path, "?#") {
		return nil, errors.New("database path must be an absolute SQLite file path without '?' or '#'")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	closeOnError := func(err error) (*Store, error) {
		_ = db.Close()
		return nil, err
	}
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return closeOnError(fmt.Errorf("set migration dialect: %w", err))
	}
	if err := goose.UpContext(ctx, db, "."); err != nil {
		return closeOnError(fmt.Errorf("apply migrations: %w", err))
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return closeOnError(fmt.Errorf("restrict database permissions: %w", err))
	}
	return &Store{db: db, queries: generated.New(db)}, nil
}

func (store *Store) Close() error { return store.db.Close() }

func (store *Store) Queries() *generated.Queries { return store.queries }

func (store *Store) Transaction(ctx context.Context, run func(*generated.Queries) error) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	if err := run(store.queries.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit()
}

func IsNotFound(err error) bool { return errors.Is(err, sql.ErrNoRows) }

func IsConflict(err error) bool {
	var sqliteError *sqlite.Error
	if !errors.As(err, &sqliteError) {
		return false
	}
	return sqliteError.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE || sqliteError.Code() == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY
}
