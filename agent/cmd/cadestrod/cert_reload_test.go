package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manchtools/cadestro/agent/internal/credentials"
)

func testCreds(cert string) *credentials.Credentials {
	return &credentials.Credentials{
		DeviceID:    "dev-1",
		CACert:      []byte("ca"),
		Certificate: []byte(cert),
		PrivateKey:  []byte("key"),
		AgentAddr:   "https://gw:8443",
		ControlAddr: "https://ctl",
	}
}

func TestReloadCredsForReconnect_PicksUpRotatedCert(t *testing.T) {

	if !credentials.MachineIDAvailable() {
		t.Skip("no machine-id on this host; credential save/load is unavailable")
	}

	dir := t.TempDir()
	store := credentials.NewStore(dir)

	inMemory := testCreds("OLD-CERT")
	require.NoError(t, store.Save(inMemory))

	require.NoError(t, store.Save(testCreds("NEW-CERT")))

	got := reloadCredsForReconnect(store, inMemory, slog.Default())
	assert.Equal(t, []byte("NEW-CERT"), got.Certificate,
		"reconnect must use the rotated cert from disk, not the stale in-memory one")
}

func TestReloadCredsForReconnect_FallsBackOnError(t *testing.T) {
	dir := t.TempDir()
	store := credentials.NewStore(dir)

	inMemory := testCreds("WORKING-CERT")

	require.NoError(t, os.RemoveAll(filepath.Join(dir)))

	got := reloadCredsForReconnect(store, inMemory, slog.Default())
	assert.Same(t, inMemory, got, "a failed reload must return the in-memory credentials unchanged")
}
