package store

import (
	"context"
	"fmt"

	"github.com/manchtools/cadestro/server/internal/store/generated"
)

type searchTouch struct {
	resourceType string
	resourceID   string
}

func refreshSearchDocumentsForEffects(ctx context.Context, q *generated.Queries, effects []AuditEffect, touches []searchTouch) error {
	seen := make(map[string]struct{}, len(effects)+len(touches))
	refresh := func(resourceType, resourceID string) error {
		if resourceID == "" {
			return fmt.Errorf("refresh search document: empty %s resource id", resourceType)
		}
		scope := searchScopeForResource(resourceType)
		if scope == "" {
			return fmt.Errorf("refresh search document: unknown resource type %q", resourceType)
		}
		key := scope + "\x00" + resourceID
		if _, ok := seen[key]; ok {
			return nil
		}
		seen[key] = struct{}{}
		return refreshSearchDocument(ctx, q, scope, resourceID)
	}
	for _, effect := range effects {
		scope := searchScopeForResource(effect.ResourceType)
		if scope == "" {
			continue
		}
		if err := refresh(effect.ResourceType, effect.ResourceID); err != nil {
			return err
		}
	}
	for _, touch := range touches {
		if err := refresh(touch.resourceType, touch.resourceID); err != nil {
			return err
		}
	}
	return nil
}

func searchScopeForResource(resourceType string) string {
	switch resourceType {
	case "action":
		return "actions"
	case "action_set":
		return "action_sets"
	case "definition":
		return "definitions"
	case "compliance_policy":
		return "compliance_policies"
	case "device", "device_inventory":
		return "devices"
	case "device_group":
		return "device_groups"
	case "user":
		return "users"
	case "user_group":
		return "user_groups"
	default:
		return ""
	}
}

func refreshSearchDocument(ctx context.Context, q *generated.Queries, scope, id string) error {
	if err := q.DeleteSearchDocument(ctx, generated.DeleteSearchDocumentParams{Scope: scope, EntityID: id}); err != nil {
		return fmt.Errorf("refresh %s search document: delete: %w", scope, err)
	}
	if err := insertSearchDocument(ctx, q, scope, id); err != nil {
		return fmt.Errorf("refresh %s search document: insert: %w", scope, err)
	}
	return nil
}

func insertSearchDocument(ctx context.Context, q *generated.Queries, scope, id string) error {
	switch scope {
	case "actions":
		return q.RefreshActionsSearchDocument(ctx, id)
	case "action_sets":
		return q.RefreshActionSetsSearchDocument(ctx, id)
	case "definitions":
		return q.RefreshDefinitionsSearchDocument(ctx, id)
	case "compliance_policies":
		return q.RefreshCompliancePoliciesSearchDocument(ctx, id)
	case "devices":
		return q.RefreshDevicesSearchDocument(ctx, id)
	case "device_groups":
		return q.RefreshDeviceGroupsSearchDocument(ctx, id)
	case "users":
		return q.RefreshUsersSearchDocument(ctx, id)
	case "user_groups":
		return q.RefreshUserGroupsSearchDocument(ctx, id)
	case "audit_events":
		return q.RefreshAuditEventsSearchDocument(ctx, id)
	default:
		return fmt.Errorf("refresh search document: unknown scope %q", scope)
	}
}

var searchDocumentScopes = []string{
	"actions", "action_sets", "definitions", "compliance_policies", "devices",
	"device_groups", "users", "user_groups", "audit_events",
}

func rebuildSearchDocuments(ctx context.Context, q *generated.Queries) error {
	if err := q.DeleteAllSearchDocuments(ctx); err != nil {
		return fmt.Errorf("rebuild search documents: clear: %w", err)
	}
	for _, scope := range searchDocumentScopes {
		if err := rebuildSearchDocumentsForScope(ctx, q, scope); err != nil {
			return fmt.Errorf("rebuild %s search documents: %w", scope, err)
		}
	}
	return nil
}

func rebuildSearchDocumentsForScope(ctx context.Context, q *generated.Queries, scope string) error {
	switch scope {
	case "actions":
		return q.RebuildActionsSearchDocuments(ctx)
	case "action_sets":
		return q.RebuildActionSetsSearchDocuments(ctx)
	case "definitions":
		return q.RebuildDefinitionsSearchDocuments(ctx)
	case "compliance_policies":
		return q.RebuildCompliancePoliciesSearchDocuments(ctx)
	case "devices":
		return q.RebuildDevicesSearchDocuments(ctx)
	case "device_groups":
		return q.RebuildDeviceGroupsSearchDocuments(ctx)
	case "users":
		return q.RebuildUsersSearchDocuments(ctx)
	case "user_groups":
		return q.RebuildUserGroupsSearchDocuments(ctx)
	case "audit_events":
		return q.RebuildAuditEventsSearchDocuments(ctx)
	default:
		return fmt.Errorf("rebuild search documents: unknown scope %q", scope)
	}
}
