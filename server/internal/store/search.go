package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/manchtools/cadestro/server/internal/store/generated"
	"github.com/manchtools/cadestro/server/internal/store/sqlitetype"
)

var ErrInvalidSearch = errors.New("invalid search request")

type SearchDateRange struct {
	Field      string
	Start, End int64
}

type SearchParams struct {
	Scope      string
	Query      string
	Offset     int32
	Limit      int32
	DateRanges []SearchDateRange
	TagFilters map[string][]string
	SortField  string
	Descending bool

	ScopeRestricted bool
	ScopeGroupIDs   []string
	AssignedUserID  *string
	OnlineSince     time.Time
}

type SearchRow struct {
	ID, Name, Description string
	MemberCount           int64
	Fields                map[string]string
}

type searchDocument struct {
	row     SearchRow
	related string
}

const fuzzyCandidateLimit = 20_000

type sqliteSearchFacet struct {
	defaultSort string
	sorts       map[string]bool
	tags        map[string]bool
	dates       map[string]bool
}

var sqliteSearchFacets = map[string]sqliteSearchFacet{
	"actions":             {"created_at", keys("name", "type", "created_at", "updated_at"), keys("type", "is_compliance", "assigned"), keys("created_at", "updated_at")},
	"action_sets":         {"created_at", keys("name", "member_count", "created_at", "updated_at"), keys("member_count", "assigned"), keys("created_at", "updated_at")},
	"definitions":         {"created_at", keys("name", "member_count", "created_at", "updated_at"), keys("member_count", "assigned"), keys("created_at", "updated_at")},
	"compliance_policies": {"created_at", keys("name", "rule_count", "created_at"), keys("rule_count", "assigned"), keys("created_at")},
	"devices":             {"last_seen_at", keys("hostname", "compliance_status", "registered_at", "last_seen_at"), keys("agent_version", "os_name", "os_arch", "compliance_status", "status"), keys("registered_at", "last_seen_at")},
	"device_groups":       {"created_at", keys("name", "member_count", "created_at"), keys("is_dynamic", "member_count"), keys("created_at")},
	"users":               {"created_at", keys("email", "display_name", "linux_username", "disabled", "created_at", "last_login_at"), keys("disabled", "role"), keys("created_at", "last_login_at")},
	"user_groups":         {"created_at", keys("name", "member_count", "created_at"), keys("is_dynamic", "member_count"), keys("created_at")},
	"audit_events":        {"occurred_at", keys("event_type", "stream_type", "actor_type", "occurred_at"), keys("stream_type", "actor_type", "actor_id"), keys("occurred_at")},
}

