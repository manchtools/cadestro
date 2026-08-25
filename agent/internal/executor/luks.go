package executor

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysenc "github.com/manchtools/cadestro/sdk/sys/encryption"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"

	"github.com/manchtools/cadestro/agent/internal/store"
)

func luksSecret(s string) sysexec.Secret { return sysexec.NewMultilineSecret(s) }

func luksSecretBytes(b []byte) sysexec.Secret { return sysexec.NewMultilineSecret(string(b)) }

type LuksKeyStore interface {
	GetKey(ctx context.Context, actionID string) (string, error)
	StoreKey(ctx context.Context, actionID, devicePath, passphrase string, reason pb.RotationReason) error
}

func (e *Executor) requireLuksStoreReady() error {
	if e.getLuksKeyStore() == nil {
		return fmt.Errorf("no server connection; cannot store the new passphrase (fail closed, no cleartext-only rotation)")
	}
	return nil
}

const luksTimestampFailureThreshold = 3

func (e *Executor) recordLuksTimestampFailure(actionID, site string, err error) {
	e.luksTimestampFailMu.Lock()
	if e.luksTimestampFailCount == nil {
		e.luksTimestampFailCount = make(map[string]int)
	}
	e.luksTimestampFailCount[actionID]++
	n := e.luksTimestampFailCount[actionID]
	e.luksTimestampFailMu.Unlock()

	if n >= luksTimestampFailureThreshold {
		e.logger.Error("LUKS: SetLuksLastRotatedAt failing persistently — rotation may hot-loop or never start; investigate the agent store",
			"action_id", actionID,
			"site", site,
			"consecutive_failures", n,
			"error", err,
		)
		return
	}
	e.logger.Warn("LUKS: failed to persist rotation timestamp",
		"action_id", actionID,
		"site", site,
		"consecutive_failures", n,
		"error", err,
	)
}

func (e *Executor) clearLuksTimestampFailures(actionID string) {
	e.luksTimestampFailMu.Lock()
	if e.luksTimestampFailCount != nil {
		delete(e.luksTimestampFailCount, actionID)
	}
	e.luksTimestampFailMu.Unlock()
}

func (e *Executor) executeLuks(ctx context.Context, params *pb.EncryptionParams, state pb.DesiredState, actionID string, openPresharedKey func() ([]byte, error)) (*pb.CommandOutput, bool, map[string]string, error) {
	e.ensureDeps()
	if params == nil {
		return nil, false, nil, fmt.Errorf("luks params required")
	}
	if actionID == "" {
		return nil, false, nil, fmt.Errorf("action ID required for LUKS state tracking")
	}
	if e.getLuksKeyStore() == nil {
		return nil, false, nil, fmt.Errorf("LUKS key store not configured (no stream connection)")
	}
	if e.getStore() == nil {
		return nil, false, nil, fmt.Errorf("agent store not configured")
	}

	switch state {
	case pb.DesiredState_DESIRED_STATE_ABSENT:
		return e.removeLuksManagement(ctx, actionID)
	default:
		return e.setupLuks(ctx, params, actionID, openPresharedKey)
	}
}

func (e *Executor) removeLuksManagement(ctx context.Context, actionID string) (*pb.CommandOutput, bool, map[string]string, error) {
	st := e.getStore()
	if st == nil {
		return nil, false, nil, fmt.Errorf("agent store not configured")
	}
	localState, err := st.GetLuksState(ctx, actionID)
	if err != nil {

		e.logger.Error("removeLuksManagement: failed to read local state",
			"action_id", actionID, "error", err)
		return nil, false, nil, fmt.Errorf("get luks state: %w", err)
	}
	if localState != nil {
		if err := st.DeleteLuksState(ctx, actionID); err != nil {

			e.logger.Error("removeLuksManagement: failed to delete local state",
				"action_id", actionID, "error", err)
			return nil, false, nil, fmt.Errorf("delete luks state: %w", err)
		}
		return &pb.CommandOutput{
			ExitCode: 0,
			Stdout:   "LUKS: management removed, keys remain on device\n",
		}, true, nil, nil
	}

	return &pb.CommandOutput{
		ExitCode: 0,
		Stdout:   "LUKS: no managed state for this action, nothing to remove\n",
	}, false, nil, nil
}

