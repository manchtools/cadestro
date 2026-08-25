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

// CHARTER — LUKS passphrase custody (AGENT side).
//
//   - The passphrase control receives is EXACTLY the one added to the volume.
//   - Without a route to control the store path fails closed BEFORE any LUKS
//     mutation — no half-owned volume, no slot holding a passphrase nobody can
//     recover.
//
// The mTLS transport boundary is tested at the runtime adapter. These tests
// pin local custody:
// a volume must never end up with a passphrase control did not receive.

// fakeEncManager stubs the encryption Manager for the custody tests: AddKey
// and RemoveKey are recorded no-ops, VerifyPassphrase always matches. Every
// un-overridden method nil-panics via the embedded interface.
type fakeEncManager struct {
	sysenc.Manager
	addKeyCalls    int
	removeKeyCalls int
	// onAddKey observes the NEW key handed to AddKey, so a test can compare it
	// against what reached the key store.
	onAddKey func(newKey sysexec.Secret)
}

func (f *fakeEncManager) AddKey(_ context.Context, _ string, _, newKey sysexec.Secret, _ sysenc.AddKeyOptions) error {
	f.addKeyCalls++
	if f.onAddKey != nil {
		f.onAddKey(newKey)
	}
	return nil
}

func (f *fakeEncManager) RemoveKey(_ context.Context, _ string, _ sysexec.Secret) error {
	f.removeKeyCalls++
	return nil
}

func (f *fakeEncManager) VerifyPassphrase(_ context.Context, _ string, _ sysexec.Secret) (bool, error) {
	return true, nil
}

// The custody property: the passphrase control is given is the same one the
// volume now accepts, and it is stored BEFORE the PSK is removed — so a store
// failure can never strand a volume whose only remaining key is unknown to the
// server.
func TestTakeOwnership_StoresTheSamePassphraseItAddsToTheVolume(t *testing.T) {
	const (
		actionID = "01HXSTORE00000000000000000"
	)

	st, err := store.New(t.TempDir())
	require.NoError(t, err)
	defer st.Close()

	fakeEnc := &fakeEncManager{}
	// executor wiring is set below before the action runs

	var stored string
	var storedAtRemoveCount int
	var addedPassphrase string
	fakeEnc.onAddKey = func(newKey sysexec.Secret) { addedPassphrase = newKey.Reveal() }

	ks := &fakeLuksKeyStore{
		getKeyFunc: func(_ context.Context, _ string) (string, error) {
			if stored == "" {
				return "", nil // no server-side key yet → PSK ownership path
			}
			return stored, nil
		},
		storeKeyFunc: func(_ context.Context, gotAction, _, passphrase string, _ pb.RotationReason) error {
			assert.Equal(t, actionID, gotAction, "the key must be stored under the action that generated it")
			stored = passphrase
			storedAtRemoveCount = fakeEnc.removeKeyCalls
			return nil
		},
	}

	e := NewExecutor(nil)
	e.deps.encrypt = fakeEnc
	e.logger = slog.Default()
	e.now = time.Now
	e.SetStore(st)
	e.SetLuksKeyStore(ks)

	params := &pb.EncryptionParams{MinWords: 3}
	require.NoError(t, e.takeOwnership(context.Background(), params, actionID, "/dev/mapper/test", []byte("psk-value")))

	require.NotEmpty(t, stored, "StoreKey must have been called with the managed passphrase")
	require.NotEmpty(t, addedPassphrase, "AddKey must have been called")
	assert.Equal(t, addedPassphrase, stored,
		"control holds a different passphrase than the one in the LUKS slot — the volume would be unrecoverable")
	assert.NotEqual(t, "psk-value", stored, "the PSK must not be what gets stored as the managed key")
	assert.Zero(t, storedAtRemoveCount,
		"the passphrase must reach control BEFORE the PSK slot is removed")
}

func TestVerifyKeyRoundTrip_DoesNotRetryCommittedMismatch(t *testing.T) {
	fakeEnc := &fakeEncManager{}
	// executor wiring is set below before the action runs

	ks := &fakeLuksKeyStore{
		getKeyFunc: func(_ context.Context, _ string) (string, error) {
			return "different-key", nil
		},
	}
	e := NewExecutor(nil)
	e.deps.encrypt = fakeEnc
	e.logger = slog.Default()
	e.now = time.Now
	e.SetLuksKeyStore(ks)

	err := e.verifyKeyRoundTrip(context.Background(), "01HXMISMATCH00000000000000", "/dev/mapper/test", "expected-key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "different key")
	assert.Equal(t, 1, ks.getKeyCalls, "a committed CRUD read mismatch is not eventual-consistency lag")
}

// The disconnected case, which is the one that actually happens in the field:
// no key store wired means no route to control.
func TestTakeOwnership_NotConnected_FailsClosedBeforeMutation(t *testing.T) {
	st, err := store.New(t.TempDir())
	require.NoError(t, err)
	defer st.Close()

	fakeEnc := &fakeEncManager{}
	// executor wiring is set below before the action runs

	e := NewExecutor(nil)
	e.deps.encrypt = fakeEnc
	e.logger = slog.Default()
	e.now = time.Now
	e.SetStore(st)
	// No key store wired: the agent is not connected.

	params := &pb.EncryptionParams{MinWords: 3}
	err = e.takeOwnership(context.Background(), params, "01HXNOCONN0000000000000000", "/dev/mapper/test", []byte("psk-value"))
	require.Error(t, err, "not connected → fail closed")
	assert.Zero(t, fakeEnc.addKeyCalls,
		"a volume must not be taken over while the passphrase cannot be reported")
}
