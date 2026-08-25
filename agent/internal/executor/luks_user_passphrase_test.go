package executor

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manchtools/cadestro/agent/internal/store"
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func TestReconcileDeviceKey_UserPassphrasePersistsModeAndConverges(t *testing.T) {
	st, err := store.New(t.TempDir())
	require.NoError(t, err)
	defer st.Close()

	const actionID = "01HXLUKSUSERPASS0000000000"
	const devicePath = "/dev/mapper/test"
	require.NoError(t, st.SetLuksOwnershipTaken(context.Background(), actionID, devicePath))

	e := &Executor{logger: slog.Default(), now: time.Now}
	e.SetStore(st)

	params := &pb.EncryptionParams{
		DeviceBoundKeyType: pb.EncryptionDeviceBoundKeyType_ENCRYPTION_DEVICE_BOUND_KEY_TYPE_USER_PASSPHRASE,
	}

	ls, err := st.GetLuksState(context.Background(), actionID)
	require.NoError(t, err)
	require.Equal(t, "none", ls.DeviceKeyType)

	changed, err := e.reconcileDeviceKey(context.Background(), params, ls, actionID, devicePath)
	require.NoError(t, err)
	assert.True(t, changed, "first reconcile to user_passphrase is a change")

	ls2, err := st.GetLuksState(context.Background(), actionID)
	require.NoError(t, err)
	assert.Equal(t, "user_passphrase", ls2.DeviceKeyType,
		"reconcile must persist the user_passphrase mode so it converges")

	changed2, err := e.reconcileDeviceKey(context.Background(), params, ls2, actionID, devicePath)
	require.NoError(t, err)
	assert.False(t, changed2, "user_passphrase must converge, not report changed=true forever")
}
