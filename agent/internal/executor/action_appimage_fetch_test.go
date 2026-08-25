package executor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func withRemoteTestClient(t *testing.T, srv *httptest.Server) {
	t.Helper()
	prev := remoteHTTPClient
	remoteHTTPClient = srv.Client()
	t.Cleanup(func() { remoteHTTPClient = prev })
}

func appImageFetchExecutor() *Executor {
	return &Executor{logger: slog.Default(), now: time.Now, repairFS: func(context.Context) bool { return true }}
}

func TestExecuteAppImage_InstallsViaRemoteFetch(t *testing.T) {
	content := []byte("#!/bin/sh\necho appimage-ok\n")
	sum := sha256.Sum256(content)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(content)
	}))
	defer srv.Close()
	withRemoteTestClient(t, srv)

	installDir := t.TempDir()
	out, changed, err := appImageFetchExecutor().executeAppImage(context.Background(), &pb.AppInstallParams{
		Url:            srv.URL + "/app.AppImage",
		ChecksumSha256: hex.EncodeToString(sum[:]),
		InstallPath:    installDir,
	}, pb.DesiredState_DESIRED_STATE_PRESENT)
	require.NoError(t, err)
	assert.True(t, changed, "a fresh install reports changed=true")
	_ = out

	dest := filepath.Join(installDir, "app.AppImage")
	info, statErr := os.Stat(dest)
	require.NoError(t, statErr, "AppImage must be placed in the install dir")
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm(), "AppImage must be placed at 0755 (executable)")
	got, _ := os.ReadFile(dest)
	assert.True(t, bytes.Equal(got, content), "placed bytes must match the download")
}

func TestExecuteAppImage_RemoteFetchMismatchPreservesExisting(t *testing.T) {
	installDir := t.TempDir()
	dest := filepath.Join(installDir, "app.AppImage")
	sentinel := []byte("EXISTING-GOOD-APPIMAGE-DO-NOT-CLOBBER")
	require.NoError(t, os.WriteFile(dest, sentinel, 0o755))

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("CORRUPT-OR-TAMPERED"))
	}))
	defer srv.Close()
	withRemoteTestClient(t, srv)

	wrong := sha256.Sum256([]byte("a third value — neither sentinel nor served"))
	_, _, err := appImageFetchExecutor().executeAppImage(context.Background(), &pb.AppInstallParams{
		Url:            srv.URL + "/app.AppImage",
		ChecksumSha256: hex.EncodeToString(wrong[:]),
		InstallPath:    installDir,
	}, pb.DesiredState_DESIRED_STATE_PRESENT)
	require.Error(t, err, "a checksum-mismatched download must fail")

	got, _ := os.ReadFile(dest)
	assert.True(t, bytes.Equal(got, sentinel), "the pre-existing AppImage must be left intact on a failed download")
}
