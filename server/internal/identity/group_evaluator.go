package identity

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/manchtools/cadestro/server/internal/dynamicquery"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
	"github.com/manchtools/cadestro/server/internal/store/sqlitetype"
)

var (
	errUserGroupNotDynamic   = errors.New("user group is not dynamic")
	errUserGroupInvalidQuery = errors.New("invalid user group query")
)

type userGroupEvaluationResult struct {
	added   int64
	removed int64
}

func (h *Handlers) countMatchingUsers(ctx context.Context, raw string) (int64, error) {
	query, err := parseUserGroupQuery(raw)
	if err != nil {
		return 0, err
	}
	rows, err := h.store.ListUsersForDynamicUserGroupEvaluation(ctx)
	if err != nil {
		return 0, err
	}
	matched, err := matchingUserIDs(ctx, query, rows)
	if err != nil {
		return 0, err
	}
	return int64(len(matched)), nil
}

// evaluateDynamicUserGroup reconciles the materialized membership, session
// invalidation and audit evidence in one transaction.
func (h *Handlers) evaluateDynamicUserGroup(ctx context.Context, op store.AuditOperation, groupID, actorID string) (userGroupEvaluationResult, error) {
	var result userGroupEvaluationResult
	_, err := h.store.WithAudit(ctx, op, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		group, err := tx.GetDynamicUserGroupQueryForUpdate(ctx, groupID)
		if err != nil {
			return err
		}
		if !group.IsDynamic {
			return errUserGroupNotDynamic
		}
		if group.DynamicQuery == nil {
			return errUserGroupInvalidQuery
		}
		query, err := parseUserGroupQuery(*group.DynamicQuery)
		if err != nil {
			return err
		}
		users, err := tx.ListUsersForDynamicUserGroupEvaluation(ctx)
		if err != nil {
			return err
		}
		wanted, err := matchingUserIDs(ctx, query, users)
		if err != nil {
			return err
		}
		current, err := tx.ListUserGroupMemberIDs(ctx, groupID)
		if err != nil {
			return err
		}
		added, removed := userMembershipDelta(current, wanted)

		if len(removed) > 0 {
			removed, err = tx.RemoveDynamicUserGroupMembers(ctx, db.RemoveDynamicUserGroupMembersParams{
				GroupID: groupID, UserIdsJson: sqlitetype.StringList(removed),
			})
			if err != nil {
				return err
			}
		}
		if len(added) > 0 {
			at := h.now().UTC()
			added, err = tx.AddDynamicUserGroupMembers(ctx, db.AddDynamicUserGroupMembersParams{
				GroupID: groupID, UserIdsJson: sqlitetype.StringList(added), AddedAt: at, AddedBy: actorID,
			})
			if err != nil {
				return err
			}
		}
		sort.Strings(added)
		sort.Strings(removed)
		changed := append(append(make([]string, 0, len(added)+len(removed)), added...), removed...)
		if len(changed) > 0 {
			at := h.now().UTC()
			affected, err := tx.BumpUserSessionsByIDs(ctx, db.BumpUserSessionsByIDsParams{
				UpdatedAt: &at, UserIds: changed,
			})
			if err != nil {
				return err
			}
			effect := userGroupEffect(groupID, "INVALIDATE_MEMBER_SESSIONS", "session_version")
			effect.AfterCount = &affected
			rec.Effect(effect)
			for _, userID := range changed {
				rec.RefreshSearch("user", userID)
			}
		}
		before, after := int64(len(current)), int64(len(current)-len(removed)+len(added))
		effect := userGroupEffect(groupID, "EVALUATE", "members")
		effect.BeforeCount, effect.AfterCount = &before, &after
		rec.Effect(effect)
		result.added, result.removed = int64(len(added)), int64(len(removed))
		return nil
	})
	return result, err
}

func parseUserGroupQuery(raw string) (dynamicquery.UserQuery, error) {
	query, err := dynamicquery.CompileUser(raw)
	if err != nil {
		return dynamicquery.UserQuery{}, fmt.Errorf("%w: %w", errUserGroupInvalidQuery, err)
	}
	return query, nil
}

func matchingUserIDs(ctx context.Context, query dynamicquery.UserQuery, rows []store.UserDynamicEvaluationRow) ([]string, error) {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		matched, err := query.Eval(ctx, dynamicquery.User{
			Email: row.Email, Disabled: row.Disabled, DisplayName: row.DisplayName,
			PreferredUsername: row.PreferredUsername, Locale: row.Locale,
		})
		if err != nil {
			return nil, fmt.Errorf("evaluate user %s: %w", row.ID, err)
		}
		if matched {
			ids = append(ids, row.ID)
		}
	}
	return ids, nil
}

func userMembershipDelta(current, wanted []string) (added, removed []string) {
	currentSet := make(map[string]struct{}, len(current))
	wantedSet := make(map[string]struct{}, len(wanted))
	for _, id := range current {
		currentSet[id] = struct{}{}
	}
	for _, id := range wanted {
		wantedSet[id] = struct{}{}
		if _, exists := currentSet[id]; !exists {
			added = append(added, id)
		}
	}
	for _, id := range current {
		if _, exists := wantedSet[id]; !exists {
			removed = append(removed, id)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}
