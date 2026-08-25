package devicegroup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/oklog/ulid/v2"

	"github.com/manchtools/cadestro/server/internal/dynamicquery"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

const maxBatchDevices = 256

var (
	ErrInvalidInput   = errors.New("invalid device group input")
	ErrInvalidQuery   = errors.New("invalid device group query")
	ErrStaticGroup    = errors.New("static device group has no dynamic query")
	ErrDynamicGroup   = errors.New("dynamic group membership is evaluator-owned")
	ErrMemberNotFound = errors.New("device group member not found")
	errNoChange       = errors.New("device group mutation made no change")
)

type Config struct {
	Store *store.Store
	Now   func() time.Time
}

type State struct {
	store *store.Store
	now   func() time.Time
}

func NewState(cfg Config) *State {
	if cfg.Store == nil {
		panic("device group: store is required")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &State{store: cfg.Store, now: cfg.Now}
}

type CreateParams struct {
	Name        string
	Description string
	CreatedBy   string
	Query       *string
}

func (s *State) Create(ctx context.Context, op store.AuditOperation, p CreateParams) (store.DeviceGroupView, error) {
	if ctx == nil || !validID(p.CreatedBy) || (op.ActorID != "" && op.ActorID != p.CreatedBy) ||
		p.Name == "" || utf8.RuneCountInString(p.Name) > 255 || utf8.RuneCountInString(p.Description) > 1024 {
		return store.DeviceGroupView{}, ErrInvalidInput
	}
	query, err := validatedQuery(p.Query)
	if err != nil {
		return store.DeviceGroupView{}, err
	}
	id, now := ulid.Make().String(), s.now().UTC()
	_, err = s.store.WithAudit(ctx, op, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		if _, err := tx.InsertDeviceGroup(ctx, db.InsertDeviceGroupParams{
			ID: id, Name: p.Name, Description: p.Description, CreatedAt: &now,
			CreatedBy: p.CreatedBy, DynamicQuery: query,
		}); err != nil {
			return fmt.Errorf("device group: insert: %w", err)
		}
		rec.Effect(groupEffect(id, "CREATE", "name", "description", "dynamic_query"))
		return nil
	})
	if err != nil {
		return store.DeviceGroupView{}, err
	}
	return s.store.GetDeviceGroup(ctx, id)
}

func (s *State) Rename(ctx context.Context, op store.AuditOperation, id, name string) (store.DeviceGroupView, error) {
	if ctx == nil || !validID(id) || name == "" || utf8.RuneCountInString(name) > 255 {
		return store.DeviceGroupView{}, ErrInvalidInput
	}
	_, err := s.store.WithAudit(ctx, op, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		if _, err := tx.RenameDeviceGroup(ctx, db.RenameDeviceGroupParams{ID: id, NewName: name}); err != nil {
			return err
		}
		rec.Effect(groupEffect(id, "UPDATE", "name"))
		return nil
	})
	return s.readAfter(ctx, id, err)
}

func (s *State) UpdateDescription(ctx context.Context, op store.AuditOperation, id, description string) (store.DeviceGroupView, error) {
	if ctx == nil || !validID(id) || utf8.RuneCountInString(description) > 1024 {
		return store.DeviceGroupView{}, ErrInvalidInput
	}
	_, err := s.store.WithAudit(ctx, op, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		if _, err := tx.UpdateDeviceGroupDescription(ctx, db.UpdateDeviceGroupDescriptionParams{
			ID: id, Description: description,
		}); err != nil {
			return err
		}
		rec.Effect(groupEffect(id, "UPDATE", "description"))
		return nil
	})
	return s.readAfter(ctx, id, err)
}

