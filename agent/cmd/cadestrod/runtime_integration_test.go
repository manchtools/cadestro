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
	pm "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
)

type runtimeStreamService struct {
	cadestrov1connect.UnimplementedAgentServiceHandler
	state *pm.SyncState
	mu    sync.Mutex
	syncs chan string
}

func (s *runtimeStreamService) Stream(ctx context.Context, stream *connect.BidiStream[pm.AgentMessage, pm.ServerMessage]) error {
	for {
		message, err := stream.Receive()
		if err != nil {
			return err
		}
		switch {
		case message.GetHello() != nil:
			if err := stream.Send(&pm.ServerMessage{Id: message.Id, Payload: &pm.ServerMessage_Welcome{Welcome: &pm.Welcome{}}}); err != nil {
				return err
			}
		case message.GetSyncRequest() != nil:
			s.mu.Lock()
			s.syncs <- message.Id
			s.mu.Unlock()
			if err := stream.Send(&pm.ServerMessage{Id: message.Id, Payload: &pm.ServerMessage_SyncState{SyncState: s.state}}); err != nil {
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

func newRuntimeLoopback(t *testing.T, state *pm.SyncState) *runtimeLoopback {
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
	return scheduler.New(state, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestPeriodicSyncLiveTriggerSendsFullSync(t *testing.T) {
	loopback := newRuntimeLoopback(t, &pm.SyncState{})
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
	loopback := newRuntimeLoopback(t, &pm.SyncState{SyncIntervalMinutes: 7})
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
	loopback := newRuntimeLoopback(t, &pm.SyncState{})
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

func TestSyncPendingResultsPreservesFailedResultForRetry(t *testing.T) {
	st, err := store.New(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	delivery := &pm.ManifestDelivery{
		DeliveryId: "01K00000000000000000000011",
		Manifest:   &pm.Manifest{ManifestId: "01K00000000000000000000012", Schedule: &pm.ActionSchedule{RunOnAssign: true}, Occurrences: []*pm.ManifestOccurrence{{OccurrenceId: "01K00000000000000000000013", Action: &pm.Action{Id: &pm.ActionId{Value: "01K00000000000000000000014"}}}}},
	}
	_, err = st.RecordManifestDelivery(context.Background(), delivery)
	require.NoError(t, err)
	_, err = st.BeginManifestRun(delivery, time.Now())
	require.NoError(t, err)
	occurrence := delivery.GetManifest().GetOccurrences()[0]
	require.NoError(t, st.MarkOccurrenceStarted(delivery.GetDeliveryId(), occurrence.GetOccurrenceId(), time.Now()))
	_, err = st.RecoverInterruptedOccurrences()
	require.NoError(t, err)
	sched := scheduler.New(st, nil, slog.Default())
	client := sdk.NewClient("http://127.0.0.1:1")
	syncPendingResults(context.Background(), sched, client, slog.Default())
	syncPendingResults(context.Background(), sched, client, slog.Default())
	pending, err := sched.GetPendingResults()
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, occurrence.GetOccurrenceId(), pending[0].ActionResult.GetOccurrenceId())
}
