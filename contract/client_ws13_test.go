package contract

import (
	"context"
	"errors"
	"net/http"
	"testing"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/stretchr/testify/require"
)

const testULID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"

// recordingTransport records CloseIdleConnections calls so the #8 seam can be
// verified without a real network.
type recordingTransport struct{ closeCalls int }

func (r *recordingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("recordingTransport: no real requests")
}
func (r *recordingTransport) CloseIdleConnections() { r.closeCalls++ }

// TestClient_CloseIdleConnections pins WS13 #8: CloseIdleConnections releases the
// transport's idle connections (so a reconnect loop doesn't leak transports) and
// is nil-safe.
func TestClient_CloseIdleConnections(t *testing.T) {
	rt := &recordingTransport{}
	c := NewClient("https://gw.example", WithHTTPClient(&http.Client{Transport: rt}))

	c.CloseIdleConnections()
	require.Equal(t, 1, rt.closeCalls, "CloseIdleConnections must release the underlying transport's idle connections")

	var nilClient *Client
	require.NotPanics(t, func() { nilClient.CloseIdleConnections() }, "must be nil-safe")
}

// TestDispatch_DropsInvalidInbound pins WS13 #5: a command frame that violates
// the inbound `validate` gotags (out-of-range PTY dims, non-ULID session id) is
// dropped before the handler; a conformant frame reaches it.
func TestDispatch_DropsInvalidInbound(t *testing.T) {
	ctx := context.Background()
	start := func(sid, tty string, cols, rows uint32) *cadestrov1.ServerMessage {
		return &cadestrov1.ServerMessage{Id: NewULID(), Payload: &cadestrov1.ServerMessage_TerminalStart{
			TerminalStart: &cadestrov1.TerminalStart{SessionId: sid, TtyUser: tty, Cols: cols, Rows: rows},
		}}
	}

	t.Run("cols=0 dropped (gt=0)", func(t *testing.T) {
		c := newTestClient()
		h := &fakeTerminalHandler{}
		require.NoError(t, c.dispatchServerMessage(ctx, start(testULID, "cadestro-tty-x", 0, 24), h))
		require.Empty(t, h.startCalls, "a TerminalStart with cols=0 must be dropped by inbound validation")
	})

	t.Run("non-ULID session id dropped", func(t *testing.T) {
		c := newTestClient()
		h := &fakeTerminalHandler{}
		require.NoError(t, c.dispatchServerMessage(ctx, start("not-a-ulid", "cadestro-tty-x", 80, 24), h))
		require.Empty(t, h.startCalls, "a TerminalStart with a non-ULID session id must be dropped")
	})

	t.Run("conformant frame reaches the handler", func(t *testing.T) {
		c := newTestClient()
		h := &fakeTerminalHandler{}
		require.NoError(t, c.dispatchServerMessage(ctx, start(testULID, "cadestro-tty-x", 80, 24), h))
		require.Len(t, h.startCalls, 1, "a valid TerminalStart must reach the handler")
	})
}
