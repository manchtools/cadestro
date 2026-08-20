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
	pmcrypto "github.com/manchtools/cadestro/server/internal/crypto"
	"github.com/manchtools/cadestro/server/internal/dispatch"
	manifestpkg "github.com/manchtools/cadestro/server/internal/manifest"
	"github.com/manchtools/cadestro/server/internal/store"
)

const maxSyncDeliveries = int32(1024)

var (
	ErrInvalidInput = errors.New("invalid agent sync input")
	ErrNotConnected = errors.New("agent stream is not connected")
)

// Config supplies durable work, assignment resolution, and live connections.
type Config struct {
	Store       *store.Store
	Manager     *connection.Manager
	Assignments *dispatch.Handlers
	Now         func() time.Time
	AtRest      *pmcrypto.Encryptor
}

// Service implements durable stream synchronization.
type Service struct {
	store       *store.Store
	manager     *connection.Manager
	assignments *dispatch.Handlers
	now         func() time.Time
	atRest      *pmcrypto.Encryptor
}

// New constructs the agent sync service.
func New(cfg Config) *Service {
	if cfg.Store == nil || cfg.Manager == nil || cfg.AtRest == nil {
		panic("agentsync: store, manager, and at-rest cipher are required")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Service{store: cfg.Store, manager: cfg.Manager, assignments: cfg.Assignments, now: cfg.Now, atRest: cfg.AtRest}
}

// Sync returns due one-shot work plus the device's current assignment snapshot.
func (s *Service) Sync(ctx context.Context, deviceID string) (*pmv1.SyncState, error) {
	if ctx == nil || !validID(deviceID) {
		return nil, ErrInvalidInput
	}
	device, err := s.store.GetDevice(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	agent, connected := s.manager.Get(deviceID)
	if !connected || agent == nil || agent.Terminated() {
		return nil, ErrNotConnected
	}
	now := s.now()
	rows, err := s.store.ListDueDeviceDeliveries(ctx, deviceID, now, maxSyncDeliveries)
	if err != nil {
		return nil, err
	}
	deliveries := make([]*pmv1.ManifestDelivery, 0, len(rows))
	for _, row := range rows {
		// A delivery remains pullable until its terminal result. The stable id
		// lets the agent absorb repeated Sync responses locally.
		manifest := &pmv1.Manifest{}
		if err := protojson.Unmarshal(row.Manifest, manifest); err != nil {
			return nil, fmt.Errorf("decode delivery %s manifest: %w", row.DeliveryID, err)
		}
		if manifest.ManifestId != row.ManifestID {
			return nil, fmt.Errorf("delivery %s manifest id mismatch", row.DeliveryID)
		}
		if err := manifestpkg.MaterializeSecrets(manifest, s.atRest); err != nil {
			return nil, err
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
		for _, item := range manifests {
			if err := manifestpkg.MaterializeSecrets(item, s.atRest); err != nil {
				return nil, err
			}
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
