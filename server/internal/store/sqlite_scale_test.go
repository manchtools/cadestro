package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/agentsync"
	"github.com/manchtools/cadestro/server/internal/connection"
	"github.com/manchtools/cadestro/server/internal/crypto"
	"github.com/manchtools/cadestro/server/internal/jobs"
	"github.com/manchtools/cadestro/server/internal/store"
	"github.com/manchtools/cadestro/server/internal/testdb"
)

const (
	scaleAgents          = 10_000
	scaleTerminalFrames  = 50_000
	scaleHeartbeatRounds = 5
	scaleSearches        = 100
	scaleWorkers         = 16
	scaleQueueSize       = 1_024
)

type latencySummary struct {
	P50Millis float64 `json:"p50_ms"`
	P95Millis float64 `json:"p95_ms"`
	P99Millis float64 `json:"p99_ms"`
	MaxMillis float64 `json:"max_ms"`
}

type sqliteScaleResult struct {
	Agents                 int            `json:"agents"`
	ElapsedSeconds         float64        `json:"elapsed_seconds"`
	RegistrationMillis     float64        `json:"registration_ms"`
	HeartbeatFlush         latencySummary `json:"heartbeat_flush"`
	Sync                   latencySummary `json:"sync"`
	Search                 latencySummary `json:"search"`
	TerminalRoute          latencySummary `json:"terminal_route"`
	BackupDurationMillis   float64        `json:"backup_duration_ms"`
	BackupLagSeconds       float64        `json:"backup_lag_seconds"`
	AgentRegistryHeapBytes uint64         `json:"agent_registry_heap_growth_bytes"`
	RegistryBytesPerAgent  uint64         `json:"agent_registry_bytes_per_agent"`
	PeakHeapGrowthBytes    uint64         `json:"peak_heap_growth_bytes"`
	PeakGoroutines         int64          `json:"peak_goroutines"`
	JobQueueAccepted       int            `json:"job_queue_accepted"`
	JobQueueDropped        int            `json:"job_queue_dropped"`
}

