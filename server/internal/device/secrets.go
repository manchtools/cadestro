package device

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/types/known/timestamppb"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/manchtools/cadestro/server/internal/actionparams"
	"github.com/manchtools/cadestro/server/internal/auth"
	pmcrypto "github.com/manchtools/cadestro/server/internal/crypto"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

const luksTokenTTL = 24 * time.Hour

// ListLpsPasswords returns bounded current and historical LPS metadata. Its
// store query does not select ciphertext.
func (h *Handlers) ListLpsPasswords(ctx context.Context, req *connect.Request[cadestrov1.ListLpsPasswordsRequest]) (*connect.Response[cadestrov1.ListLpsPasswordsResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := h.readDevice(ctx, "ListLpsPasswords", req.Msg.DeviceId); err != nil {
		return nil, err
	}
	currentRows, historyRows, err := h.store.ListDeviceLpsPasswords(ctx, req.Msg.DeviceId)
	if err != nil {
		return nil, h.internal(ctx, "read LPS passwords", err)
	}
	current, err := h.lpsPasswordsToProto(currentRows)
	if err != nil {
		return nil, h.internal(ctx, "decode current LPS passwords", err)
	}
	history, err := h.lpsPasswordsToProto(historyRows)
	if err != nil {
		return nil, h.internal(ctx, "decode historical LPS passwords", err)
	}
	if err := h.recordSensitiveRead(ctx, req, actor,
		cadestrov1connect.ControlServiceListLpsPasswordsProcedure,
		"ListLpsPasswords", "device_lps_passwords", req.Msg.DeviceId); err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.ListLpsPasswordsResponse{Current: current, History: history}), nil
}

// RevealLpsPassword returns one plaintext password only after the dedicated
// reveal operation and its device/action/entry effects are durable.
func (h *Handlers) RevealLpsPassword(ctx context.Context, req *connect.Request[cadestrov1.RevealLpsPasswordRequest]) (*connect.Response[cadestrov1.RevealLpsPasswordResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "RevealLpsPassword", ""); err != nil {
		return nil, err
	}
	secret, err := h.store.GetLpsPasswordForReveal(ctx, req.Msg.Id)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, errLpsPasswordNotFound, "LPS password not found")
		}
		return nil, h.internal(ctx, "read LPS password for reveal", err)
	}
	if _, err := h.readDevice(ctx, "RevealLpsPassword", secret.DeviceID); err != nil {
		return nil, err
	}
	stored, err := h.store.GetDeviceSecret(ctx, secret.ID)
	if err != nil || stored.DeviceID != secret.DeviceID || stored.Kind != "lps" {
		if err == nil {
			err = errors.New("generic LPS secret owner mismatch")
		}
		return nil, h.internal(ctx, "read generic LPS password", err)
	}
	password, err := h.openStoredSecret(stored.Ciphertext,
		pmcrypto.DeviceSecretAAD(stored.ID, stored.DeviceID, stored.Kind, stored.Subject, uint32(stored.Version)))
	if err != nil {
		return nil, h.internal(ctx, "open LPS password", err)
	}
	if err := h.recordSecretReveal(ctx, req, actor,
		cadestrov1connect.ControlServiceRevealLpsPasswordProcedure,
		"RevealLpsPassword", "lps_password", secret.ID, secret.DeviceID, secret.ActionID); err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.RevealLpsPasswordResponse{Password: password}), nil
}

// ListLuksKeys returns bounded current and historical LUKS metadata. Its store
// query does not select ciphertext.
func (h *Handlers) ListLuksKeys(ctx context.Context, req *connect.Request[cadestrov1.ListLuksKeysRequest]) (*connect.Response[cadestrov1.ListLuksKeysResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := h.readDevice(ctx, "ListLuksKeys", req.Msg.DeviceId); err != nil {
		return nil, err
	}
	currentRows, historyRows, err := h.store.ListDeviceLuksKeys(ctx, req.Msg.DeviceId)
	if err != nil {
		return nil, h.internal(ctx, "read LUKS keys", err)
	}
	current, err := h.luksKeysToProto(currentRows)
	if err != nil {
		return nil, h.internal(ctx, "decode current LUKS keys", err)
	}
	history, err := h.luksKeysToProto(historyRows)
	if err != nil {
		return nil, h.internal(ctx, "decode historical LUKS keys", err)
	}
	if err := h.recordSensitiveRead(ctx, req, actor,
		cadestrov1connect.ControlServiceListLuksKeysProcedure,
		"ListLuksKeys", "device_luks_keys", req.Msg.DeviceId); err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.ListLuksKeysResponse{Current: current, History: history}), nil
}

