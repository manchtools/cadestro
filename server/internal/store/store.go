package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/manchtools/cadestro/server/internal/store/generated"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

const migrationsDir = "migrations"

type Tx struct {
	*generated.Queries
	raw *sql.Tx
}

type Store struct {
	now     func() time.Time
	db      *sql.DB
	queries *generated.Queries

	writeMu sync.Mutex

	wireMu sync.RWMutex

	logger *slog.Logger
}

func (s *Store) SetLogger(logger *slog.Logger) {
	s.wireMu.Lock()
	s.logger = logger
	s.wireMu.Unlock()
}

const sqliteOpenConnections = 10

func New(ctx context.Context, path string) (*Store, error) {
	db, err := openSQLite(ctx, path, true)
	if err != nil {
		return nil, err
	}
	if err := migrateSQLite(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return newStore(db), nil
}

func NewWithoutMigrations(ctx context.Context, path string) (*Store, error) {
	db, err := openSQLite(ctx, path, false)
	if err != nil {
		return nil, err
	}
	if err := verifySQLiteMigrated(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return newStore(db), nil
}

func openSQLite(ctx context.Context, path string, create bool) (*sql.DB, error) {
	if ctx == nil || strings.TrimSpace(path) == "" {
		return nil, errors.New("open SQLite database: path is required")
	}
	if err := prepareSQLiteFile(path, create); err != nil {
		return nil, err
	}
	dsn, err := sqliteDSN(path)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	db.SetMaxOpenConns(sqliteOpenConnections)
	db.SetMaxIdleConns(sqliteOpenConnections)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping SQLite database: %w", err)
	}
	return db, nil
}

func prepareSQLiteFile(path string, create bool) error {
	if path == ":memory:" || strings.HasPrefix(path, "file:") {
		return nil
	}
	flags := os.O_RDWR
	if create {
		flags |= os.O_CREATE
	}
	file, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		return fmt.Errorf("open SQLite database file: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("secure SQLite database file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close SQLite database file: %w", err)
	}
	return nil
}

func sqliteDSN(path string) (string, error) {
	var base string
	if path == ":memory:" {
		base = "file:cadestro?mode=memory&cache=shared"
	} else if strings.HasPrefix(path, "file:") {
		base = path
	} else {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("resolve SQLite path: %w", err)
		}
		base = (&url.URL{Scheme: "file", Path: absolute}).String()
	}
	separator := "?"
	if strings.Contains(base, "?") {
		separator = "&"
	}
	return base + separator +
		"_pragma=busy_timeout%285000%29" +
		"&_pragma=foreign_keys%281%29" +
		"&_pragma=journal_mode%28WAL%29" +
		"&_pragma=synchronous%28FULL%29" +
		"&_time_format=sqlite", nil
}

func configureGoose() error {
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("configure SQLite migration dialect: %w", err)
	}
	goose.SetBaseFS(migrationsFS)
	goose.SetLogger(goose.NopLogger())
	return nil
}

func migrateSQLite(ctx context.Context, db *sql.DB) error {
	if err := configureGoose(); err != nil {
		return err
	}
	if err := goose.UpContext(ctx, db, migrationsDir); err != nil {
		return fmt.Errorf("apply SQLite migrations: %w", err)
	}
	return nil
}

func verifySQLiteMigrated(ctx context.Context, db *sql.DB) error {
	if err := configureGoose(); err != nil {
		return err
	}
	migrations, err := goose.CollectMigrations(migrationsDir, 0, goose.MaxVersion)
	if err != nil {
		return fmt.Errorf("collect SQLite migrations: %w", err)
	}
	var want int64
	if len(migrations) > 0 {
		want = migrations[len(migrations)-1].Version
	}
	got, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		return fmt.Errorf("read SQLite migration version: %w", err)
	}
	if got != want {
		return fmt.Errorf("open SQLite database: schema version is %d, want %d", got, want)
	}
	return nil
}

func newStore(db *sql.DB) *Store {
	return &Store{
		now:     time.Now,
		db:      db,
		queries: generated.New(db),
	}
}

func (s *Store) Close() {
	_ = s.db.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Store) withTx(ctx context.Context, fn func(*sql.Tx, *generated.Queries) error) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := fn(tx, s.queries.WithTx(tx)); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func (s *Store) clock() time.Time {
	s.wireMu.RLock()
	now := s.now
	s.wireMu.RUnlock()
	return now()
}