func (e *Executor) setupLuks(ctx context.Context, params *pb.EncryptionParams, actionID string, openPresharedKey func() ([]byte, error)) (*pb.CommandOutput, bool, map[string]string, error) {
	e.ensureDeps()
	st := e.getStore()
	if st == nil {
		return nil, false, nil, fmt.Errorf("agent store not configured")
	}

	as := e.getActionStore()

	var output strings.Builder

	localState, err := st.GetLuksState(ctx, actionID)
	if err != nil {
		return nil, false, nil, fmt.Errorf("get luks state: %w", err)
	}

	if as != nil {
		winnerID, err := resolveLuksConflict(ctx, as, actionID)
		if err != nil {
			return nil, false, nil, fmt.Errorf("conflict resolution failed: %w", err)
		}
		if winnerID != actionID {
			return &pb.CommandOutput{
				ExitCode: 0,
				Stdout:   fmt.Sprintf("LUKS: skipped — another action %s takes precedence\n", winnerID),
			}, false, nil, nil
		}
	}

	var devicePath string
	var presharedKey []byte
	if localState != nil && localState.OwnershipTaken && localState.DevicePath != "" {

		devicePath = localState.DevicePath
		isLuks, err := e.deps.encrypt.IsEncrypted(ctx, devicePath)
		if err != nil {
			return nil, false, nil, fmt.Errorf("failed to check LUKS status: %w", err)
		}
		if !isLuks {
			return nil, false, nil, fmt.Errorf("previously managed device %s is no longer a LUKS volume", devicePath)
		}
		output.WriteString(fmt.Sprintf("LUKS: managing volume %s\n", devicePath))
	} else {
		if openPresharedKey == nil {
			return nil, false, nil, fmt.Errorf("encryption pre-shared key is not configured")
		}
		presharedKey, err = openPresharedKey()
		if err != nil {
			return nil, false, nil, err
		}
		defer clear(presharedKey)

		vol, err := e.deps.encrypt.DetectVolumeByKey(ctx, luksSecretBytes(presharedKey))
		if err != nil {

			vol, err = e.deps.encrypt.DetectVolume(ctx)
			if err != nil {
				return nil, false, nil, fmt.Errorf("no LUKS-encrypted volumes detected on this device")
			}
			output.WriteString(fmt.Sprintf("LUKS: detected volume %s (fallback)\n", vol.DevicePath))
		} else {
			output.WriteString(fmt.Sprintf("LUKS: matched volume %s by pre-shared key\n", vol.DevicePath))
		}
		devicePath = vol.DevicePath
	}

	changed := false

	if localState == nil || !localState.OwnershipTaken {
		if err := e.takeOwnership(ctx, params, actionID, devicePath, presharedKey); err != nil {
			return nil, false, nil, fmt.Errorf("failed to take ownership: %w", err)
		}
		output.WriteString("LUKS: ownership taken, managed passphrase set\n")
		changed = true

		var reloadErr error
		localState, reloadErr = st.GetLuksState(ctx, actionID)
		if reloadErr != nil {
			e.logger.Warn("failed to reload LUKS state after ownership", "action_id", actionID, "error", reloadErr)
		}
	}

	if localState != nil && localState.OwnershipTaken {
		rotated, err := e.checkAndRotate(ctx, params, localState, actionID, devicePath)
		if err != nil {
			e.logger.Warn("LUKS rotation failed", "action_id", actionID, "error", err)
			output.WriteString(fmt.Sprintf("LUKS: rotation check failed: %v\n", err))
		} else if rotated {
			output.WriteString("LUKS: managed passphrase rotated\n")
			changed = true

			if reloaded, rerr := st.GetLuksState(ctx, actionID); rerr == nil && reloaded != nil {
				localState = reloaded
			} else if rerr != nil {
				e.logger.Warn("LUKS: state reload after rotation failed; continuing with pre-rotation snapshot",
					"action_id", actionID, "error", rerr)
			}
		}
	}

	if localState != nil {
		keyChanged, err := e.reconcileDeviceKey(ctx, params, localState, actionID, devicePath)
		if err != nil {
			e.logger.Warn("LUKS device key reconciliation failed", "action_id", actionID, "error", err)
			output.WriteString(fmt.Sprintf("LUKS: device key reconciliation failed: %v\n", err))
		} else if keyChanged {
			output.WriteString("LUKS: device-bound key updated\n")
			changed = true
		}
	}

	return &pb.CommandOutput{
		ExitCode: 0,
		Stdout:   output.String(),
	}, changed, nil, nil
}

