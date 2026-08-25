package crypto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manchtools/cadestro/server/internal/crypto"
)

func TestDeviceSecretAAD_BindsImmutableRowID(t *testing.T) {
	enc, err := crypto.NewEncryptor(testKey())
	require.NoError(t, err)

	const (
		deviceID = "01HDEVICEA"
		actionID = "01HACTIONA"
	)

	t.Run("a ciphertext opens only under its own row id", func(t *testing.T) {
		const rowID = "01HROWCURRENT"
		const secret = "administrator-password"
		ct, err := enc.EncryptWithContext(secret, crypto.DeviceSecretAAD(rowID, deviceID, "lps", actionID, 1))
		require.NoError(t, err)

		pt, err := enc.DecryptWithContext(ct, crypto.DeviceSecretAAD(rowID, deviceID, "lps", actionID, 1))
		require.NoError(t, err, "the same row must open its own ciphertext")
		assert.Equal(t, secret, pt)

		_, err = enc.DecryptWithContext(ct, crypto.DeviceSecretAAD("01HROWSIBLING", deviceID, "lps", actionID, 1))
		assert.Error(t, err,
			"a ciphertext sealed for one row must not open under a sibling row id sharing the device and action")
	})

	t.Run("sibling rotation rows for one username are not interchangeable", func(t *testing.T) {

		const currentRow, historicalRow = "01HROWNEW", "01HROWOLD"
		ctOld, err := enc.EncryptWithContext("retired-secret",
			crypto.DeviceSecretAAD(historicalRow, deviceID, "lps", actionID, 1))
		require.NoError(t, err)

		_, err = enc.DecryptWithContext(ctOld, crypto.DeviceSecretAAD(currentRow, deviceID, "lps", actionID, 1))
		assert.Error(t, err,
			"a retired rotation row's ciphertext must not open under the current row's context")

		pt, err := enc.DecryptWithContext(ctOld, crypto.DeviceSecretAAD(historicalRow, deviceID, "lps", actionID, 1))
		require.NoError(t, err, "the retired row must still open under its own id")
		assert.Equal(t, "retired-secret", pt)
	})

	t.Run("LUKS rows bind the row id too", func(t *testing.T) {
		const rowID = "01HLUKSROW"
		const secret = "a-real-luks-passphrase"
		ct, err := enc.EncryptWithContext(secret, crypto.DeviceSecretAAD(rowID, deviceID, "luks", actionID, 1))
		require.NoError(t, err)

		pt, err := enc.DecryptWithContext(ct, crypto.DeviceSecretAAD(rowID, deviceID, "luks", actionID, 1))
		require.NoError(t, err, "the same row must open its own ciphertext")
		assert.Equal(t, secret, pt)

		_, err = enc.DecryptWithContext(ct, crypto.DeviceSecretAAD("01HLUKSROWB", deviceID, "luks", actionID, 1))
		assert.Error(t, err,
			"a LUKS ciphertext sealed for one row must not open under a sibling row id")
	})
}
