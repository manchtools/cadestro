package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/manchtools/cadestro/agent/internal/store/generated"
	"github.com/manchtools/cadestro/agent/internal/store/migrations"
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

const (
	defaultInterval = 8 * time.Hour
)

type Store struct {
	db      *sql.DB
	queries *generated.Queries
	mu      sync.RWMutex
	now     func() time.Time
}

func New(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	dbPath := filepath.Join(dataDir, "agent.db")
	if strings.ContainsAny(dbPath, "?#") {
		return nil, fmt.Errorf("store: data dir path %q contains '?' or '#', which would corrupt SQLite DSN pragmas", dbPath)
	}
	dsn := dbPath + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)"
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
		return closeOnError(fmt.Errorf("set goose dialect: %w", err))
	}
	if err := goose.Up(db, "."); err != nil {
		return closeOnError(fmt.Errorf("run migrations: %w", err))
	}
	if err := os.Chmod(dataDir, 0o700); err != nil {
		return closeOnError(fmt.Errorf("restrict data dir mode: %w", err))
	}
	if err := verifyRestrictiveDirMode(dataDir); err != nil {
		return closeOnError(err)
	}
	for _, path := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		if err := os.Chmod(path, 0o600); err != nil && !os.IsNotExist(err) {
			return closeOnError(fmt.Errorf("restrict %s mode: %w", filepath.Base(path), err))
		}
	}
	return &Store{db: db, queries: generated.New(db), now: time.Now}, nil
}

func verifyRestrictiveDirMode(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("stat data dir after chmod: %w", err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		return fmt.Errorf("data dir %s is %#o after tightening; refusing to store state there", dir, perm)
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

func calculateNextExecuteFromSchedule(schedule *pb.ActionSchedule, lastExecuted *time.Time, now time.Time) time.Time {
	now = now.UTC()
	interval := defaultInterval
	if hours := schedule.GetIntervalHours(); hours > 0 {
		interval = time.Duration(hours) * time.Hour
	}
	if lastExecuted == nil {
		return now
	}
	return clampInterval(lastExecuted.UTC().Add(interval), now, interval)
}

func clampInterval(computed, now time.Time, interval time.Duration) time.Time {
	ceiling := now.UTC().Add(interval)
	if computed.After(ceiling) {
		return ceiling
	}
	return computed
}