func keys(values ...string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func (s *Store) Search(ctx context.Context, p SearchParams) ([]SearchRow, int64, error) {
	if ctx == nil || s == nil || utf8.RuneCountInString(p.Query) > 1024 ||
		p.Offset < 0 || p.Offset > 100_000 || p.Limit < 1 || p.Limit > 200 {
		return nil, 0, ErrInvalidSearch
	}
	facet, ok := sqliteSearchFacets[p.Scope]
	if !ok || !validSearchFilters(p, facet) {
		return nil, 0, ErrInvalidSearch
	}
	if p.OnlineSince.IsZero() {
		p.OnlineSince = time.Now().UTC().Add(-5 * time.Minute)
	}

	exact, err := s.searchDocuments(ctx, p.Scope, p.Query, true)
	if err != nil {
		return nil, 0, err
	}
	exact, err = s.filterSearchDocuments(ctx, p, exact)
	if err != nil {
		return nil, 0, err
	}
	exact = filterDocumentFields(p, exact)
	sortDocuments(exact, p, facet.defaultSort)

	exactIDs := make(map[string]struct{}, len(exact))
	for _, document := range exact {
		exactIDs[document.row.ID] = struct{}{}
	}
	fuzzy := make([]fuzzySearchResult, 0)
	if len(fuzzyEligibleQueryTokens(p.Query)) > 0 {
		candidates, err := s.searchDocuments(ctx, p.Scope, p.Query, false)
		if err != nil {
			return nil, 0, err
		}
		candidates, err = s.filterSearchDocuments(ctx, p, candidates)
		if err != nil {
			return nil, 0, err
		}
		candidates = filterDocumentFields(p, candidates)
		for _, document := range candidates {
			if _, found := exactIDs[document.row.ID]; found {
				continue
			}
			rank, fuzzyOnly, matches := matchFuzzyDocument(
				p.Query, document.row.ID+" "+document.row.Name,
				document.row.Description, document.related,
			)
			if matches && fuzzyOnly {
				fuzzy = append(fuzzy, fuzzySearchResult{row: document.row, rank: rank})
			}
		}
		sort.Slice(fuzzy, func(i, j int) bool { return fuzzyResultBetter(fuzzy[i], fuzzy[j]) })
	}

	all := make([]SearchRow, 0, len(exact)+len(fuzzy))
	for _, document := range exact {
		all = append(all, document.row)
	}
	for _, result := range fuzzy {
		all = append(all, result.row)
	}
	total := int64(len(all))
	start := min(int(p.Offset), len(all))
	end := min(start+int(p.Limit), len(all))
	return all[start:end], total, nil
}

func validSearchFilters(p SearchParams, facet sqliteSearchFacet) bool {
	sortField := p.SortField
	if sortField == "" {
		sortField = facet.defaultSort
	}
	if !facet.sorts[sortField] {
		return false
	}
	for name, values := range p.TagFilters {
		if !facet.tags[name] || len(compactFilterValues(values)) == 0 {
			return false
		}
	}
	for _, dateRange := range p.DateRanges {
		if !facet.dates[dateRange.Field] || dateRange.Start < 0 || dateRange.End < 0 ||
			(dateRange.Start > 0 && dateRange.End > 0 && dateRange.Start > dateRange.End) {
			return false
		}
	}
	return true
}

func (s *Store) searchDocuments(ctx context.Context, scope, query string, exact bool) ([]searchDocument, error) {
	trimmed := strings.TrimSpace(query)
	switch {
	case exact && trimmed != "":
		expression := ftsPrefixExpression(query)
		if expression == "" {
			return []searchDocument{}, nil
		}
		return s.searchDocumentsExact(ctx, scope, expression, escapeLikePrefix(trimmed)+"%")
	case !exact:
		if expression := ftsTrigramExpression(query); expression != "" {
			return s.searchDocumentsTrigram(ctx, scope, expression)
		}
		return s.searchDocumentsPlain(ctx, scope)
	default:
		return s.searchDocumentsPlain(ctx, scope)
	}
}

const searchDocumentsExactStatement = `SELECT entity_id, primary_text, description, related_text,
member_count, fields FROM search_documents WHERE scope = ?
AND (rowid IN (SELECT rowid FROM search_fts WHERE search_fts MATCH ?)
OR lower(entity_id) LIKE lower(?) ESCAPE '\')
ORDER BY entity_id`

const searchDocumentsTrigramStatement = `SELECT entity_id, primary_text, description, related_text,
member_count, fields FROM search_documents WHERE scope = ?
AND rowid IN (SELECT rowid FROM search_trigram WHERE search_trigram MATCH ?)
ORDER BY entity_id LIMIT ?`

func (s *Store) searchDocumentsExact(ctx context.Context, scope, expression, prefix string) ([]searchDocument, error) {
	rows, err := s.db.QueryContext(ctx, searchDocumentsExactStatement, scope, expression, prefix)
	if err != nil {
		return nil, fmt.Errorf("search %s documents: %w", scope, err)
	}
	defer rows.Close()
	return scanSearchDocuments(rows, scope)
}

func (s *Store) searchDocumentsTrigram(ctx context.Context, scope, expression string) ([]searchDocument, error) {
	rows, err := s.db.QueryContext(ctx, searchDocumentsTrigramStatement, scope, expression, fuzzyCandidateLimit)
	if err != nil {
		return nil, fmt.Errorf("search %s documents: %w", scope, err)
	}
	defer rows.Close()
	return scanSearchDocuments(rows, scope)
}

func (s *Store) searchDocumentsPlain(ctx context.Context, scope string) ([]searchDocument, error) {
	rows, err := s.queries.SearchDocumentsPlain(ctx, generated.SearchDocumentsPlainParams{
		Scope: scope, RowLimit: fuzzyCandidateLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("search %s documents: %w", scope, err)
	}
	documents := make([]searchDocument, 0, len(rows))
	for _, row := range rows {
		document, err := newSearchDocument(row.EntityID, row.PrimaryText, row.Description, row.RelatedText, row.MemberCount, row.Fields)
		if err != nil {
			return nil, fmt.Errorf("search %s fields: %w", scope, err)
		}
		documents = append(documents, document)
	}
	return documents, nil
}

func scanSearchDocuments(rows *sql.Rows, scope string) ([]searchDocument, error) {
	documents := make([]searchDocument, 0)
	for rows.Next() {
		var id, name, description, related string
		var memberCount int64
		var fields []byte
		if err := rows.Scan(&id, &name, &description, &related, &memberCount, &fields); err != nil {
			return nil, fmt.Errorf("search %s scan: %w", scope, err)
		}
		document, err := newSearchDocument(id, name, description, related, memberCount, fields)
		if err != nil {
			return nil, fmt.Errorf("search %s fields: %w", scope, err)
		}
		documents = append(documents, document)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search %s rows: %w", scope, err)
	}
	return documents, nil
}

func newSearchDocument(id, name, description, related string, memberCount int64, fields []byte) (searchDocument, error) {
	document := searchDocument{
		row:     SearchRow{ID: id, Name: name, Description: description, MemberCount: memberCount, Fields: make(map[string]string)},
		related: related,
	}
	if err := json.Unmarshal(fields, &document.row.Fields); err != nil {
		return searchDocument{}, err
	}
	return document, nil
}

func escapeLikePrefix(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}

func ftsTrigramExpression(query string) string {
	seen := make(map[string]struct{})
	parts := make([]string, 0)
	for _, token := range fuzzyEligibleQueryTokens(query) {
		runes := []rune(token)
		if len(runes) <= 4 {
			return ""
		}
		for i := 0; i+3 <= len(runes); i++ {
			trigram := string(runes[i : i+3])
			if _, duplicate := seen[trigram]; duplicate {
				continue
			}
			seen[trigram] = struct{}{}
			parts = append(parts, `"`+strings.ReplaceAll(trigram, `"`, `""`)+`"`)
		}
	}
	return strings.Join(parts, " OR ")
}

func ftsPrefixExpression(query string) string {
	tokens := tokenizeSearchText(query)
	parts := make([]string, len(tokens))
	for i, token := range tokens {
		parts[i] = `"` + strings.ReplaceAll(token, `"`, `""`) + `"*`
	}
	return strings.Join(parts, " AND ")
}

func filterDocumentFields(p SearchParams, documents []searchDocument) []searchDocument {
	filtered := documents[:0]
	for _, document := range documents {
		if !matchesTagFilters(p, document.row.Fields) || !matchesDateFilters(p, document.row.Fields) {
			continue
		}
		if p.Scope == "devices" {
			status := "offline"
			if seconds, _ := strconv.ParseInt(document.row.Fields["last_seen_at"], 10, 64); seconds > p.OnlineSince.Unix() {
				status = "online"
			}
			if values, found := p.TagFilters["status"]; found && !contains(compactFilterValues(values), status) {
				continue
			}
		}
		filtered = append(filtered, document)
	}
	return filtered
}

func matchesTagFilters(p SearchParams, fields map[string]string) bool {
	for name, raw := range p.TagFilters {
		if name == "status" && p.Scope == "devices" {
			continue
		}
		if !contains(compactFilterValues(raw), fields[name]) {
			return false
		}
	}
	return true
}

func matchesDateFilters(p SearchParams, fields map[string]string) bool {
	for _, dateRange := range p.DateRanges {
		value, err := strconv.ParseInt(fields[dateRange.Field], 10, 64)
		if err != nil || (dateRange.Start > 0 && value < dateRange.Start) || (dateRange.End > 0 && value > dateRange.End) {
			return false
		}
	}
	return true
}

func sortDocuments(documents []searchDocument, p SearchParams, defaultField string) {
	field := p.SortField
	if field == "" {
		field = defaultField
	}
	sort.SliceStable(documents, func(i, j int) bool {
		left, right := searchSortValue(documents[i].row, field), searchSortValue(documents[j].row, field)
		less := left < right || (left == right && documents[i].row.ID < documents[j].row.ID)
		if p.Descending && left != right {
			return !less
		}
		return less
	})
}

func searchSortValue(row SearchRow, field string) string {
	switch field {
	case "name":
		return strings.ToLower(row.Name)
	case "member_count", "rule_count":
		return fmt.Sprintf("%020d", row.MemberCount)
	default:
		return strings.ToLower(row.Fields[field])
	}
}

func (s *Store) filterSearchDocuments(ctx context.Context, p SearchParams, documents []searchDocument) ([]searchDocument, error) {
	if len(documents) == 0 {
		return documents, nil
	}
	ids := make([]string, len(documents))
	for i := range documents {
		ids[i] = documents[i].row.ID
	}
	visible, err := s.visibleSearchIDs(ctx, p, ids)
	if err != nil {
		return nil, err
	}
	filtered := documents[:0]
	for _, document := range documents {
		if visible[document.row.ID] {
			filtered = append(filtered, document)
		}
	}
	return filtered, nil
}

func (s *Store) visibleSearchIDs(ctx context.Context, p SearchParams, ids []string) (map[string]bool, error) {
	requested := sqlitetype.StringList(ids)
	groups := sqlitetype.StringList(p.ScopeGroupIDs)
	assigned := ""
	if p.AssignedUserID != nil {
		assigned = *p.AssignedUserID
	}
	visibleIDs, err := s.queryVisibleSearchIDs(ctx, p.Scope, requested, groups, p.ScopeRestricted, assigned)
	if err != nil {
		return nil, fmt.Errorf("search %s visibility: %w", p.Scope, err)
	}
	visible := make(map[string]bool, len(visibleIDs))
	for _, id := range visibleIDs {
		visible[id] = true
	}
	return visible, nil
}

func (s *Store) queryVisibleSearchIDs(ctx context.Context, scope string, requested, groups sqlitetype.StringList, restricted bool, assigned string) ([]string, error) {
	switch scope {
	case "actions":
		return s.queries.VisibleActionsSearchIDs(ctx, generated.VisibleActionsSearchIDsParams{
			RequestedJson: requested, AllowedGroupsJson: groups, ScopeRestricted: restricted,
		})
	case "action_sets":
		return s.queries.VisibleActionSetsSearchIDs(ctx, generated.VisibleActionSetsSearchIDsParams{
			RequestedJson: requested, AllowedGroupsJson: groups, ScopeRestricted: restricted,
		})
	case "definitions":
		return s.queries.VisibleDefinitionsSearchIDs(ctx, generated.VisibleDefinitionsSearchIDsParams{
			RequestedJson: requested, AllowedGroupsJson: groups, ScopeRestricted: restricted,
		})
	case "compliance_policies":
		return s.queries.VisibleCompliancePoliciesSearchIDs(ctx, generated.VisibleCompliancePoliciesSearchIDsParams{
			RequestedJson: requested, AllowedGroupsJson: groups, ScopeRestricted: restricted,
		})
	case "devices":
		return s.queries.VisibleDevicesSearchIDs(ctx, generated.VisibleDevicesSearchIDsParams{
			RequestedJson: requested, AllowedGroupsJson: groups, ScopeRestricted: restricted, AssignedUserID: assigned,
		})
	case "device_groups":
		return s.queries.VisibleDeviceGroupsSearchIDs(ctx, generated.VisibleDeviceGroupsSearchIDsParams{
			RequestedJson: requested, AllowedGroupsJson: groups, ScopeRestricted: restricted,
		})
	case "users":
		return s.queries.VisibleUsersSearchIDs(ctx, generated.VisibleUsersSearchIDsParams{
			RequestedJson: requested, AllowedGroupsJson: groups, ScopeRestricted: restricted,
		})
	case "user_groups":
		return s.queries.VisibleUserGroupsSearchIDs(ctx, generated.VisibleUserGroupsSearchIDsParams{
			RequestedJson: requested, AllowedGroupsJson: groups, ScopeRestricted: restricted,
		})
	case "audit_events":
		return s.queries.VisibleAuditEventsSearchIDs(ctx, generated.VisibleAuditEventsSearchIDsParams{
			RequestedJson: requested, AllowedGroupsJson: groups,
		})
	default:
		return nil, ErrInvalidSearch
	}
}

func (s *Store) RebuildSearchIndexes(ctx context.Context, op AuditOperation) error {
	_, err := s.WithAudit(ctx, op, func(ctx context.Context, tx *Tx, rec *AuditRecorder) error {
		if err := rebuildSearchDocuments(ctx, tx.Queries); err != nil {
			return err
		}
		rec.Effect(AuditEffect{ResourceType: "server_settings", ResourceID: "00000000000000000000000003", Action: "REBUILD_SEARCH", Outcome: EffectApplied})
		return nil
	})
	return err
}

func compactFilterValues(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