func (s *State) UpdateQuery(ctx context.Context, op store.AuditOperation, id string, raw *string) (store.DeviceGroupView, error) {
	if ctx == nil || !validID(id) {
		return store.DeviceGroupView{}, ErrInvalidInput
	}
	query, err := validatedQuery(raw)
	if err != nil {
		return store.DeviceGroupView{}, err
	}
	if _, err := s.store.GetDeviceGroup(ctx, id); err != nil {
		return store.DeviceGroupView{}, translateNotFound(err)
	}
	_, err = s.store.WithAudit(ctx, op, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		current, err := tx.GetDeviceGroup(ctx, id)
		if err != nil {
			return err
		}
		fields := []string{"dynamic_query"}

		if query != nil && current.DynamicQuery == nil {
			if _, err := tx.DeleteDeviceGroupMembers(ctx, id); err != nil {
				return err
			}
			fields = append(fields, "members")
		}
		if _, err := tx.UpdateDeviceGroupQuery(ctx, db.UpdateDeviceGroupQueryParams{
			ID: id, DynamicQuery: query,
		}); err != nil {
			return err
		}
		rec.Effect(groupEffect(id, "UPDATE", fields...))
		return nil
	})
	return s.readAfter(ctx, id, err)
}

func (s *State) AddDevices(ctx context.Context, op store.AuditOperation, groupID string, deviceIDs []string) (int64, error) {
	if ctx == nil || !validID(groupID) || len(deviceIDs) == 0 || len(deviceIDs) > maxBatchDevices {
		return 0, ErrInvalidInput
	}
	group, err := s.store.GetDeviceGroup(ctx, groupID)
	if err != nil {
		return 0, translateNotFound(err)
	}
	if group.DynamicQuery != nil {
		return 0, ErrDynamicGroup
	}
	unique := make([]string, 0, len(deviceIDs))
	seen := make(map[string]struct{}, len(deviceIDs))
	for _, id := range deviceIDs {
		if !validID(id) {
			return 0, ErrInvalidInput
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if _, err := s.store.GetDevice(ctx, id); err != nil {
			return 0, translateNotFound(err)
		}
		unique = append(unique, id)
	}
	members, err := s.store.ListDeviceGroupMembers(ctx, groupID)
	if err != nil {
		return 0, err
	}
	for _, member := range members {
		delete(seen, member.DeviceID)
	}
	if len(seen) == 0 {
		return 0, nil
	}

	now := s.now().UTC()
	var added int64
	_, err = s.store.WithAudit(ctx, op, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		current, err := tx.GetDeviceGroup(ctx, groupID)
		if err != nil {
			return err
		}
		if current.DynamicQuery != nil {
			return ErrDynamicGroup
		}
		for _, id := range unique {
			if _, wanted := seen[id]; !wanted {
				continue
			}
			rows, err := tx.AddDeviceGroupMember(ctx, db.AddDeviceGroupMemberParams{
				GroupID: groupID, DeviceID: id, AddedAt: &now,
			})
			if err != nil {
				return err
			}
			if rows == 0 {
				continue
			}
			added += rows
			after := id
			effect := groupEffect(groupID, "UPDATE", "members")
			effect.AfterRef = &after
			rec.Effect(effect)
		}
		if added == 0 {
			return errNoChange
		}
		return nil
	})
	if errors.Is(err, errNoChange) {
		return 0, nil
	}
	return added, translateNotFound(err)
}

func (s *State) RemoveDevice(ctx context.Context, op store.AuditOperation, groupID, deviceID string) error {
	if ctx == nil || !validID(groupID) || !validID(deviceID) {
		return ErrInvalidInput
	}
	group, err := s.store.GetDeviceGroup(ctx, groupID)
	if err != nil {
		return translateNotFound(err)
	}
	if group.DynamicQuery != nil {
		return ErrDynamicGroup
	}
	_, err = s.store.WithAudit(ctx, op, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		rows, err := tx.RemoveDeviceGroupMember(ctx, db.RemoveDeviceGroupMemberParams{
			GroupID: groupID, DeviceID: deviceID,
		})
		if err != nil {
			return err
		}
		if rows == 0 {
			return ErrMemberNotFound
		}
		before := deviceID
		effect := groupEffect(groupID, "UPDATE", "members")
		effect.BeforeRef = &before
		rec.Effect(effect)
		return nil
	})
	return translateNotFound(err)
}