func (e *Executor) takeOwnership(ctx context.Context, params *pb.EncryptionParams, actionID, devicePath string, presharedKey []byte) error {
	e.ensureDeps()
	ks := e.getLuksKeyStore()
	if ks == nil {
		return fmt.Errorf("LUKS key store not configured (no stream connection)")
	}
	st := e.getStore()
	if st == nil {
		return fmt.Errorf("agent store not configured")
	}

	existingKey, getKeyErr := ks.GetKey(ctx, actionID)
	if getKeyErr == nil && existingKey != "" {
		e.logger.Info("LUKS: server has stored key, testing against volume",
			"action_id", actionID, "key_len", len(existingKey))
		ok, testErr := e.deps.encrypt.VerifyPassphrase(ctx, devicePath, luksSecret(existingKey))
		e.logger.Info("LUKS: test-passphrase result", "ok", ok, "error", testErr)
		if testErr == nil && ok {

			e.logger.Info("LUKS: recovered ownership from server-stored key", "action_id", actionID)
			return st.SetLuksOwnershipTaken(ctx, actionID, devicePath)
		}
		e.logger.Warn("LUKS: server has key but it does not unlock the volume, proceeding with PSK",
			"action_id", actionID, "test_error", testErr)
	} else if getKeyErr != nil {

		return fmt.Errorf("server not reachable, cannot manage LUKS keys (retry when connected): %w", getKeyErr)
	}

	if err := e.requireLuksStoreReady(); err != nil {
		return fmt.Errorf("take ownership: %w", err)
	}

	minWords := int(params.MinWords)
	if minWords < 3 {
		minWords = 5
	}

	passSecret, err := sysenc.GeneratePassphrase(minWords)
	if err != nil {
		return fmt.Errorf("generate passphrase: %w", err)
	}
	passphrase := passSecret.Reveal()

	e.logger.Info("LUKS: adding managed key using PSK",
		"psk_len", len(presharedKey),
		"new_key_len", len(passphrase))
	if err := e.deps.encrypt.AddKey(ctx, devicePath, luksSecretBytes(presharedKey), luksSecret(passphrase), sysenc.AddKeyOptions{}); err != nil {
		return fmt.Errorf("add managed key: %w", err)
	}

	if err := ks.StoreKey(ctx, actionID, devicePath, passphrase, pb.RotationReason_ROTATION_REASON_INITIAL); err != nil {

		if rmErr := e.deps.encrypt.RemoveKey(ctx, devicePath, luksSecret(passphrase)); rmErr != nil {
			e.logger.Error("LUKS: rollback failed — managed key remains in slot",
				"action_id", actionID, "error", rmErr)
		}
		return fmt.Errorf("store key on server: %w", err)
	}

	if err := e.verifyKeyRoundTrip(ctx, actionID, devicePath, passphrase); err != nil {
		return fmt.Errorf("round-trip verification failed, keeping both keys: %w", err)
	}

	if err := e.deps.encrypt.RemoveKey(ctx, devicePath, luksSecretBytes(presharedKey)); err != nil {
		e.logger.Warn("failed to remove PSK after ownership (both keys work)", "error", err)
	}

	return st.SetLuksOwnershipTaken(ctx, actionID, devicePath)
}

