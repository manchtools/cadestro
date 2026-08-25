package devicegroup

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/manchtools/cadestro/server/internal/dynamicquery"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
	"github.com/manchtools/cadestro/server/internal/store/sqlitetype"
)

type EvaluationResult struct {
	Group   store.DeviceGroupView
	Added   int64
	Removed int64
}

func (s *State) CountMatchingDevices(ctx context.Context, raw string) (int64, error) {
	query, err := parseDeviceQuery(raw)
	if err != nil {
		return 0, err
	}
	rows, err := s.store.ListDevicesForDynamicEvaluation(ctx)
	if err != nil {
		return 0, err
	}
	matches, err := matchingDeviceIDs(ctx, query, rows)
	return int64(len(matches)), err
}

func (s *State) EvaluateDynamicGroup(ctx context.Context, op store.AuditOperation, id string) (EvaluationResult, error) {
	if ctx == nil || !validID(id) {
		return EvaluationResult{}, ErrInvalidInput
	}
	var added, removed []string
	_, err := s.store.WithAudit(ctx, op, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		group, err := tx.GetDynamicDeviceGroupQueryForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if !group.IsDynamic {
			return ErrStaticGroup
		}
		if group.DynamicQuery == nil {
			return ErrInvalidQuery
		}
		query, err := parseDeviceQuery(*group.DynamicQuery)
		if err != nil {
			return err
		}
		devices, err := tx.ListDevicesForDynamicEvaluation(ctx)
		if err != nil {
			return fmt.Errorf("device group: list evaluation devices: %w", err)
		}
		wanted, err := matchingDeviceIDs(ctx, query, devices)
		if err != nil {
			return err
		}
		current, err := tx.ListDeviceGroupMemberIDs(ctx, id)
		if err != nil {
			return fmt.Errorf("device group: list evaluation members: %w", err)
		}
		added, removed = membershipDelta(current, wanted)
		if len(removed) > 0 {
			removed, err = tx.RemoveDynamicDeviceGroupMembers(ctx, db.RemoveDynamicDeviceGroupMembersParams{
				GroupID: id, DeviceIdsJson: sqlitetype.StringList(removed),
			})
			if err != nil {
				return fmt.Errorf("device group: remove evaluated members: %w", err)
			}
			sort.Strings(removed)
		}
		if len(added) > 0 {
			now := s.now().UTC()
			added, err = tx.AddDynamicDeviceGroupMembers(ctx, db.AddDynamicDeviceGroupMembersParams{
				GroupID: id, DeviceIdsJson: sqlitetype.StringList(added), AddedAt: &now,
			})
			if err != nil {
				return fmt.Errorf("device group: add evaluated members: %w", err)
			}
			sort.Strings(added)
		}
		for _, deviceID := range removed {
			before := deviceID
			effect := groupEffect(id, "UPDATE", "members")
			effect.BeforeRef = &before
			rec.Effect(effect)
		}
		for _, deviceID := range added {
			after := deviceID
			effect := groupEffect(id, "UPDATE", "members")
			effect.AfterRef = &after
			rec.Effect(effect)
		}
		return nil
	})
	if err != nil {
		return EvaluationResult{}, translateNotFound(err)
	}
	group, err := s.store.GetDeviceGroup(ctx, id)
	if err != nil {
		return EvaluationResult{}, err
	}
	return EvaluationResult{Group: group, Added: int64(len(added)), Removed: int64(len(removed))}, nil
}

func parseDeviceQuery(raw string) (dynamicquery.DeviceQuery, error) {
	query, err := dynamicquery.CompileDevice(raw)
	if err != nil {
		return dynamicquery.DeviceQuery{}, fmt.Errorf("%w: %w", ErrInvalidQuery, err)
	}
	return query, nil
}

func matchingDeviceIDs(ctx context.Context, query dynamicquery.DeviceQuery, rows []db.ListDevicesForDynamicEvaluationRow) ([]string, error) {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		device, err := evaluationContext(row)
		if err != nil {
			return nil, fmt.Errorf("device group: decode evaluation device %s: %w", row.ID, err)
		}
		matched, err := query.Eval(ctx, device)
		if err != nil {
			return nil, fmt.Errorf("device group: evaluate device %s: %w", row.ID, err)
		}
		if matched {
			ids = append(ids, row.ID)
		}
	}
	return ids, nil
}

func evaluationContext(row db.ListDevicesForDynamicEvaluationRow) (dynamicquery.Device, error) {
	labels := map[string]string{}
	if err := json.Unmarshal(row.LabelsJson, &labels); err != nil {
		return dynamicquery.Device{}, err
	}
	device, err := inventoryDevice(row.Hostname, row.InventoryJson)
	if err != nil {
		return dynamicquery.Device{}, err
	}
	var groupNames []string
	if err := json.Unmarshal(row.GroupNamesJson, &groupNames); err != nil {
		return dynamicquery.Device{}, err
	}
	device.Labels, device.Groups = labels, groupNames
	return device, nil
}

