// Package agentsync builds stream synchronization state from scheduled policy
// work and the authenticated device's assignment snapshot.
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

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/maintenance"
	"github.com/manchtools/cadestro/server/internal/connection"
	"github.com/manchtools/cadestro/server/internal/crypto"
	"github.com/manchtools/cadestro/server/internal/devicecontrol"
	manifestpkg "github.com/manchtools/cadestro/server/internal/manifest"
	"github.com/manchtools/cadestro/server/internal/store"
)

var (
	ErrInvalidInput = errors.New("invalid agent sync input")
	ErrNotConnected = errors.New("agent stream is not connected")
)

// Config supplies durable work, assignment resolution, and live connections.
type Config struct {
	Store       *store.Store
	Manager     *connection.Manager
	Assignments *devicecontrol.Handlers
	Now         func() time.Time
	AtRest      *crypto.Encryptor
}

// Service implements durable stream synchronization.
type Service struct {
	store       *store.Store
	manager     *connection.Manager
	assignments *devicecontrol.Handlers
	now         func() time.Time
	atRest      *crypto.Encryptor
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

// Sync returns the device's current assignment snapshot and scheduling state.
func (s *Service) Sync(ctx context.Context, deviceID string) (*cadestrov1.SyncState, error) {
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
	var desiredPolicy *cadestrov1.DesiredPolicy
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
		desiredPolicy = &cadestrov1.DesiredPolicy{Revision: policyRevision(manifests), Manifests: manifests}
	}
	windows, err := s.store.ListDeviceMaintenanceWindows(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	window, err := unionMaintenanceWindows(windows)
	if err != nil {
		return nil, err
	}
	return &cadestrov1.SyncState{
		SyncIntervalMinutes: device.SyncIntervalMinutes,
		MaintenanceWindow:   window,
		DesiredPolicy:       desiredPolicy,
	}, nil
}

func policyRevision(manifests []*cadestrov1.Manifest) string {
	identity := make([]*cadestrov1.Manifest, 0, len(manifests))
	for _, manifest := range manifests {
		if manifest == nil {
			continue
		}
		clone := proto.Clone(manifest).(*cadestrov1.Manifest)
		for _, occurrence := range clone.Occurrences {
			if occurrence != nil && occurrence.Action != nil {
				occurrence.Action = &cadestrov1.Action{
					Id: occurrence.Action.Id, Type: occurrence.Action.Type,
					DesiredState: occurrence.Action.DesiredState,
				}
			}
		}
		identity = append(identity, clone)
	}
	payload, err := proto.MarshalOptions{Deterministic: true}.Marshal(&cadestrov1.DesiredPolicy{Manifests: identity})
	if err != nil {
		payload = nil
	}
	digest := sha256.Sum256(payload)
	digest[0] &= 0x03
	return ulid.ULID(digest[:16]).String()
}

func unionMaintenanceWindows(rows [][]byte) (*cadestrov1.MaintenanceWindow, error) {
	windows := make([]*cadestrov1.MaintenanceWindow, 0, len(rows))
	for _, row := range rows {
		window := &cadestrov1.MaintenanceWindow{}
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
