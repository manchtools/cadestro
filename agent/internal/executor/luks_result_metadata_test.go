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
	sysenc "github.com/manchtools/cadestro/sdk/sys/encryption"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

type fakeDetectEncManager struct {
	fakeEncManager
	devicePath string
}

func (f *fakeDetectEncManager) DetectVolumeByKey(context.Context, sysexec.Secret) (sysenc.Volume, error) {
	return sysenc.Volume{DevicePath: f.devicePath}, nil
}

func newLuksExecutor(t *testing.T, devicePath string) *Executor {
	t.Helper()
	st, err := store.New(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	fakeEnc := &fakeDetectEncManager{devicePath: devicePath}

	var stored string
	keys := &fakeLuksKeyStore{
		getKeyFunc: func(context.Context, string) (string, error) { return stored, nil },
		storeKeyFunc: func(_ context.Context, _, _, passphrase string, _ pb.RotationReason) error {
			stored = passphrase
			return nil
		},
	}
	e := NewExecutor(nil)
	e.deps.encrypt = fakeEnc
	e.logger = slog.Default()
	e.now = time.Now
	e.SetStore(st)
	e.SetLuksKeyStore(keys)
	return e
}

func TestSetupLuksReportsNoMetadata(t *testing.T) {
	e := newLuksExecutor(t, "/dev/mapper/root")

	_, _, metadata, err := e.setupLuks(context.Background(),
		&pb.EncryptionParams{MinWords: 3},
		"01HXLUKSMETA00000000000000", func() ([]byte, error) { return []byte("psk-value"), nil })
	require.NoError(t, err)
	assert.Empty(t, metadata, "control rejects every result that carries metadata")
}

func TestExecuteEncryptionActionReportsNoResultMetadata(t *testing.T) {
	e := newLuksExecutor(t, "/dev/mapper/root")
	const actionID = "01HXLUKSEXEC00000000000000"

	result := e.ExecuteAction(context.Background(), &pb.Action{
		Id:           &pb.ActionId{Value: actionID},
		Type:         pb.ActionType_ACTION_TYPE_ENCRYPTION,
		DesiredState: pb.DesiredState_DESIRED_STATE_PRESENT,
		Params: &pb.Action_Encryption{Encryption: &pb.EncryptionParams{
			PresharedKey: []byte("psk-value"), MinWords: 3,
		}},
	})
	require.NotNil(t, result)

	require.Equal(t, pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS, result.Status, result.Error)
	assert.Empty(t, result.Metadata,
		"a result control refuses is lost on send or replayed on every reconnect")
}