func (e *Executor) checkAndRotate(ctx context.Context, params *pb.EncryptionParams, localState *store.LuksState, actionID, devicePath string) (bool, error) {
	e.ensureDeps()
	ks := e.getLuksKeyStore()
	if ks == nil {
		return false, fmt.Errorf("LUKS key store not configured (no stream connection)")
	}
	st := e.getStore()
	if st == nil {
		return false, fmt.Errorf("agent store not configured")
	}

	if params.RotationIntervalDays > 0 {

		if localState.LastRotatedAt.IsZero() {
			if err := st.SetLuksLastRotatedAt(ctx, actionID, e.now().UTC()); err != nil {

				e.recordLuksTimestampFailure(actionID, "initial", err)
				return false, fmt.Errorf("persist initial LUKS rotation timestamp (rotation cannot start until this succeeds): %w", err)
			}
			e.clearLuksTimestampFailures(actionID)
			return false, nil
		}
		intervalDuration := time.Duration(params.RotationIntervalDays) * 24 * time.Hour
		if e.now().Sub(localState.LastRotatedAt) < intervalDuration {
			return false, nil
		}
	}

	if err := e.requireLuksStoreReady(); err != nil {
		return false, fmt.Errorf("rotate: %w", err)
	}

	currentKey, err := ks.GetKey(ctx, actionID)
	if err != nil {
		return false, fmt.Errorf("get current key: %w", err)
	}

	minWords := int(params.MinWords)
	if minWords < 3 {
		minWords = 5
	}

	newPassSecret, err := sysenc.GeneratePassphrase(minWords)
	if err != nil {
		return false, fmt.Errorf("generate passphrase: %w", err)
	}
	newPassphrase := newPassSecret.Reveal()

	if err := e.deps.encrypt.AddKey(ctx, devicePath, luksSecret(currentKey), luksSecret(newPassphrase), sysenc.AddKeyOptions{}); err != nil {
		return false, fmt.Errorf("add new key: %w", err)
	}

	if err := ks.StoreKey(ctx, actionID, devicePath, newPassphrase, pb.RotationReason_ROTATION_REASON_SCHEDULED); err != nil {

		if rmErr := e.deps.encrypt.RemoveKey(ctx, devicePath, luksSecret(newPassphrase)); rmErr != nil {
			e.logger.Error("LUKS: rotation rollback failed — new key remains in slot",
				"action_id", actionID, "error", rmErr)
		}
		return false, fmt.Errorf("store new key on server: %w", err)
	}

	if err := e.verifyKeyRoundTrip(ctx, actionID, devicePath, newPassphrase); err != nil {
		return false, fmt.Errorf("round-trip verification failed, keeping both keys: %w", err)
	}

	if err := e.deps.encrypt.RemoveKey(ctx, devicePath, luksSecret(currentKey)); err != nil {
		e.logger.Warn("failed to remove old key after rotation (both keys work)", "error", err)
	}

	if err := st.SetLuksLastRotatedAt(ctx, actionID, e.now().UTC()); err != nil {
		e.recordLuksTimestampFailure(actionID, "post_rotation", err)
	} else {
		e.clearLuksTimestampFailures(actionID)
	}

	return true, nil
}

func (e *Executor) reconcileDeviceKey(ctx context.Context, params *pb.EncryptionParams, localState *store.LuksState, actionID, devicePath string) (bool, error) {
	currentType := localState.DeviceKeyType
	desiredType := "none"
	switch params.DeviceBoundKeyType {
	case pb.EncryptionDeviceBoundKeyType_ENCRYPTION_DEVICE_BOUND_KEY_TYPE_TPM:
		desiredType = "tpm"
	case pb.EncryptionDeviceBoundKeyType_ENCRYPTION_DEVICE_BOUND_KEY_TYPE_USER_PASSPHRASE:
		desiredType = "user_passphrase"
	}

	if currentType == desiredType {
		return false, nil
	}

	if currentType != "none" {
		if err := e.revokeDeviceKeyInternal(ctx, localState, actionID); err != nil {
			return false, fmt.Errorf("revoke current device key: %w", err)
		}
	}

	switch desiredType {
	case "tpm":
		if err := e.enrollTpm(ctx, actionID, devicePath); err != nil {
			return false, fmt.Errorf("enroll TPM: %w", err)
		}

	case "user_passphrase":

		st := e.getStore()
		if st == nil {
			return false, fmt.Errorf("agent store not configured")
		}
		if err := st.SetLuksDeviceKeyType(ctx, actionID, "user_passphrase"); err != nil {
			return false, fmt.Errorf("persist user_passphrase device key type: %w", err)
		}
	}

	return true, nil
}

func (e *Executor) enrollTpm(ctx context.Context, actionID, devicePath string) error {
	ks := e.getLuksKeyStore()
	if ks == nil {
		return fmt.Errorf("LUKS key store not configured (no stream connection)")
	}
	st := e.getStore()
	if st == nil {
		return fmt.Errorf("agent store not configured")
	}

	tpm, ok := e.deps.encrypt.TPM()
	if !ok {
		return fmt.Errorf("TPM2 not supported by the encryption backend")
	}
	hasTPM, err := tpm.Available(ctx)
	if err != nil {
		return fmt.Errorf("check TPM2: %w", err)
	}
	if !hasTPM {
		return fmt.Errorf("TPM2 device not found")
	}

	managedKey, err := ks.GetKey(ctx, actionID)
	if err != nil {
		return fmt.Errorf("get managed key: %w", err)
	}

	if err := tpm.Enroll(ctx, devicePath, luksSecret(managedKey)); err != nil {
		return err
	}

	return st.SetLuksDeviceKeyType(ctx, actionID, "tpm")
}