func (s *State) SetSyncInterval(ctx context.Context, op store.AuditOperation, id string, minutes int32) (store.DeviceGroupView, error) {
	if ctx == nil || !validID(id) || minutes < 0 || minutes > 1440 {
		return store.DeviceGroupView{}, ErrInvalidInput
	}
	_, err := s.store.WithAudit(ctx, op, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		if _, err := tx.SetDeviceGroupSyncInterval(ctx, db.SetDeviceGroupSyncIntervalParams{
			ID: id, SyncIntervalMinutes: minutes,
		}); err != nil {
			return err
		}
		rec.Effect(groupEffect(id, "UPDATE", "sync_interval_minutes"))
		return nil
	})
	return s.readAfter(ctx, id, err)
}

func (s *State) SetInventoryInterval(ctx context.Context, op store.AuditOperation, id string, minutes int32) (store.DeviceGroupView, error) {
	if ctx == nil || !validID(id) || (minutes != 0 && (minutes < 120 || minutes > 10080)) {
		return store.DeviceGroupView{}, ErrInvalidInput
	}
	_, err := s.store.WithAudit(ctx, op, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		if _, err := tx.SetDeviceGroupInventoryInterval(ctx, db.SetDeviceGroupInventoryIntervalParams{
			ID: id, InventoryIntervalMinutes: minutes,
		}); err != nil {
			return err
		}
		rec.Effect(groupEffect(id, "UPDATE", "inventory_interval_minutes"))
		return nil
	})
	return s.readAfter(ctx, id, err)
}

func (s *State) SetMaintenanceWindow(ctx context.Context, op store.AuditOperation, id string, raw []byte) (store.DeviceGroupView, error) {
	var object *struct{}
	if ctx == nil || !validID(id) || len(raw) == 0 || json.Unmarshal(raw, &object) != nil || object == nil {
		return store.DeviceGroupView{}, ErrInvalidInput
	}
	_, err := s.store.WithAudit(ctx, op, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		if _, err := tx.SetDeviceGroupMaintenanceWindow(ctx, db.SetDeviceGroupMaintenanceWindowParams{
			ID: id, MaintenanceWindow: raw,
		}); err != nil {
			return err
		}
		rec.Effect(groupEffect(id, "UPDATE", "maintenance_window"))
		return nil
	})
	return s.readAfter(ctx, id, err)
}

func (s *State) Delete(ctx context.Context, op store.AuditOperation, id string) error {
	if ctx == nil || !validID(id) {
		return ErrInvalidInput
	}
	_, err := s.store.WithAudit(ctx, op, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		members, err := tx.DeleteDeviceGroupMembers(ctx, id)
		if err != nil {
			return err
		}
		assignments, err := tx.DeleteDeviceGroupAssignments(ctx, id)
		if err != nil {
			return err
		}
		scopeID := id
		userGrants, err := tx.DeleteDeviceGroupUserRoleScopes(ctx, &scopeID)
		if err != nil {
			return err
		}
		groupGrants, err := tx.DeleteDeviceGroupUserGroupRoleScopes(ctx, &scopeID)
		if err != nil {
			return err
		}
		if _, err := tx.SoftDeleteDeviceGroup(ctx, id); err != nil {
			return err
		}
		count := members + assignments + userGrants + groupGrants
		effect := groupEffect(id, "DELETE", "is_deleted", "members", "assignments", "scoped_grants")
		effect.BeforeCount = &count
		rec.Effect(effect)
		return nil
	})
	return translateNotFound(err)
}

func (s *State) readAfter(ctx context.Context, id string, mutationErr error) (store.DeviceGroupView, error) {
	if mutationErr != nil {
		return store.DeviceGroupView{}, translateNotFound(mutationErr)
	}
	return s.store.GetDeviceGroup(ctx, id)
}

func validatedQuery(raw *string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	if _, err := dynamicquery.CompileDevice(*raw); err != nil {
		return nil, ErrInvalidQuery
	}
	return raw, nil
}

func validID(id string) bool {
	_, err := ulid.ParseStrict(id)
	return err == nil
}

func translateNotFound(err error) error {
	if store.IsNotFound(err) {
		return store.ErrNotFound
	}
	return err
}

func groupEffect(id, action string, fields ...string) store.AuditEffect {
	return store.AuditEffect{
		ResourceType: "device_group", ResourceID: id, Action: action,
		Outcome: store.EffectApplied, ChangedFields: fields,
	}
}