// TestSQLiteScale_MixedWorkloadAtTenThousandAgents is the repeatable target
// database gate. Normal suites skip it; run explicitly with
// CADESTRO_RUN_SCALE_TEST=1.
func TestSQLiteScale_MixedWorkloadAtTenThousandAgents(t *testing.T) {
	if os.Getenv("CADESTRO_RUN_SCALE_TEST") != "1" {
		t.Skip("set CADESTRO_RUN_SCALE_TEST=1 to run the 10,000-agent SQLite gate")
	}
	if testing.Short() {
		t.Fatal("the explicit scale gate cannot run in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	st, raw := setupSQLitePool(t, 32)
	now := time.Now().UTC()
	deviceIDs := seedScaleDevices(t, raw, now)
	// The bulk seed writes device rows directly. Production cannot: every device
	// mutation refreshes its search document inside the same audited
	// transaction. Rebuild once through the production path so the search
	// workload measures a real 10,000-document FTS5 corpus.
	rebuildSearchFixture(t, st)

	runtime.GC()
	var beforeAgents, afterAgents runtime.MemStats
	runtime.ReadMemStats(&beforeAgents)
	manager := connection.NewManager()
	registerStarted := time.Now()
	for index, id := range deviceIDs {
		manager.Register(ctx, id, fmt.Sprintf("scale-device-%05d", index), "scale", nil)
	}
	registrationDuration := time.Since(registerStarted)
	require.Equal(t, scaleAgents, manager.Count())
	snapshot := manager.LastSeenSnapshot()
	require.Len(t, snapshot, scaleAgents)
	runtime.ReadMemStats(&afterAgents)
	agentHeapGrowth := positiveDelta(afterAgents.Alloc, beforeAgents.Alloc)

	atRest, err := crypto.NewEncryptor("0101010101010101010101010101010101010101010101010101010101010101")
	require.NoError(t, err)
	syncer := agentsync.New(agentsync.Config{Store: st, Manager: manager, AtRest: atRest})
	jobState := jobs.New(jobs.Config{
		Store: st, LeaseDuration: 2 * time.Minute, RetryDelay: 30 * time.Second,
	})
	jobRunner := jobs.NewRunner(jobs.RunnerConfig{
		Store: st, State: jobState, QueueSize: scaleQueueSize, Workers: 1, BatchSize: 1,
		Handlers: map[string]jobs.Handler{"scale.noop": func(context.Context, jobs.Job) error { return nil }},
	})
	jobAccepted := fillJobQueue(jobRunner)

	terminalRegistry := connection.NewTerminalSessionRegistry()
	terminalSession := connection.NewTerminalSession(newID(), deviceIDs[0], newID(), "cadestro-tty-scale", 120, 40)
	terminalRegistry.Register(terminalSession)
	defer terminalRegistry.Unregister(terminalSession.SessionID)

	var peakGoroutines atomic.Int64
	peakGoroutines.Store(int64(runtime.NumGoroutine()))
	var currentMemory runtime.MemStats
	runtime.ReadMemStats(&currentMemory)
	var peakHeap atomic.Uint64
	peakHeap.Store(currentMemory.Alloc)
	stopSampler := make(chan struct{})
	var sampler sync.WaitGroup
	sampler.Add(1)
	go sampleRuntime(stopSampler, &sampler, &peakGoroutines, &peakHeap)

	start := make(chan struct{})
	workStarted := time.Now()
	backupDirectory := t.TempDir()
	type workloadResult struct {
		name      string
		latencies map[string][]time.Duration
		duration  time.Duration
		completed time.Time
		err       error
	}
	results := make(chan workloadResult, 5)
	go func() {
		<-start
		latencies, err := exerciseSync(ctx, syncer, deviceIDs)
		results <- workloadResult{name: "sync", latencies: latencies, err: err}
	}()
	go func() {
		<-start
		latencies, err := exerciseHeartbeats(ctx, st, snapshot)
		results <- workloadResult{name: "heartbeat", latencies: map[string][]time.Duration{"heartbeat": latencies}, err: err}
	}()
	go func() {
		<-start
		latencies, err := exerciseSearch(ctx, st)
		results <- workloadResult{name: "search", latencies: map[string][]time.Duration{"search": latencies}, err: err}
	}()
	go func() {
		<-start
		latencies, err := exerciseTerminal(ctx, terminalRegistry, terminalSession.SessionID)
		results <- workloadResult{name: "terminal", latencies: map[string][]time.Duration{"terminal": latencies}, err: err}
	}()
	go func() {
		<-start
		duration, completed, err := exerciseBackup(ctx, raw, backupDirectory)
		results <- workloadResult{name: "backup", duration: duration, completed: completed, err: err}
	}()
	close(start)

	latencies := make(map[string][]time.Duration)
	var backupDuration time.Duration
	var backupCompleted time.Time
	var workloadErr error
	for range 5 {
		result := <-results
		if result.err != nil && workloadErr == nil {
			workloadErr = fmt.Errorf("%s workload: %w", result.name, result.err)
		}
		for name, samples := range result.latencies {
			latencies[name] = samples
		}
		if result.name == "backup" {
			backupDuration, backupCompleted = result.duration, result.completed
		}
	}
	elapsed := time.Since(workStarted)
	close(stopSampler)
	sampler.Wait()
	require.NoError(t, workloadErr)

	assertScaleDatabaseState(t, ctx, raw, scaleAgents)
	backupLag := time.Since(backupCompleted)
	result := sqliteScaleResult{
		Agents:         scaleAgents,
		ElapsedSeconds: elapsed.Seconds(), RegistrationMillis: milliseconds(registrationDuration),
		HeartbeatFlush: summarizeLatency(latencies["heartbeat"]),
		Sync:           summarizeLatency(latencies["sync"]), Search: summarizeLatency(latencies["search"]),
		TerminalRoute:        summarizeLatency(latencies["terminal"]),
		BackupDurationMillis: milliseconds(backupDuration), BackupLagSeconds: backupLag.Seconds(),
		AgentRegistryHeapBytes: agentHeapGrowth, RegistryBytesPerAgent: agentHeapGrowth / scaleAgents,
		PeakHeapGrowthBytes: positiveDelta(peakHeap.Load(), beforeAgents.Alloc),
		PeakGoroutines:      peakGoroutines.Load(),
		JobQueueAccepted:    jobAccepted, JobQueueDropped: scaleAgents - jobAccepted,
	}
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	t.Logf("SQLITE_SCALE_RESULT %s", encoded)

	assert.Equal(t, scaleQueueSize, jobAccepted)
	assert.Less(t, result.HeartbeatFlush.P99Millis, 30_000.0)
	assert.Less(t, result.Sync.P99Millis, 10_000.0)
	assert.Less(t, result.Search.P99Millis, 5_000.0)
	assert.Less(t, result.TerminalRoute.P99Millis, 100.0)
	assert.Less(t, result.BackupDurationMillis, 120_000.0)
	assert.Less(t, result.BackupLagSeconds, (26 * time.Hour).Seconds())
	assert.Less(t, result.PeakHeapGrowthBytes, uint64(512<<20))
	assert.Less(t, result.PeakGoroutines, int64(512))
}

func seedScaleDevices(t *testing.T, raw *testdb.DB, now time.Time) []string {
	t.Helper()
	ids := make([]string, scaleAgents)
	tx, err := raw.Begin(context.Background())
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(context.Background()) }()
	for index := range scaleAgents {
		ids[index] = newID()
		_, err = tx.Exec(context.Background(), `
			INSERT INTO devices (id, hostname, registered_at, last_seen_at)
			VALUES (?, ?, ?, ?)`,
			ids[index], fmt.Sprintf("scale-device-%05d", index), now, now.Add(-time.Minute))
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit(context.Background()))
	return ids
}

func fillJobQueue(runner *jobs.Runner) int {
	accepted := 0
	for range scaleAgents {
		if runner.Wake(newID()) {
			accepted++
		}
	}
	return accepted
}

func exerciseSync(ctx context.Context, syncer *agentsync.Service, deviceIDs []string) (map[string][]time.Duration, error) {
	out := map[string][]time.Duration{"sync": {}}
	for _, deviceID := range deviceIDs[:scaleSearches] {
		started := time.Now()
		_, err := syncer.Sync(ctx, deviceID)
		if err != nil {
			return nil, err
		}
		out["sync"] = append(out["sync"], time.Since(started))
	}
	return out, nil
}

func exerciseHeartbeats(ctx context.Context, st *store.Store, snapshot map[string]time.Time) ([]time.Duration, error) {
	latencies := make([]time.Duration, 0, scaleHeartbeatRounds)
	for range scaleHeartbeatRounds {
		at := time.Now().UTC()
		for id := range snapshot {
			snapshot[id] = at
		}
		started := time.Now()
		if err := st.RecordHeartbeatTelemetry(ctx, snapshot); err != nil {
			return nil, err
		}
		latencies = append(latencies, time.Since(started))
	}
	return latencies, nil
}

func exerciseSearch(ctx context.Context, st *store.Store) ([]time.Duration, error) {
	latencies := make([]time.Duration, 0, scaleSearches)
	for index := range scaleSearches {
		started := time.Now()
		rows, total, err := st.Search(ctx, store.SearchParams{
			Scope: "devices", Query: fmt.Sprintf("scale-device-%05d", index), Limit: 10,
		})
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 || total == 0 {
			return nil, errors.New("scale search returned no device")
		}
		latencies = append(latencies, time.Since(started))
	}
	return latencies, nil
}

func exerciseTerminal(ctx context.Context, registry *connection.TerminalSessionRegistry, sessionID string) ([]time.Duration, error) {
	latencies := make([]time.Duration, 0, scaleTerminalFrames)
	message := &cadestrov1.AgentMessage{}
	for range scaleTerminalFrames {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		started := time.Now()
		if !registry.RouteAgentMessage(sessionID, message) {
			return nil, errors.New("terminal session disappeared")
		}
		latencies = append(latencies, time.Since(started))
	}
	return latencies, nil
}

func exerciseBackup(ctx context.Context, raw *testdb.DB, directory string) (time.Duration, time.Time, error) {
	path := filepath.Join(directory, "cadestro-scale.db")
	started := time.Now()
	if err := raw.Backup(ctx, path); err != nil {
		return 0, time.Time{}, err
	}
	backup, err := testdb.Open(ctx, path)
	if err != nil {
		return 0, time.Time{}, err
	}
	defer backup.Close()
	var integrity string
	if err := backup.QueryRow(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		return 0, time.Time{}, fmt.Errorf("verify SQLite backup: %w", err)
	}
	if integrity != "ok" {
		return 0, time.Time{}, fmt.Errorf("verify SQLite backup: %s", integrity)
	}
	violations, err := backup.Query(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("verify SQLite backup foreign keys: %w", err)
	}
	defer violations.Close()
	if violations.Next() {
		var table, parent string
		var rowID any
		var foreignKeyID int
		if err := violations.Scan(&table, &rowID, &parent, &foreignKeyID); err != nil {
			return 0, time.Time{}, fmt.Errorf("read SQLite backup foreign-key violation: %w", err)
		}
		return 0, time.Time{}, fmt.Errorf("verify SQLite backup foreign keys: table %s row %v references %s (constraint %d)", table, rowID, parent, foreignKeyID)
	}
	if err := violations.Err(); err != nil {
		return 0, time.Time{}, fmt.Errorf("verify SQLite backup foreign keys: %w", err)
	}
	return time.Since(started), time.Now().UTC(), nil
}

func sampleRuntime(stop <-chan struct{}, done *sync.WaitGroup, peakGoroutines *atomic.Int64, peakHeap *atomic.Uint64) {
	defer done.Done()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			goroutines := int64(runtime.NumGoroutine())
			for current := peakGoroutines.Load(); goroutines > current && !peakGoroutines.CompareAndSwap(current, goroutines); current = peakGoroutines.Load() {
			}
			var memory runtime.MemStats
			runtime.ReadMemStats(&memory)
			for current := peakHeap.Load(); memory.Alloc > current && !peakHeap.CompareAndSwap(current, memory.Alloc); current = peakHeap.Load() {
			}
		}
	}
}

