package device

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

func requireOne(operation string, rows int64, err error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if rows != 1 {
		return fmt.Errorf("%s: affected %d rows", operation, rows)
	}
	return nil
}

func deviceEffect(deviceID, action string, fields ...string) store.AuditEffect {
	return store.AuditEffect{
		ResourceType: "device", ResourceID: deviceID, Action: action,
		Outcome: store.EffectApplied, ChangedFields: fields,
	}
}

func (h *Handlers) SetDeviceLabel(ctx context.Context, req *connect.Request[cadestrov1.SetDeviceLabelRequest]) (*connect.Response[cadestrov1.UpdateDeviceResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "SetDeviceLabel", req.Msg.GetId().GetValue()); err != nil {
		return nil, err
	}
	if _, err := h.store.GetDevice(ctx, req.Msg.GetId().GetValue()); err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, errDeviceNotFound, "device not found")
		}
		return nil, h.internal(ctx, "read label target", err)
	}
	_, err = h.store.WithAudit(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceSetDeviceLabelProcedure, "SetDeviceLabel"),
		func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
			n, err := tx.SetDeviceLabel(ctx, db.SetDeviceLabelParams{
				DeviceID: req.Msg.GetId().GetValue(), Key: req.Msg.Key, Value: req.Msg.Value,
			})
			if err := requireOne("set device label", n, err); err != nil {
				return err
			}
			rec.Effect(deviceEffect(req.Msg.GetId().GetValue(), "UPDATE", "labels"))
			return nil
		})
	if err != nil {
		return nil, h.internal(ctx, "set device label", err)
	}
	return h.updatedDevice(ctx, req.Msg.GetId().GetValue())
}

// RemoveDeviceLabel removes one label. Missing labels are an idempotent success.
func (h *Handlers) RemoveDeviceLabel(ctx context.Context, req *connect.Request[cadestrov1.RemoveDeviceLabelRequest]) (*connect.Response[cadestrov1.UpdateDeviceResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "RemoveDeviceLabel", req.Msg.GetId().GetValue()); err != nil {
		return nil, err
	}
	if _, err := h.store.GetDevice(ctx, req.Msg.GetId().GetValue()); err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, errDeviceNotFound, "device not found")
		}
		return nil, h.internal(ctx, "read label target", err)
	}
	_, err = h.store.WithAudit(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceRemoveDeviceLabelProcedure, "RemoveDeviceLabel"),
		func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
			n, err := tx.RemoveDeviceLabel(ctx, db.RemoveDeviceLabelParams{DeviceID: req.Msg.GetId().GetValue(), Key: req.Msg.Key})
			if err != nil {
				return fmt.Errorf("remove device label: %w", err)
			}
			if n == 1 {
				rec.Effect(deviceEffect(req.Msg.GetId().GetValue(), "UPDATE", "labels"))
			}
			return nil
		})
	if err != nil {
		return nil, h.internal(ctx, "remove device label", err)
	}
	return h.updatedDevice(ctx, req.Msg.GetId().GetValue())
}