// RevealLuksKey returns one plaintext passphrase only after the dedicated
// reveal operation and its device/action/entry effects are durable.
func (h *Handlers) RevealLuksKey(ctx context.Context, req *connect.Request[cadestrov1.RevealLuksKeyRequest]) (*connect.Response[cadestrov1.RevealLuksKeyResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "RevealLuksKey", ""); err != nil {
		return nil, err
	}
	secret, err := h.store.GetLuksKeyForReveal(ctx, req.Msg.Id)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, errLuksKeyNotFound, "LUKS key not found")
		}
		return nil, h.internal(ctx, "read LUKS key for reveal", err)
	}
	if _, err := h.readDevice(ctx, "RevealLuksKey", secret.DeviceID); err != nil {
		return nil, err
	}
	stored, err := h.store.GetDeviceSecret(ctx, secret.ID)
	if err != nil || stored.DeviceID != secret.DeviceID || stored.Kind != "luks" {
		if err == nil {
			err = errors.New("generic LUKS secret owner mismatch")
		}
		return nil, h.internal(ctx, "read generic LUKS key", err)
	}
	passphrase, err := h.openStoredSecret(stored.Ciphertext,
		pmcrypto.DeviceSecretAAD(stored.ID, stored.DeviceID, stored.Kind, stored.Subject, uint32(stored.Version)))
	if err != nil {
		return nil, h.internal(ctx, "open LUKS passphrase", err)
	}
	if err := h.recordSecretReveal(ctx, req, actor,
		cadestrov1connect.ControlServiceRevealLuksKeyProcedure,
		"RevealLuksKey", "luks_key", secret.ID, secret.DeviceID, secret.ActionID); err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.RevealLuksKeyResponse{Passphrase: passphrase}), nil
}

func (h *Handlers) lpsPasswordsToProto(rows []store.LpsPasswordView) ([]*cadestrov1.LpsPassword, error) {
	out := make([]*cadestrov1.LpsPassword, len(rows))
	for i, row := range rows {
		reason, ok := rotationReasonFromString(row.RotationReason)
		if !ok {
			return nil, fmt.Errorf("invalid LPS rotation reason %q", row.RotationReason)
		}
		out[i] = &cadestrov1.LpsPassword{
			Id: row.ID, DeviceId: row.DeviceID, DeviceHostname: row.DeviceHostname,
			ActionId: row.ActionID, ActionName: row.ActionName,
			Username:  row.Username,
			RotatedAt: timestamppb.New(row.RotatedAt), RotationReason: reason,
		}
	}
	return out, nil
}

func (h *Handlers) luksKeysToProto(rows []store.LuksKeyView) ([]*cadestrov1.LuksKey, error) {
	out := make([]*cadestrov1.LuksKey, len(rows))
	for i, row := range rows {
		reason, ok := rotationReasonFromString(row.RotationReason)
		if !ok {
			return nil, fmt.Errorf("invalid LUKS rotation reason %q", row.RotationReason)
		}
		key := &cadestrov1.LuksKey{
			Id: row.ID, DeviceId: row.DeviceID, DeviceHostname: row.DeviceHostname,
			ActionId: row.ActionID, ActionName: row.ActionName,
			DevicePath: row.DevicePath,
			RotatedAt:  timestamppb.New(row.RotatedAt), RotationReason: reason,
		}
		if row.RevocationStatus != nil {
			status, ok := luksRevocationStatusFromString(*row.RevocationStatus)
			if !ok {
				return nil, fmt.Errorf("invalid LUKS revocation status %q", *row.RevocationStatus)
			}
			key.RevocationStatus = status
		}
		if row.RevocationError != nil {
			key.RevocationError = *row.RevocationError
		}
		if row.RevocationAt != nil {
			key.RevocationAt = timestamppb.New(*row.RevocationAt)
		}
		out[i] = key
	}
	return out, nil
}

func (h *Handlers) recordSecretReveal(
	ctx context.Context,
	req connect.AnyRequest,
	actor *auth.UserContext,
	procedure, permission, secretType, secretID, deviceID, actionID string,
) error {
	op := h.operation(req, actor, procedure, permission)
	op.Class = store.ClassSensitiveRead
	if _, err := h.store.RecordOperation(ctx, op,
		store.AuditEffect{ResourceType: secretType, ResourceID: secretID, Action: "REVEAL", Outcome: store.EffectApplied},
		store.AuditEffect{ResourceType: "device", ResourceID: deviceID, Action: "REVEAL", Outcome: store.EffectApplied},
		store.AuditEffect{ResourceType: "action", ResourceID: actionID, Action: "REVEAL", Outcome: store.EffectApplied},
	); err != nil {
		return h.internal(ctx, "record secret reveal", err)
	}
	return nil
}

