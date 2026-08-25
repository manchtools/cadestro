package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func TestGetArchEntry(t *testing.T) {
	amd := &pb.AgentUpdateArch{BinaryUrl: "https://example.com/amd64"}
	arm := &pb.AgentUpdateArch{BinaryUrl: "https://example.com/arm64"}
	params := &pb.AgentUpdateParams{Amd64: amd, Arm64: arm}

	entry := getArchEntry(params)
	if entry == nil {
		t.Fatal("expected non-nil arch entry for current runtime")
	}
}

func TestGetArchEntry_NilForMissing(t *testing.T) {

	params := &pb.AgentUpdateParams{}
	entry := getArchEntry(params)
	if entry != nil {
		t.Error("expected nil for empty params")
	}
}

func writeStateForTest(t *testing.T, dataDir, phase, version string) {
	t.Helper()
	dir := filepath.Join(dataDir, "update")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data := fmt.Sprintf(`{"phase":%q,"version":%q}`, phase, version)
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(data), 0o600); err != nil {
		t.Fatalf("write state.json: %v", err)
	}
}

func TestWriteAndReadUpdateState(t *testing.T) {
	dir := t.TempDir()

	writeStateForTest(t, dir, "staged", "v2026.04.01")

	phase, version, err := readUpdateState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if phase != "staged" {
		t.Errorf("phase = %q, want %q", phase, "staged")
	}
	if version != "v2026.04.01" {
		t.Errorf("version = %q, want %q", version, "v2026.04.01")
	}
}

func TestReadUpdateState_NotFound(t *testing.T) {
	dir := t.TempDir()
	phase, version, err := readUpdateState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if phase != "" || version != "" {
		t.Errorf("expected empty state for missing file, got phase=%q version=%q", phase, version)
	}
}

func TestClearUpdateState(t *testing.T) {
	dir := t.TempDir()
	writeStateForTest(t, dir, "staged", "v1.0")
	clearUpdateState(dir)

	phase, _, err := readUpdateState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if phase != "" {
		t.Errorf("expected empty phase after clear, got %q", phase)
	}
}

func TestMarkAgentUpdateExecuted(t *testing.T) {

	e := &Executor{now: time.Now}

	if !e.markAgentUpdateExecuted() {
		t.Error("expected first markAgentUpdateExecuted to return true")
	}

	if e.markAgentUpdateExecuted() {
		t.Error("expected second markAgentUpdateExecuted to return false")
	}

	e.ResetUpdateCycle()
	if !e.markAgentUpdateExecuted() {
		t.Error("expected markAgentUpdateExecuted to return true after reset")
	}
}

func TestCheckStartupUpdateState_CleansStaleState(t *testing.T) {
	dir := t.TempDir()
	writeStateForTest(t, dir, "staged", "2026.04.01")

	logger := &testLogger{}
	CheckStartupUpdateState(dir, logger, time.Now)

	phase, _, _ := readUpdateState(dir)
	if phase != "" {
		t.Errorf("expected state to be cleared, got phase=%q", phase)
	}

	if len(logger.infos) == 0 {
		t.Error("expected info log for stale state cleanup")
	}
}

func TestCheckStartupUpdateState_NoState(t *testing.T) {
	dir := t.TempDir()

	logger := &testLogger{}
	CheckStartupUpdateState(dir, logger, time.Now)

	if len(logger.infos) > 0 || len(logger.warns) > 0 {
		t.Error("expected no logs for clean startup without state file")
	}
}

func TestGetBinaryVersion(t *testing.T) {

	dir := t.TempDir()
	script := filepath.Join(dir, "fake-agent")
	err := os.WriteFile(script, []byte("#!/bin/sh\necho 'v2026.04.01'\n"), 0755)
	if err != nil {
		t.Fatal(err)
	}

	version, err := getBinaryVersion(context.Background(), script)
	if err != nil {
		t.Fatal(err)
	}
	if version != "v2026.04.01" {
		t.Errorf("version = %q, want %q", version, "v2026.04.01")
	}
}

func TestGetBinaryVersion_Empty(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-agent")
	err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0755)
	if err != nil {
		t.Fatal(err)
	}

	_, err = getBinaryVersion(context.Background(), script)
	if err == nil {
		t.Error("expected error for empty version output")
	}
}

func TestGetBinaryVersion_UsesCallerCancellation(t *testing.T) {
	dir := t.TempDir()
	release := filepath.Join(dir, "release")
	script := filepath.Join(dir, "fake-agent")
	contents := fmt.Sprintf("#!/bin/sh\nwhile [ ! -f %q ]; do sleep 0.01; done\necho 'v2026.04.01'\n", release)
	if err := os.WriteFile(script, []byte(contents), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := getBinaryVersion(ctx, script)
		done <- err
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("getBinaryVersion error = %v, want context.Canceled", err)
		}
	case <-time.After(500 * time.Millisecond):
		if err := os.WriteFile(release, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		<-done
		t.Fatal("getBinaryVersion ignored caller cancellation")
	}
}

func TestSelfTestScript_ExitCode(t *testing.T) {

	dir := t.TempDir()

	successScript := filepath.Join(dir, "success")
	if err := os.WriteFile(successScript, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}

	failScript := filepath.Join(dir, "fail")
	if err := os.WriteFile(failScript, []byte("#!/bin/sh\necho 'connection failed' >&2\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	cmd := exec.CommandContext(ctx, successScript)
	if err := cmd.Run(); err != nil {
		t.Errorf("expected exit 0 script to succeed, got: %v", err)
	}

	cmd = exec.CommandContext(ctx, failScript)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Error("expected exit 1 script to fail")
	}
	if !strings.Contains(string(out), "connection failed") {
		t.Errorf("expected error output, got: %s", string(out))
	}
}

type testLogger struct {
	infos  []string
	warns  []string
	errors []string
}

func (l *testLogger) Info(msg string, args ...any) {
	l.infos = append(l.infos, msg)
}

func (l *testLogger) Warn(msg string, args ...any) {
	l.warns = append(l.warns, msg)
}

func (l *testLogger) Error(msg string, args ...any) {
	l.errors = append(l.errors, msg)
}
