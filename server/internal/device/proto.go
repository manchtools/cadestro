package device

import (
	"crypto/x509"
	"encoding/pem"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/store"
)

const onlineWindow = 5 * time.Minute

func (h *Handlers) toProto(view store.DeviceView) *cadestrov1.Device {
	device := &cadestrov1.Device{
		Id: view.ID, Hostname: view.Hostname, AgentVersion: view.AgentVersion,
		Status:                   cadestrov1.DeviceStatus_DEVICE_STATUS_OFFLINE,
		Labels:                   make(map[string]string, len(view.Labels)),
		AssignedUserIds:          append([]string(nil), view.AssignedUserIDs...),
		AssignedGroupIds:         append([]string(nil), view.AssignedGroupIDs...),
		SyncIntervalMinutes:      view.SyncIntervalMinutes,
		InventoryIntervalMinutes: view.InventoryIntervalMinutes,
		ComplianceStatus:         cadestrov1.ComplianceStatus(view.ComplianceStatus),
		ComplianceTotal:          view.ComplianceTotal, CompliancePassing: view.CompliancePassing,
	}
	for key, value := range view.Labels {
		device.Labels[key] = value
	}
	if view.RegisteredAt != nil {
		device.RegisteredAt = timestamppb.New(*view.RegisteredAt)
	}
	if view.LastSeenAt != nil {
		device.LastSeenAt = timestamppb.New(*view.LastSeenAt)
		if view.LastSeenAt.After(h.now().Add(-onlineWindow)) {
			device.Status = cadestrov1.DeviceStatus_DEVICE_STATUS_ONLINE
		}
	}
	if block, _ := pem.Decode(view.CertificatePem); block != nil {
		if cert, err := x509.ParseCertificate(block.Bytes); err == nil {
			device.CertExpiresAt = timestamppb.New(cert.NotAfter)
		}
	}
	if view.ComplianceCheckedAt != nil {
		device.ComplianceCheckedAt = timestamppb.New(*view.ComplianceCheckedAt)
	}
	if view.LastInventoryAt != nil {
		device.LastInventoryAt = timestamppb.New(*view.LastInventoryAt)
	}
	device.InventoryOverdue = inventoryOverdue(
		view.LastInventoryAt, view.RegisteredAt, view.ResolvedInventoryIntervalMinutes, h.now(),
	)
	return device
}

func inventoryOverdue(lastInventoryAt, registeredAt *time.Time, intervalMinutes int32, now time.Time) bool {
	base := lastInventoryAt
	if base == nil {
		base = registeredAt
	}
	if base == nil {
		return true
	}
	if intervalMinutes <= 0 {
		intervalMinutes = store.DefaultInventoryIntervalMinutes
	}
	interval := time.Duration(intervalMinutes) * time.Minute
	grace := interval / 4
	if grace < time.Hour {
		grace = time.Hour
	}
	return now.Sub(*base) > interval+grace
}