type inventoryTables struct {
	OSVersion  []inventoryOSVersion  `json:"os_version"`
	SystemInfo []inventorySystemInfo `json:"system_info"`
	KernelInfo []inventoryKernelInfo `json:"kernel_info"`
}

type inventoryOSVersion struct {
	Name         json.RawMessage `json:"name"`
	Version      json.RawMessage `json:"version"`
	Major        json.RawMessage `json:"major"`
	Minor        json.RawMessage `json:"minor"`
	Arch         json.RawMessage `json:"arch"`
	Platform     json.RawMessage `json:"platform"`
	PlatformLike json.RawMessage `json:"platform_like"`
}

type inventorySystemInfo struct {
	CPUType        json.RawMessage `json:"cpu_type"`
	CPUBrand       json.RawMessage `json:"cpu_brand"`
	PhysicalCores  json.RawMessage `json:"cpu_physical_cores"`
	LogicalCores   json.RawMessage `json:"cpu_logical_cores"`
	PhysicalMemory json.RawMessage `json:"physical_memory"`
}

type inventoryKernelInfo struct {
	Version json.RawMessage `json:"version"`
}

func inventoryDevice(hostname string, raw []byte) (dynamicquery.Device, error) {
	var tables inventoryTables
	if err := json.Unmarshal(raw, &tables); err != nil {
		return dynamicquery.Device{}, err
	}
	device := dynamicquery.Device{Hostname: hostname}
	if len(tables.OSVersion) > 0 {
		row := tables.OSVersion[0]
		var err error
		if device.OS, err = inventoryText(row.Name, "os"); err != nil {
			return dynamicquery.Device{}, err
		}
		if device.OSVersion, err = inventoryText(row.Version, "os_version"); err != nil {
			return dynamicquery.Device{}, err
		}
		if device.OSMajor, err = inventoryNumber(row.Major, "os_major"); err != nil {
			return dynamicquery.Device{}, err
		}
		if device.OSMinor, err = inventoryNumber(row.Minor, "os_minor"); err != nil {
			return dynamicquery.Device{}, err
		}
		if device.OSArch, err = inventoryText(row.Arch, "os_arch"); err != nil {
			return dynamicquery.Device{}, err
		}
		if device.OSPlatform, err = inventoryText(row.Platform, "os_platform"); err != nil {
			return dynamicquery.Device{}, err
		}
		if device.OSPlatformLike, err = inventoryText(row.PlatformLike, "os_platform_like"); err != nil {
			return dynamicquery.Device{}, err
		}
	}
	if len(tables.SystemInfo) > 0 {
		row := tables.SystemInfo[0]
		var err error
		if device.CPUType, err = inventoryText(row.CPUType, "cpu_type"); err != nil {
			return dynamicquery.Device{}, err
		}
		if device.CPUBrand, err = inventoryText(row.CPUBrand, "cpu_brand"); err != nil {
			return dynamicquery.Device{}, err
		}
		if device.CPUCores, err = inventoryNumber(row.PhysicalCores, "cpu_cores"); err != nil {
			return dynamicquery.Device{}, err
		}
		if device.CPULogicalCores, err = inventoryNumber(row.LogicalCores, "cpu_logical_cores"); err != nil {
			return dynamicquery.Device{}, err
		}
		if device.MemoryTotal, err = inventoryNumber(row.PhysicalMemory, "memory_total"); err != nil {
			return dynamicquery.Device{}, err
		}
	}
	if len(tables.KernelInfo) > 0 {
		var err error
		device.Kernel, err = inventoryText(tables.KernelInfo[0].Version, "kernel")
		if err != nil {
			return dynamicquery.Device{}, err
		}
	}
	return device, nil
}

func inventoryText(raw json.RawMessage, field string) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return value, nil
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		return number.String(), nil
	}
	return "", fmt.Errorf("inventory %s is not a string or number", field)
}

func inventoryNumber(raw json.RawMessage, field string) (int64, error) {
	value, err := inventoryText(raw, field)
	if err != nil || value == "" {
		return 0, err
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("inventory %s is not an integer: %w", field, err)
	}
	return parsed, nil
}

func membershipDelta(current, wanted []string) (added, removed []string) {
	currentSet := make(map[string]struct{}, len(current))
	wantedSet := make(map[string]struct{}, len(wanted))
	for _, id := range current {
		currentSet[id] = struct{}{}
	}
	for _, id := range wanted {
		wantedSet[id] = struct{}{}
		if _, ok := currentSet[id]; !ok {
			added = append(added, id)
		}
	}
	for _, id := range current {
		if _, ok := wantedSet[id]; !ok {
			removed = append(removed, id)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}
