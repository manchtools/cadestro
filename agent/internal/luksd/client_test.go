package luksd

import (
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func startRecordingServer(t *testing.T, resp Response) (socketPath string, received *Request, done <-chan struct{}) {
	t.Helper()
	socketPath = filepath.Join(t.TempDir(), "luks.sock")
	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	received = &Request{}
	doneCh := make(chan struct{})

	go func() {
		defer close(doneCh)
		conn, aerr := ln.Accept()
		if aerr != nil {
			return
		}
		defer conn.Close()
		_ = json.NewDecoder(conn).Decode(received)
		_ = json.NewEncoder(conn).Encode(resp)
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return socketPath, received, doneCh
}

func awaitServer(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("recording server did not complete")
	}
}

func TestLuksClient_CollectsPassphraseAndSendsTokenOnly(t *testing.T) {
	sock, received, done := startRecordingServer(t, Response{OK: true, Code: CodeOK})

	c := NewClient(sock)
	err := c.SetPassphrase("my-token", func() (string, error) {
		return "user-chosen-passphrase", nil
	})
	require.NoError(t, err)

	awaitServer(t, done)
	assert.Equal(t, "my-token", received.Token)
	assert.Equal(t, "user-chosen-passphrase", received.Passphrase)

	b, _ := json.Marshal(received)
	assert.NotContains(t, string(b), "data_dir")
	assert.NotContains(t, string(b), "store")
}

func TestLuksClient_SurfacesDaemonError(t *testing.T) {
	sock, _, done := startRecordingServer(t, Response{OK: false, Code: CodeInvalidToken, Error: "token is invalid or has expired"})

	c := NewClient(sock)
	err := c.SetPassphrase("tok", func() (string, error) { return "user-chosen-passphrase", nil })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")
	awaitServer(t, done)
}

func TestLuksClient_RefusesEmptyPassphrase(t *testing.T) {
	dialed := false
	c := NewClient(filepath.Join(t.TempDir(), "nope.sock"))
	c.dialer = func() (net.Conn, error) {
		dialed = true
		return nil, assertNoDial(t)
	}
	err := c.SetPassphrase("tok", func() (string, error) { return "", nil })
	require.Error(t, err)
	assert.False(t, dialed, "client must not contact the daemon with an empty passphrase")
}

func assertNoDial(t *testing.T) error {
	t.Helper()
	t.Error("dialer should not have been called")
	return nil
}
