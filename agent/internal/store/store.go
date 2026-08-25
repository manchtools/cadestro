package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/robfig/cron/v3"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"

	"github.com/manchtools/cadestro/agent/internal/store/generated"
	"github.com/manchtools/cadestro/agent/internal/store/migrations"
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

const (
	nilScheduleDrift  = 8 * time.Hour
	binaryProtoPrefix = byte(0x00)
)

type Store struct {
	db      *sql.DB
	queries *generated.Queries
	mu      sync.RWMutex
	now     func() time.Time
}

type StoredAction struct {
	ID             string
	Action         *pb.Action
	AssignedAt     time.Time
	LastExecutedAt *time.Time
	NextExecuteAt  time.Time
}

func New(dataDir string) (*Store, error) { return open(dataDir, true) }

func OpenExisting(dataDir string) (*Store, error) {
	dbPath := filepath.Join(dataDir, "agent.db")
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("agent database %s does not exist — start the agent service first", dbPath)
		}
		return nil, fmt.Errorf("stat agent database: %w", err)
	}
	return open(dataDir, false)
}

func open(dataDir string, migrate bool) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	dbPath := filepath.Join(dataDir, "agent.db")
	if strings.ContainsAny(dbPath, "?#") {
		return nil, fmt.Errorf("store: data dir path %q contains '?' or '#', which would corrupt SQLite DSN pragmas", dbPath)
	}
	dsn := dbPath + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	closeOnError := func(err error) (*Store, error) {
		_ = db.Close()
		return nil, err
	}
	if migrate {
		goose.SetBaseFS(migrations.FS)
		if err := goose.SetDialect("sqlite3"); err != nil {
			return closeOnError(fmt.Errorf("set goose dialect: %w", err))
		}
		if err := goose.Up(db, "."); err != nil {
			return closeOnError(fmt.Errorf("run migrations: %w", err))
		}
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
		return fmt.Errorf("data dir %s is %#o after tightening; refusing to store secrets there", dir, perm)
	}
	return nil
}

func (s *Store) SetClockForTest(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = now
}

func (s *Store) Close() error { return s.db.Close() }

func canonicalProtoBytes(message proto.Message) ([]byte, error) {
	return proto.MarshalOptions{Deterministic: true}.Marshal(message)
}

func marshalStoredProto(message proto.Message) ([]byte, error) {
	encoded, err := canonicalProtoBytes(message)
	if err != nil {
		return nil, err
	}
	return append([]byte{binaryProtoPrefix}, encoded...), nil
}

func unmarshalStoredProto(raw []byte, message proto.Message) error {
	if len(raw) == 0 || raw[0] != binaryProtoPrefix {
		return errors.New("stored blob is not binary protobuf")
	}
	return proto.Unmarshal(raw[1:], message)
}

func calculateNextExecuteFromSchedule(schedule *pb.ActionSchedule, lastExecuted *time.Time, runImmediately bool, now time.Time) time.Time {
	now = now.UTC()
	if runImmediately && lastExecuted == nil {
		return now
	}
	if schedule == nil {
		if lastExecuted == nil {
			return now
		}
		return clampInterval(lastExecuted.UTC().Add(nilScheduleDrift), now, nilScheduleDrift)
	}
	if schedule.GetRunOnAssign() && lastExecuted == nil {
		return now
	}
	if schedule.GetCron() != "" {
		scheduleParser, err := cronParser.Parse(schedule.GetCron())
		if err == nil {
			return scheduleParser.Next(now.Local()).UTC()
		}
		slog.Warn("invalid manifest cron expression; using interval fallback", "cron", schedule.GetCron(), "error", err)
	}
	intervalHours := schedule.GetIntervalHours()
	if intervalHours <= 0 {
		intervalHours = 8
	}
	if lastExecuted == nil {
		return now
	}
	interval := time.Duration(intervalHours) * time.Hour
	return clampInterval(lastExecuted.UTC().Add(interval), now, interval)
}

func clampInterval(computed, now time.Time, interval time.Duration) time.Time {
	ceiling := now.UTC().Add(interval)
	if computed.After(ceiling) {
		return ceiling
	}
	return computed
}

