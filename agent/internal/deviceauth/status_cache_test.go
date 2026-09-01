package deviceauth

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manchtools/cadestro/agent/internal/credentials"
	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

type countingStore struct {
	exists bool
	loads  atomic.Int64
}

func (c *countingStore) Exists() bool { return c.exists }
func (c *countingStore) Load() (*credentials.Credentials, error) {
	c.loads.Add(1)
	return &credentials.Credentials{DeviceID: "dev-cached"}, nil
}
func (c *countingStore) Save(context.Context, *credentials.Credentials) error { return nil }

func newStatusHandler(store credentialStore) *EnrollHandler {
	return &EnrollHandler{credStore: store, logger: slog.Default(), now: time.Now}
}

type flakyStore struct {
	exists bool
	creds  *credentials.Credentials
	err    error
}

func (f *flakyStore) Exists() bool                                         { return f.exists }
func (f *flakyStore) Load() (*credentials.Credentials, error)              { return f.creds, f.err }
func (f *flakyStore) Save(context.Context, *credentials.Credentials) error { return nil }

func TestGetEnrollmentStatus_LoadFailureNotCached(t *testing.T) {
	store := &flakyStore{exists: true, err: errors.New("decrypt failed")}
	h := newStatusHandler(store)

	resp, err := h.GetEnrollmentStatus(context.Background(), connect.NewRequest(&cadestrov1.GetEnrollmentStatusRequest{}))
	require.NoError(t, err)
	assert.False(t, resp.Msg.Enrolled, "a load failure must report not-enrolled")

	store.err = nil
	store.creds = &credentials.Credentials{DeviceID: "dev-recovered"}
	resp, err = h.GetEnrollmentStatus(context.Background(), connect.NewRequest(&cadestrov1.GetEnrollmentStatusRequest{}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.Enrolled, "a transient load failure must not be cached as not-enrolled")
	assert.Equal(t, "dev-recovered", resp.Msg.GetDeviceId().GetValue())
}

func TestGetEnrollmentStatus_LoadsAtMostOnce(t *testing.T) {
	store := &countingStore{exists: true}
	h := newStatusHandler(store)

	for i := 0; i < 50; i++ {
		resp, err := h.GetEnrollmentStatus(context.Background(), connect.NewRequest(&cadestrov1.GetEnrollmentStatusRequest{}))
		require.NoError(t, err)
		assert.True(t, resp.Msg.Enrolled)
		assert.Equal(t, "dev-cached", resp.Msg.GetDeviceId().GetValue())
	}
	assert.LessOrEqual(t, store.loads.Load(), int64(1),
		"Load must run at most once across repeated status calls; got %d", store.loads.Load())
}

func TestGetEnrollmentStatus_ConcurrentFloodLoadsOnce(t *testing.T) {
	store := &countingStore{exists: true}
	h := newStatusHandler(store)

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = h.GetEnrollmentStatus(context.Background(), connect.NewRequest(&cadestrov1.GetEnrollmentStatusRequest{}))
		}()
	}
	wg.Wait()
	assert.Equal(t, int64(1), store.loads.Load(),
		"a concurrent status flood must trigger exactly one credential load")
}

func TestGetEnrollmentStatus_NotEnrolledNeverLoads(t *testing.T) {
	store := &countingStore{exists: false}
	h := newStatusHandler(store)

	resp, err := h.GetEnrollmentStatus(context.Background(), connect.NewRequest(&cadestrov1.GetEnrollmentStatusRequest{}))
	require.NoError(t, err)
	assert.False(t, resp.Msg.Enrolled)
	assert.Equal(t, int64(0), store.loads.Load(), "un-enrolled status must not load credentials")
}