func (e *Executor) revokeDeviceKeyInternal(ctx context.Context, localState *store.LuksState, actionID string) error {
	ks := e.getLuksKeyStore()
	if ks == nil {
		return fmt.Errorf("LUKS key store not configured (no stream connection)")
	}
	st := e.getStore()
	if st == nil {
		return fmt.Errorf("agent store not configured")
	}

	managedKey, err := ks.GetKey(ctx, actionID)
	if err != nil {
		return fmt.Errorf("get managed key: %w", err)
	}

	switch localState.DeviceKeyType {
	case "tpm":
		tpm, ok := e.deps.encrypt.TPM()
		if !ok {
			return fmt.Errorf("TPM2 not supported by the encryption backend")
		}
		if err := tpm.Wipe(ctx, localState.DevicePath, luksSecret(managedKey)); err != nil {
			return err
		}
	case "user_passphrase":
		if err := e.deps.encrypt.KillSlot(ctx, localState.DevicePath, 7, luksSecret(managedKey)); err != nil {
			return err
		}
	case "none":
		return nil
	}

	return st.SetLuksDeviceKeyType(ctx, actionID, "none")
}

func (e *Executor) RevokeLuksDeviceKey(ctx context.Context, actionID string) (bool, string) {
	st := e.getStore()
	ks := e.getLuksKeyStore()
	if st == nil {
		return false, "agent store not configured"
	}
	if ks == nil {
		return false, "LUKS key store not configured"
	}

	localState, err := st.GetLuksState(ctx, actionID)
	if err != nil {
		return false, fmt.Sprintf("failed to load LUKS state: %v", err)
	}
	if localState == nil {
		return true, ""
	}
	if localState.DeviceKeyType == "none" {
		return true, ""
	}

	if err := e.revokeDeviceKeyInternal(ctx, localState, actionID); err != nil {
		return false, fmt.Sprintf("failed to revoke device key: %v", err)
	}
	return true, ""
}

func resolveLuksConflict(ctx context.Context, as ActionStore, actionID string) (string, error) {
	if as == nil {

		return actionID, nil
	}
	stored, err := as.GetStoredActions(ctx)
	if err != nil {
		return actionID, nil
	}

	type luksCandidate struct {
		id         string
		minWords   int32
		complexity int32
		assignedAt time.Time
	}

	var candidates []luksCandidate
	for _, sa := range stored {
		if sa.Action.Type != pb.ActionType_ACTION_TYPE_ENCRYPTION {
			continue
		}
		if sa.Action.DesiredState == pb.DesiredState_DESIRED_STATE_ABSENT {
			continue
		}
		params := sa.Action.GetEncryption()
		if params == nil {
			continue
		}
		candidates = append(candidates, luksCandidate{
			id:         sa.ID,
			minWords:   params.MinWords,
			complexity: int32(params.UserPassphraseComplexity),
			assignedAt: sa.AssignedAt,
		})
	}

	if len(candidates) <= 1 {
		return actionID, nil
	}

	winner := slices.MaxFunc(candidates, func(a, b luksCandidate) int {
		if a.minWords != b.minWords {
			return int(a.minWords - b.minWords)
		}
		if a.complexity != b.complexity {
			return int(a.complexity - b.complexity)
		}

		if a.assignedAt.Before(b.assignedAt) {
			return 1
		}
		if a.assignedAt.After(b.assignedAt) {
			return -1
		}
		return 0
	})

	return winner.id, nil
}

func (e *Executor) verifyKeyRoundTrip(ctx context.Context, actionID, devicePath, expectedKey string) error {
	e.ensureDeps()
	ks := e.getLuksKeyStore()
	if ks == nil {
		return fmt.Errorf("LUKS key store not configured (no stream connection)")
	}

	storedKey, err := ks.GetKey(ctx, actionID)
	if err != nil {
		return fmt.Errorf("re-fetch stored key: %w", err)
	}
	if storedKey != expectedKey {
		return fmt.Errorf("server returned a different key than the committed value")
	}

	ok, testErr := e.deps.encrypt.VerifyPassphrase(ctx, devicePath, luksSecret(storedKey))
	if testErr != nil || !ok {
		return fmt.Errorf("server-stored key does not unlock volume (test_ok=%v, err=%v)", ok, testErr)
	}

	e.logger.Info("LUKS: round-trip verification passed", "action_id", actionID)
	return nil
}

type ActionStore interface {
	GetStoredActions(ctx context.Context) ([]*store.StoredAction, error)
}