// CreateLuksToken atomically persists a hash of a one-time owner token with
// its audit evidence. The plaintext is returned exactly once.
func (h *Handlers) CreateLuksToken(ctx context.Context, req *connect.Request[cadestrov1.CreateLuksTokenRequest]) (*connect.Response[cadestrov1.CreateLuksTokenResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := h.mutationDevice(ctx, "CreateLuksToken", req.Msg.DeviceId); err != nil {
		return nil, err
	}
	owned, err := h.store.IsDeviceDirectlyAssignedToUser(ctx, req.Msg.DeviceId, actor.ID)
	if err != nil {
		return nil, h.internal(ctx, "check LUKS token owner", err)
	}
	if !owned {
		return nil, rpcError(ctx, errPermissionDenied, connect.CodePermissionDenied,
			"only the directly assigned device owner can create a LUKS passphrase token")
	}
	action, err := h.store.GetManifestAction(ctx, req.Msg.ActionId)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, errActionNotFound, "action not found")
		}
		return nil, h.internal(ctx, "read LUKS token action", err)
	}
	if cadestrov1.ActionType(action.ActionType) != cadestrov1.ActionType_ACTION_TYPE_ENCRYPTION {
		return nil, rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument,
			"action is not an encryption action")
	}
	var params cadestrov1.EncryptionAuthoringParams
	if err := actionparams.UnmarshalActionParams(action.Params, &params); err != nil {
		return nil, h.internal(ctx, "decode encryption action params", err)
	}
	minLength := params.UserPassphraseMinLength
	if minLength < 16 {
		minLength = 16
	}
	if _, ok := cadestrov1.LpsPasswordComplexity_name[int32(params.UserPassphraseComplexity)]; !ok {
		return nil, h.internal(ctx, "decode encryption action params",
			fmt.Errorf("invalid passphrase complexity %d", params.UserPassphraseComplexity))
	}

	issuedAt := h.now().UTC()
	tokenID, err := ulid.New(ulid.Timestamp(issuedAt), rand.Reader)
	if err != nil {
		return nil, h.internal(ctx, "generate LUKS token", err)
	}
	token := tokenID.String()
	hash := sha256.Sum256([]byte(token))
	expiresAt := issuedAt.Add(luksTokenTTL)
	rowID := ulid.Make().String()
	_, err = h.store.WithAudit(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceCreateLuksTokenProcedure, "CreateLuksToken"),
		func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
			if _, err := tx.InsertLuksToken(ctx, db.InsertLuksTokenParams{
				ID: rowID, DeviceID: req.Msg.DeviceId, ActionID: req.Msg.ActionId,
				Token: hex.EncodeToString(hash[:]), MinLength: minLength,
				Complexity: int32(params.UserPassphraseComplexity),
				CreatedAt:  issuedAt, ExpiresAt: expiresAt,
			}); err != nil {
				return fmt.Errorf("insert LUKS token: %w", err)
			}
			rec.Effect(store.AuditEffect{
				ResourceType: "luks_token", ResourceID: rowID,
				Action: "CREATE", Outcome: store.EffectApplied,
				ChangedFields: []string{
					"action_id", "complexity", "device_id", "expires_at", "min_length",
				},
			})
			return nil
		})
	if err != nil {
		return nil, h.internal(ctx, "create LUKS token", err)
	}
	return connect.NewResponse(&cadestrov1.CreateLuksTokenResponse{
		Token: token,
		Uri:   "cadestro://luks/set-passphrase?token=" + token,
		// The advertised command carries NEITHER the token nor sudo (F2).
		// /proc/<pid>/cmdline is world-readable and the client collects the
		// passphrase before it dials, so a token on argv was exposed for the
		// whole typing window while being the sole authorization for a root
		// daemon that writes LUKS keyslots. The client prompts for the token
		// instead (or takes --token-file / $CADESTRO_LUKS_TOKEN). sudo is gone
		// because the sudoers rule was removed to make this client
		// unprivileged; an operator copying it back would reinstate the
		// escalation the daemon exists to remove. Token is returned as its own
		// field for the UI to display.
		CliCommand: "cadestrod luks set-passphrase",
	}), nil
}

func (h *Handlers) openStoredSecret(ciphertext string, aad []byte) (string, error) {
	if !strings.HasPrefix(ciphertext, "enc:v1:") {
		return "", fmt.Errorf("stored secret is not current ciphertext")
	}
	return h.decryptor.DecryptWithContext(ciphertext, aad)
}

func rotationReasonFromString(value string) (cadestrov1.RotationReason, bool) {
	switch value {
	case "initial":
		return cadestrov1.RotationReason_ROTATION_REASON_INITIAL, true
	case "scheduled":
		return cadestrov1.RotationReason_ROTATION_REASON_SCHEDULED, true
	case "auth_grace":
		return cadestrov1.RotationReason_ROTATION_REASON_AUTH_GRACE, true
	default:
		return cadestrov1.RotationReason_ROTATION_REASON_UNSPECIFIED, false
	}
}

func luksRevocationStatusFromString(value string) (cadestrov1.LuksRevocationStatus, bool) {
	switch value {
	case "none":
		return cadestrov1.LuksRevocationStatus_LUKS_REVOCATION_STATUS_NONE, true
	case "dispatched":
		return cadestrov1.LuksRevocationStatus_LUKS_REVOCATION_STATUS_DISPATCHED, true
	case "success":
		return cadestrov1.LuksRevocationStatus_LUKS_REVOCATION_STATUS_SUCCESS, true
	case "failed":
		return cadestrov1.LuksRevocationStatus_LUKS_REVOCATION_STATUS_FAILED, true
	default:
		return cadestrov1.LuksRevocationStatus_LUKS_REVOCATION_STATUS_UNSPECIFIED, false
	}
}