type LuksState struct {
	ActionID       string
	DevicePath     string
	OwnershipTaken bool
	DeviceKeyType  string
	LastRotatedAt  time.Time
}

func (s *Store) GetLuksState(ctx context.Context, actionID string) (*LuksState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	row, err := s.queries.GetLuksState(ctx, actionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	state := LuksState{
		ActionID:       row.ActionID,
		DevicePath:     row.DevicePath,
		OwnershipTaken: row.OwnershipTaken,
		DeviceKeyType:  row.DeviceKeyType,
	}
	if row.LastRotatedAt != "" {
		state.LastRotatedAt, err = time.Parse(time.RFC3339, row.LastRotatedAt)
		if err != nil {
			return nil, fmt.Errorf("parse LUKS last_rotated_at: %w", err)
		}
	}
	return &state, nil
}

func (s *Store) SetLuksOwnershipTaken(ctx context.Context, actionID, devicePath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queries.SetLuksOwnershipTaken(ctx, generated.SetLuksOwnershipTakenParams{
		ActionID: actionID, DevicePath: devicePath, LastRotatedAt: s.now().UTC().Format(time.RFC3339),
	})
}

func (s *Store) SetLuksDeviceKeyType(ctx context.Context, actionID, keyType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queries.SetLuksDeviceKeyType(ctx, generated.SetLuksDeviceKeyTypeParams{DeviceKeyType: keyType, ActionID: actionID})
}

func (s *Store) SetLuksLastRotatedAt(ctx context.Context, actionID string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queries.SetLuksLastRotatedAt(ctx, generated.SetLuksLastRotatedAtParams{LastRotatedAt: at.UTC().Format(time.RFC3339), ActionID: actionID})
}

func (s *Store) DeleteLuksState(ctx context.Context, actionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queries.DeleteLuksState(ctx, actionID)
}

func (s *Store) GetLuksPassphraseHashes(ctx context.Context, actionID string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.queries.GetLuksPassphraseHashes(ctx, actionID)
}

func (s *Store) AddLuksPassphraseHash(ctx context.Context, actionID, hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	if err := queries.AddLuksPassphraseHash(ctx, generated.AddLuksPassphraseHashParams{ActionID: actionID, PassphraseHash: hash}); err != nil {
		return err
	}
	if err := queries.PruneLuksPassphraseHashes(ctx, generated.PruneLuksPassphraseHashesParams{ActionID: actionID, ActionID_2: actionID}); err != nil {
		return err
	}
	return tx.Commit()
}

type LpsUserState struct {
	ActionID      string
	Username      string
	LastRotatedAt time.Time
	PasswordHash  string
}

func (s *Store) GetLpsState(ctx context.Context, actionID string) (map[string]*LpsUserState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.queries.GetLpsState(ctx, actionID)
	if err != nil {
		return nil, err
	}
	users := make(map[string]*LpsUserState)
	for _, row := range rows {
		state := LpsUserState{ActionID: row.ActionID, Username: row.Username, PasswordHash: row.PasswordHash}
		if row.LastRotatedAt != "" {
			state.LastRotatedAt, err = time.Parse(time.RFC3339, row.LastRotatedAt)
			if err != nil {
				return nil, fmt.Errorf("parse LPS last_rotated_at for %s: %w", state.Username, err)
			}
		}
		users[state.Username] = &state
	}
	return users, nil
}

func (s *Store) SetLpsUserState(ctx context.Context, actionID, username string, lastRotatedAt time.Time, passwordHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queries.SetLpsUserState(ctx, generated.SetLpsUserStateParams{
		ActionID: actionID, Username: username, LastRotatedAt: lastRotatedAt.UTC().Format(time.RFC3339), PasswordHash: passwordHash,
	})
}

func (s *Store) DeleteLpsState(ctx context.Context, actionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queries.DeleteLpsState(ctx, actionID)
}

func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, err := s.queries.GetSetting(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queries.SetSetting(ctx, generated.SetSettingParams{Key: key, Value: value})
}

func (s *Store) DeleteSetting(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queries.DeleteSetting(ctx, key)
}