func summarizeLatency(values []time.Duration) latencySummary {
	if len(values) == 0 {
		return latencySummary{}
	}
	ordered := append([]time.Duration(nil), values...)
	slices.Sort(ordered)
	return latencySummary{
		P50Millis: milliseconds(percentile(ordered, 50)),
		P95Millis: milliseconds(percentile(ordered, 95)),
		P99Millis: milliseconds(percentile(ordered, 99)),
		MaxMillis: milliseconds(ordered[len(ordered)-1]),
	}
}

func percentile(ordered []time.Duration, percent int) time.Duration {
	index := (len(ordered)*percent + 99) / 100
	return ordered[max(0, index-1)]
}

func milliseconds(value time.Duration) float64 { return float64(value) / float64(time.Millisecond) }

func positiveDelta(after, before uint64) uint64 {
	if after <= before {
		return 0
	}
	return after - before
}

func assertScaleDatabaseState(t *testing.T, ctx context.Context, raw *testdb.DB, agents int) {
	t.Helper()
	var storedAgents, seenAgents int
	require.NoError(t, raw.QueryRow(ctx, `SELECT count(*) FROM devices`).Scan(&storedAgents))
	require.NoError(t, raw.QueryRow(ctx, `SELECT count(*) FROM devices WHERE last_seen_at IS NOT NULL`).Scan(&seenAgents))
	assert.Equal(t, agents, storedAgents)
	assert.Equal(t, agents, seenAgents)
}
