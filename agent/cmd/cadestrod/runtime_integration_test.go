package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"syscall"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	"github.com/manchtools/cadestro/agent/internal/scheduler"
	"github.com/manchtools/cadestro/agent/internal/store"
	sdk "github.com/manchtools/cadestro/contract"
	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
)

type runtimeStreamService struct {
	cadestrov1connect.UnimplementedAgentServiceHandler
	state *cadestrov1.SyncState
	mu    sync.Mutex
	syncs chan string
}

func (s *runtimeStreamService) Stream(ctx context.Context, stream *connect.BidiStream[cadestrov1.AgentMessage, cadestrov1.ServerMessage]) error {
	for {
		message, err := stream.Receive()
		if err != nil {
			return err
		}
		switch {
		case message.GetHello() != nil:
			if err := stream.Send(&cadestrov1.ServerMessage{Id: message.Id, Payload: &cadestrov1.ServerMessage_Welcome{Welcome: &cadestrov1.Welcome{}}}); err != nil {
				return err
			}
		case message.GetSyncRequest() != nil:
			s.mu.Lock()
			s.syncs <- message.Id
			s.mu.Unlock()
			if err := stream.Send(&cadestrov1.ServerMessage{Id: message.Id, Payload: &cadestrov1.ServerMessage_SyncState{SyncState: s.state}}); err != nil {
				return err
			}
		}
	}
}

type runtimeLoopback struct {
	client  *sdk.Client
	service *runtimeStreamService
	close   func()
}

func newRuntimeLoopback(t *testing.T, state *cadestrov1.SyncState) *runtimeLoopback {
	t.Helper()
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if errors.Is(err, syscall.EPERM) {
		t.Skip("sandbox forbids TCP sockets")
	}
	require.NoError(t, err)
	_ = probe.Close()
	service := &runtimeStreamService{state: state, syncs: make(chan string, 4)}
	path, handler := cadestrov1connect.NewAgentServiceHandler(service)
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	server := httptest.NewUnstartedServer(mux)
	server.Config.Protocols = protocols
	server.Start()
	httpClient := &http.Client{Transport: &http.Transport{Protocols: protocols}}
	client := sdk.NewClient(server.URL, sdk.WithHTTPClient(httpClient), sdk.WithAuth("01K00000000000000000000001", ""))
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, client.Connect(ctx))
	require.NoError(t, client.SendHello(ctx, "host", "test"))
	receiverCancel := client.StartReceiver(ctx)
	return &runtimeLoopback{
		client: client, service: service,
		close: func() {
			receiverCancel()
			cancel()
			_ = client.Close()
			server.Close()
		},
	}
}

func testRuntimeScheduler(t *testing.T) *scheduler.Scheduler {
	t.Helper()
	state, err := store.New(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, state.Close()) })
	return scheduler.New(context.Background(), state, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestPeriodicSyncLiveTriggerSendsFullSync(t *testing.T) {
	loopback := newRuntimeLoopback(t, &cadestrov1.SyncState{})
	t.Cleanup(loopback.close)
	trigger := make(chan struct{}, 1)
	updates := make(chan time.Duration, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go periodicSync(ctx, loopback.client, testRuntimeScheduler(t), time.Hour, updates, trigger, slog.Default(), nil)
	trigger <- struct{}{}
	select {
	case <-loopback.service.syncs:
	case <-time.After(2 * time.Second):
		t.Fatal("live sync trigger did not send a Sync request")
	}
}

func TestPeriodicSyncPropagatesServerInterval(t *testing.T) {
	loopback := newRuntimeLoopback(t, &cadestrov1.SyncState{SyncIntervalMinutes: 7})
	t.Cleanup(loopback.close)
	trigger := make(chan struct{}, 1)
	updates := make(chan time.Duration, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go periodicSync(ctx, loopback.client, testRuntimeScheduler(t), time.Hour, updates, trigger, slog.Default(), nil)
	trigger <- struct{}{}
	select {
	case <-loopback.service.syncs:
	case <-time.After(2 * time.Second):
		t.Fatal("server interval test did not send a Sync request")
	}
	select {
	case got := <-updates:
		require.Equal(t, 7*time.Minute, got)
	case <-time.After(2 * time.Second):
		t.Fatal("server-provided sync interval was not propagated")
	}
}

func TestPeriodicSyncRenewalRunsBeforeSync(t *testing.T) {
	loopback := newRuntimeLoopback(t, &cadestrov1.SyncState{})
	t.Cleanup(loopback.close)
	trigger := make(chan struct{}, 1)
	before := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go periodicSync(ctx, loopback.client, testRuntimeScheduler(t), time.Hour, make(chan time.Duration, 1), trigger, slog.Default(), func() bool {
		before <- struct{}{}
		return true
	})
	trigger <- struct{}{}
	select {
	case <-before:
	case <-time.After(2 * time.Second):
		t.Fatal("renewal seam was not called before Sync")
	}
	select {
	case requestID := <-loopback.service.syncs:
		t.Fatalf("Sync ran after renewal staged a certificate: %s", requestID)
	default:
	}
}
