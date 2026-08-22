//go:build linux

package luksd

import (
	"context"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPeerCredListener_AcceptsSameUIDPeer(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "peer.sock")
	base, err := net.Listen("unix", socket)
	require.NoError(t, err)
	l := newPeerCredListener(base, slog.Default())
	defer func() { _ = l.Close() }()

	accepted := make(chan net.Conn, 1)
	acceptErr := make(chan error, 1)
	go func() {
		conn, err := l.Accept()
		if err != nil {
			acceptErr <- err
			return
		}
		accepted <- conn
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client, err := (&net.Dialer{}).DialContext(ctx, "unix", socket)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	select {
	case conn := <-accepted:
		_ = conn.Close()
	case err := <-acceptErr:
		t.Fatalf("Accept refused an authorized same-uid peer: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the authorized peer to be accepted")
	}
}

func TestPeerCredListener_RefusesPeerWithUnreadableCredentials(t *testing.T) {
	base, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	l := newPeerCredListener(base, slog.Default())
	defer func() { _ = l.Close() }()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := l.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client, err := (&net.Dialer{}).DialContext(ctx, "tcp", base.Addr().String())
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	select {
	case conn := <-accepted:
		_ = conn.Close()
		t.Fatal("Accept yielded a connection whose peer credentials cannot be read; it must fail closed")
	case <-time.After(500 * time.Millisecond):
	}
}

func TestLuksDaemon_SocketRemainsConnectableAtTightenedMode(t *testing.T) {
	sess := &ctxRecordingSession{}
	sock := startDaemon(t, sess)

	info, err := os.Stat(sock)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o622), info.Mode().Perm())

	resp, err := submit(sock, Request{Token: "tok", Passphrase: goodPassphrase})
	require.NoError(t, err, "the tightened socket mode must not break the unprivileged client")
	require.True(t, resp.OK, "%+v", resp)
}
