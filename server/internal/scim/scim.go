package scim

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

const (
	UserSchema         = "urn:ietf:params:scim:schemas:core:2.0:User"
	GroupSchema        = "urn:ietf:params:scim:schemas:core:2.0:Group"
	ListResponseSchema = "urn:ietf:params:scim:api:messages:2.0:ListResponse"
	PatchOpSchema      = "urn:ietf:params:scim:api:messages:2.0:PatchOp"
	ErrorSchema        = "urn:ietf:params:scim:api:messages:2.0:Error"
	SPConfigSchema     = "urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"
	ResourceTypeSchema = "urn:ietf:params:scim:schemas:core:2.0:ResourceType"
	SchemaSchema       = "urn:ietf:params:scim:schemas:core:2.0:Schema"
)

const scimContentType = "application/scim+json"

const maxSCIMBodySize = 1 << 20

func limitBody(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxSCIMBodySize)
}

type SCIMError struct {
	Schemas  []string `json:"schemas"`
	Detail   string   `json:"detail"`
	Status   string   `json:"status"`
	ScimType string   `json:"scimType,omitempty"`
}

type SCIMListResponse struct {
	Schemas      []string `json:"schemas"`
	TotalResults int      `json:"totalResults"`
	StartIndex   int      `json:"startIndex"`
	ItemsPerPage int      `json:"itemsPerPage"`
	Resources    []any    `json:"Resources"`
}

type SCIMUser struct {
	Schemas    []string    `json:"schemas"`
	ID         string      `json:"id"`
	ExternalID string      `json:"externalId,omitempty"`
	UserName   string      `json:"userName"`
	Name       *SCIMName   `json:"name,omitempty"`
	Emails     []SCIMEmail `json:"emails,omitempty"`
	Active     *bool       `json:"active,omitempty"`
	Meta       *SCIMMeta   `json:"meta,omitempty"`
}

func (u SCIMUser) IsActive() bool {
	if u.Active == nil {
		return true
	}
	return *u.Active
}

func boolPtr(b bool) *bool { return &b }

type SCIMName struct {
	Formatted  string `json:"formatted,omitempty"`
	FamilyName string `json:"familyName,omitempty"`
	GivenName  string `json:"givenName,omitempty"`
}

type SCIMEmail struct {
	Value   string `json:"value"`
	Type    string `json:"type,omitempty"`
	Primary bool   `json:"primary,omitempty"`
}

type SCIMMeta struct {
	ResourceType string `json:"resourceType"`
	Location     string `json:"location,omitempty"`
	Created      string `json:"created,omitempty"`
	LastModified string `json:"lastModified,omitempty"`
}

type SCIMGroup struct {
	Schemas     []string     `json:"schemas"`
	ID          string       `json:"id"`
	ExternalID  string       `json:"externalId,omitempty"`
	DisplayName string       `json:"displayName"`
	Members     []SCIMMember `json:"members,omitempty"`
	Meta        *SCIMMeta    `json:"meta,omitempty"`
}

type SCIMMember struct {
	Value   string `json:"value"`
	Display string `json:"display,omitempty"`
	Ref     string `json:"$ref,omitempty"`
}

type SCIMPatchOpType string

const (
	SCIMPatchOpAdd SCIMPatchOpType = "add"

	SCIMPatchOpRemove SCIMPatchOpType = "remove"

	SCIMPatchOpReplace SCIMPatchOpType = "replace"
)

func (o SCIMPatchOpType) IsValid() bool {
	switch SCIMPatchOpType(strings.ToLower(string(o))) {
	case SCIMPatchOpAdd, SCIMPatchOpRemove, SCIMPatchOpReplace:
		return true
	default:
		return false
	}
}

func (o SCIMPatchOpType) Normalize() SCIMPatchOpType {
	return SCIMPatchOpType(strings.ToLower(string(o)))
}

type SCIMPatchOp struct {
	Op    SCIMPatchOpType `json:"op"`
	Path  string          `json:"path,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
}

type SCIMPatchRequest struct {
	Schemas    []string      `json:"schemas"`
	Operations []SCIMPatchOp `json:"Operations"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", scimContentType)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("failed to encode SCIM JSON response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, SCIMError{
		Schemas: []string{ErrorSchema},
		Detail:  detail,
		Status:  strconv.Itoa(status),
	})
}
