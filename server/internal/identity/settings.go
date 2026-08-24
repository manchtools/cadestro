package identity

import (
	"context"

	"connectrpc.com/connect"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

const serverSettingsID = "00000000000000000000000003"

// GetServerSettings returns the singleton fleet settings row.
func (h *Handlers) GetServerSettings(ctx context.Context, req *connect.Request[cadestrov1.GetServerSettingsRequest]) (*connect.Response[cadestrov1.GetServerSettingsResponse], error) {
	if _, err := h.requireActor(ctx); err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, PermGetServerSettings, ""); err != nil {
		return nil, err
	}
	row, err := h.store.GetServerSettings(ctx)
	if err != nil {
		return nil, internalError(ctx, "failed to load server settings")
	}
	return connect.NewResponse(&cadestrov1.GetServerSettingsResponse{Settings: settingsToProto(row)}), nil
}

// UpdateServerSettings replaces the two fleet-wide toggles directly.
func (h *Handlers) UpdateServerSettings(ctx context.Context, req *connect.Request[cadestrov1.UpdateServerSettingsRequest]) (*connect.Response[cadestrov1.UpdateServerSettingsResponse], error) {
	actor, err := h.requireActor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, PermUpdateServerSettings, ""); err != nil {
		return nil, err
	}

	at := h.now().UTC()
	var updated store.ServerSettingsRow
	_, err = h.store.WithAudit(ctx, h.mutationOp(req, actor, PermUpdateServerSettings),
		func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
			before, err := tx.GetServerSettings(ctx)
			if err != nil {
				return err
			}
			updated, err = tx.UpdateServerSettings(ctx, db.UpdateServerSettingsParams{
				UserProvisioningEnabled: req.Msg.UserProvisioningEnabled,
				SshAccessForAll:         req.Msg.SshAccessForAll,
				UpdatedAt:               at,
			})
			if err != nil {
				return err
			}
			beforeProvisioning, afterProvisioning := before.UserProvisioningEnabled, updated.UserProvisioningEnabled
			rec.Effect(store.AuditEffect{
				ResourceType: "server_settings", ResourceID: serverSettingsID,
				Action: "SET_USER_PROVISIONING", Outcome: store.EffectApplied,
				ChangedFields: []string{"user_provisioning_enabled"},
				BeforeFlag:    &beforeProvisioning, AfterFlag: &afterProvisioning,
			})
			beforeSSH, afterSSH := before.SshAccessForAll, updated.SshAccessForAll
			rec.Effect(store.AuditEffect{
				ResourceType: "server_settings", ResourceID: serverSettingsID,
				Action: "SET_SSH_ACCESS", Outcome: store.EffectApplied,
				ChangedFields: []string{"ssh_access_for_all"},
				BeforeFlag:    &beforeSSH, AfterFlag: &afterSSH,
			})
			return nil
		})
	if err != nil {
		return nil, internalError(ctx, "failed to update server settings")
	}
	return connect.NewResponse(&cadestrov1.UpdateServerSettingsResponse{Settings: settingsToProto(updated)}), nil
}

func settingsToProto(row store.ServerSettingsRow) *cadestrov1.ServerSettings {
	return &cadestrov1.ServerSettings{
		UserProvisioningEnabled: row.UserProvisioningEnabled,
		SshAccessForAll:         row.SshAccessForAll,
	}
}
