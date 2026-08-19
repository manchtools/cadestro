// Package agentsync builds a stream synchronization state from durable explicit
// delivery rows and the authenticated device's assignment snapshot.
package agentsync

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	pmv1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/maintenance"
	"github.com/manchtools/cadestro/server/internal/connection"
	"github.com/manchtools/cadestro/server/internal/delivery"
	"github.com/manchtools/cadestro/server/internal/dispatch"
	"github.com/manchtools/cadestro/server/internal/store"
)

const maxSyncDeliveries = int32(1024)

var (
	ErrInvalidInput = errors.New("invalid agent sync input")
	ErrNotConnected = errors.New("agent stream is not connected")
)

// Config supplies authoritative delivery state, assignment resolution, and the
// live epoch registry.
type Config struct {
	Store       *store.Store
	Manager     *connection.Manager
	Deliveries  *delivery.Service
	Assignments *dispatch.Handlers
	Now         func() time.Time
}

// Service implements durable stream synchronization.
type Service struct {
	store       *store.Store
	manager     *connection.Manager
	deliveries  *delivery.Service
	assignments *dispatch.Handlers
	now         func() time.Time
}

// New constructs the agent sync service.
func New(cfg Config) *Service {
	if cfg.Store == nil || cfg.Manager == nil || cfg.Deliveries == nil {
		panic("agentsync: store, manager, and delivery state are required")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Service{store: cfg.Store, manager: cfg.Manager, deliveries: cfg.Deliveries, assignments: cfg.Assignments, now: cfg.Now}
}

// Sync returns each still-sendable explicit delivery plus the device's current
// assignment snapshot. Only explicit deliveries are marked against the live
// connection epoch; assignment policy is pulled and reconciled by the agent.
func (s *Service) Sync(ctx context.Context, deviceID string) (*pmv1.SyncState, error) {
	if ctx == nil || !validID(deviceID) {
		return nil, ErrInvalidInput
	}
	device, err := s.store.GetDevice(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	agent, connected := s.manager.Get(deviceID)
	if !connected || agent == nil || agent.Epoch <= 0 || agent.Terminated() {
		return nil, ErrNotConnected
	}
	rows, err := s.store.ListDeviceDeliveries(ctx, deviceID, maxSyncDeliveries)
	if err != nil {
		return nil, err
	}
	now := s.now()
	deliveries := make([]*pmv1.ManifestDelivery, 0, len(rows))
	for _, row := range rows {
		// The listed rows are PENDING or PUSHED with no availability filter, so
		// a future-scheduled row must be skipped here rather than pushed early.
		// This is the same availability/epoch guard the dispatcher enforces.
		if !delivery.Sendable(row, agent.Epoch, now) {
			continue
		}
		manifest := &pmv1.Manifest{}
		if err := protojson.Unmarshal(row.Manifest, manifest); err != nil {
			return nil, fmt.Errorf("decode delivery %s manifest: %w", row.DeliveryID, err)
		}
		if manifest.ManifestId != row.ManifestID {
			return nil, delivery.ErrWrongManifest
		}
		changed, err := s.deliveries.MarkPushed(ctx, row.DeliveryID, deviceID, agent.Epoch)
		if err != nil {
			return nil, err
		}
		if !changed {
			continue
		}
		deliveries = append(deliveries, &pmv1.ManifestDelivery{
			DeliveryId: row.DeliveryID, Manifest: manifest,
		})
	}
	var desiredPolicy *pmv1.DesiredPolicy
	if s.assignments != nil {
		manifests, err := s.assignments.AssignedPolicy(ctx, deviceID)
		if err != nil {
			return nil, err
		}
		desiredPolicy = &pmv1.DesiredPolicy{Revision: policyRevision(manifests), Manifests: manifests}
	}
	windows, err := s.store.ListDeviceMaintenanceWindows(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	window, err := unionMaintenanceWindows(windows)
	if err != nil {
		return nil, err
	}
	return &pmv1.SyncState{
		SyncIntervalMinutes: device.SyncIntervalMinutes,
		Deliveries:          deliveries,
		MaintenanceWindow:   window,
		DesiredPolicy:       desiredPolicy,
	}, nil
}

func policyRevision(manifests []*pmv1.Manifest) string {
	identity := make([]*pmv1.Manifest, 0, len(manifests))
	for _, manifest := range manifests {
		if manifest == nil {
			continue
		}
		clone := proto.Clone(manifest).(*pmv1.Manifest)
		for _, occurrence := range clone.Occurrences {
			if occurrence != nil && occurrence.Action != nil {
				occurrence.Action = &pmv1.Action{
					Id: occurrence.Action.Id, Type: occurrence.Action.Type,
					DesiredState: occurrence.Action.DesiredState,
				}
			}
		}
		identity = append(identity, clone)
	}
	payload, err := proto.MarshalOptions{Deterministic: true}.Marshal(&pmv1.DesiredPolicy{Manifests: identity})
	if err != nil {
		payload = nil
	}
	digest := sha256.Sum256(payload)
	digest[0] &= 0x03
	return ulid.ULID(digest[:16]).String()
}

func unionMaintenanceWindows(rows [][]byte) (*pmv1.MaintenanceWindow, error) {
	windows := make([]*pmv1.MaintenanceWindow, 0, len(rows))
	for _, row := range rows {
		window := &pmv1.MaintenanceWindow{}
		if err := protojson.Unmarshal(row, window); err != nil {
			return nil, fmt.Errorf("decode maintenance window: %w", err)
		}
		if len(window.Schedule) != 0 {
			windows = append(windows, window)
		}
	}
	if len(windows) == 0 {
		return nil, nil
	}
	return maintenance.Union(windows...), nil
}

func validID(id string) bool {
	_, err := ulid.ParseStrict(id)
	return err == nil
}