// AssignDevice assigns distinct users and groups with one audited transaction.
func (h *Handlers) AssignDevice(ctx context.Context, req *connect.Request[cadestrov1.AssignDeviceRequest]) (*connect.Response[cadestrov1.AssignDeviceResponse], error) {
	deviceID := req.Msg.GetDeviceId().GetValue()
	userIDs := make([]string, 0, len(req.Msg.GetUserIds()))
	for _, id := range req.Msg.GetUserIds() {
		userIDs = append(userIDs, id.GetValue())
	}
	userIDs = distinct(userIDs, req.Msg.GetUserId().GetValue())
	groupIDs := make([]string, 0, len(req.Msg.GetGroupIds()))
	for _, id := range req.Msg.GetGroupIds() {
		groupIDs = append(groupIDs, id.GetValue())
	}
	groupIDs = distinct(groupIDs, req.Msg.GetGroupId().GetValue())
	if len(userIDs) == 0 && len(groupIDs) == 0 {
		return nil, rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument, "at least one user or group is required")
	}
	if len(userIDs) > 256 || len(groupIDs) > 256 {
		return nil, rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument, "too many users or groups")
	}
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "AssignDevice", deviceID); err != nil {
		return nil, err
	}
	view, err := h.store.GetDeviceView(ctx, deviceID)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, errDeviceNotFound, "device not found")
		}
		return nil, h.internal(ctx, "read assignment target", err)
	}
	for _, id := range userIDs {
		if _, err := h.store.GetUser(ctx, id); err != nil {
			if store.IsNotFound(err) {
				return nil, notFound(ctx, errUserNotFound, "user not found")
			}
			return nil, h.internal(ctx, "read assignment user", err)
		}
	}
	for _, id := range groupIDs {
		if _, err := h.store.GetUserGroup(ctx, id); err != nil {
			if store.IsNotFound(err) {
				return nil, notFound(ctx, errUserGroupMissing, "user group not found")
			}
			return nil, h.internal(ctx, "read assignment group", err)
		}
	}
	existingUsers := set(view.AssignedUserIDs)
	existingGroups := set(view.AssignedGroupIDs)
	_, err = h.store.WithAudit(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceAssignDeviceProcedure, "AssignDevice"),
		func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
			for _, id := range userIDs {
				if existingUsers[id] {
					continue
				}
				n, err := tx.AssignDeviceUser(ctx, db.AssignDeviceUserParams{
					DeviceID: deviceID, UserID: id, AssignedAt: h.now().UTC(), AssignedBy: actor.ID,
				})
				if err := requireOne("assign device user", n, err); err != nil {
					return err
				}
				effect := deviceEffect(deviceID, "ASSIGN", "assigned_user_ids")
				effect.AfterRef = &id
				rec.Effect(effect)
			}
			for _, id := range groupIDs {
				if existingGroups[id] {
					continue
				}
				n, err := tx.AssignDeviceGroup(ctx, db.AssignDeviceGroupParams{
					DeviceID: deviceID, GroupID: id, AssignedAt: h.now().UTC(), AssignedBy: actor.ID,
				})
				if err := requireOne("assign device group", n, err); err != nil {
					return err
				}
				effect := deviceEffect(deviceID, "ASSIGN", "assigned_group_ids")
				effect.AfterRef = &id
				rec.Effect(effect)
			}
			return nil
		})
	if err != nil {
		return nil, h.internal(ctx, "assign device", err)
	}
	updated, err := h.store.GetDeviceView(ctx, deviceID)
	if err != nil {
		return nil, h.internal(ctx, "read assigned device", err)
	}
	return connect.NewResponse(&cadestrov1.AssignDeviceResponse{Device: h.toProto(updated)}), nil
}

// UnassignDevice removes exactly one user or group assignment.
func (h *Handlers) UnassignDevice(ctx context.Context, req *connect.Request[cadestrov1.UnassignDeviceRequest]) (*connect.Response[cadestrov1.UnassignDeviceResponse], error) {
	deviceID := req.Msg.GetDeviceId().GetValue()
	if (req.Msg.GetUserId().GetValue() == "") == (req.Msg.GetGroupId().GetValue() == "") {
		return nil, rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument, "exactly one user or group is required")
	}
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "UnassignDevice", deviceID); err != nil {
		return nil, err
	}
	if _, err := h.store.GetDevice(ctx, deviceID); err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, errDeviceNotFound, "device not found")
		}
		return nil, h.internal(ctx, "read unassignment target", err)
	}
	_, err = h.store.WithAudit(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceUnassignDeviceProcedure, "UnassignDevice"),
		func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
			var n int64
			var err error
			var field, ref string
			if req.Msg.GetUserId().GetValue() != "" {
				ref, field = req.Msg.GetUserId().GetValue(), "assigned_user_ids"
				n, err = tx.UnassignDeviceUser(ctx, db.UnassignDeviceUserParams{DeviceID: deviceID, UserID: ref})
			} else {
				ref, field = req.Msg.GetGroupId().GetValue(), "assigned_group_ids"
				n, err = tx.UnassignDeviceGroup(ctx, db.UnassignDeviceGroupParams{DeviceID: deviceID, GroupID: ref})
			}
			if err != nil {
				return fmt.Errorf("unassign device: %w", err)
			}
			if n == 1 {
				effect := deviceEffect(deviceID, "UNASSIGN", field)
				effect.BeforeRef = &ref
				rec.Effect(effect)
			}
			return nil
		})
	if err != nil {
		return nil, h.internal(ctx, "unassign device", err)
	}
	view, err := h.store.GetDeviceView(ctx, deviceID)
	if err != nil {
		return nil, h.internal(ctx, "read unassigned device", err)
	}
	return connect.NewResponse(&cadestrov1.UnassignDeviceResponse{Device: h.toProto(view)}), nil
}

