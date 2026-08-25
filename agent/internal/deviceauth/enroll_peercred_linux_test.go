//go:build linux

package deviceauth

import (
	"log/slog"
	"net"
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

	client, err := net.Dial("unix", socket)
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