// SetDeviceSyncInterval writes the device-level sync override directly.
func (h *Handlers) SetDeviceSyncInterval(ctx context.Context, req *connect.Request[cadestrov1.SetDeviceSyncIntervalRequest]) (*connect.Response[cadestrov1.UpdateDeviceResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := h.mutationDevice(ctx, "SetDeviceSyncInterval", req.Msg.GetId().GetValue()); err != nil {
		return nil, err
	}
	_, err = h.store.WithAudit(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceSetDeviceSyncIntervalProcedure, "SetDeviceSyncInterval"),
		func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
			n, err := tx.SetDeviceSyncInterval(ctx, db.SetDeviceSyncIntervalParams{ID: req.Msg.GetId().GetValue(), Minutes: req.Msg.SyncIntervalMinutes})
			if err := requireOne("set device sync interval", n, err); err != nil {
				return err
			}
			rec.Effect(deviceEffect(req.Msg.GetId().GetValue(), "UPDATE", "sync_interval_minutes"))
			return nil
		})
	if err != nil {
		return nil, h.internal(ctx, "set device sync interval", err)
	}
	return h.updatedDevice(ctx, req.Msg.GetId().GetValue())
}

// SetDeviceInventoryInterval writes the device-level inventory override.
func (h *Handlers) SetDeviceInventoryInterval(ctx context.Context, req *connect.Request[cadestrov1.SetDeviceInventoryIntervalRequest]) (*connect.Response[cadestrov1.UpdateDeviceResponse], error) {
	if minutes := req.Msg.InventoryIntervalMinutes; minutes != 0 && (minutes < 120 || minutes > 10080) {
		return nil, rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument, "inventory interval is out of range")
	}
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := h.mutationDevice(ctx, "SetDeviceInventoryInterval", req.Msg.GetId().GetValue()); err != nil {
		return nil, err
	}
	_, err = h.store.WithAudit(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceSetDeviceInventoryIntervalProcedure, "SetDeviceInventoryInterval"),
		func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
			n, err := tx.SetDeviceInventoryInterval(ctx, db.SetDeviceInventoryIntervalParams{ID: req.Msg.GetId().GetValue(), Minutes: req.Msg.InventoryIntervalMinutes})
			if err := requireOne("set device inventory interval", n, err); err != nil {
				return err
			}
			rec.Effect(deviceEffect(req.Msg.GetId().GetValue(), "UPDATE", "inventory_interval_minutes"))
			return nil
		})
	if err != nil {
		return nil, h.internal(ctx, "set device inventory interval", err)
	}
	return h.updatedDevice(ctx, req.Msg.GetId().GetValue())
}

// DeleteDevice atomically soft-deletes the device, revokes its current
// certificate, and records the audit effect. The active stream closes only
// after that transaction commits.
func (h *Handlers) DeleteDevice(ctx context.Context, req *connect.Request[cadestrov1.DeleteDeviceRequest]) (*connect.Response[cadestrov1.DeleteDeviceResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	view, err := h.mutationDevice(ctx, "DeleteDevice", req.Msg.GetId().GetValue())
	if err != nil {
		return nil, err
	}
	_, err = h.store.WithAudit(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceDeleteDeviceProcedure, "DeleteDevice"),
		func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
			n, err := tx.SoftDeleteDevice(ctx, req.Msg.GetId().GetValue())
			if err := requireOne("delete device", n, err); err != nil {
				return err
			}
			effect := deviceEffect(req.Msg.GetId().GetValue(), "DELETE", "is_deleted")
			before, after := false, true
			effect.BeforeFlag, effect.AfterFlag = &before, &after
			if view.ActiveCertSerial != nil {
				effect.EvidenceKind = "certificate_serial"
				effect.EvidenceFingerprint = *view.ActiveCertSerial
			}
			rec.Effect(effect)
			return nil
		})
	if err != nil {
		return nil, h.internal(ctx, "delete device", err)
	}
	h.closeStream(req.Msg.GetId().GetValue())
	return connect.NewResponse(&cadestrov1.DeleteDeviceResponse{}), nil
}

func (h *Handlers) updatedDevice(ctx context.Context, deviceID string) (*connect.Response[cadestrov1.UpdateDeviceResponse], error) {
	view, err := h.store.GetDeviceView(ctx, deviceID)
	if err != nil {
		return nil, h.internal(ctx, "read updated device", err)
	}
	return connect.NewResponse(&cadestrov1.UpdateDeviceResponse{Device: h.toProto(view)}), nil
}

func distinct(ids []string, extra string) []string {
	seen := make(map[string]bool, len(ids)+1)
	out := make([]string, 0, len(ids)+1)
	for _, id := range append(append([]string(nil), ids...), extra) {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func set(ids []string) map[string]bool {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}
