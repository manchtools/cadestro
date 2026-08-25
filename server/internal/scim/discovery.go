package scim

import (
	"net/http"
)

type serviceProviderConfig struct {
	Schemas               []string               `json:"schemas"`
	DocumentationURI      string                 `json:"documentationUri"`
	Patch                 supportedCapability    `json:"patch"`
	Bulk                  bulkCapability         `json:"bulk"`
	Filter                filterCapability       `json:"filter"`
	ChangePassword        supportedCapability    `json:"changePassword"`
	Sort                  supportedCapability    `json:"sort"`
	ETag                  supportedCapability    `json:"etag"`
	AuthenticationSchemes []authenticationScheme `json:"authenticationSchemes"`
	Meta                  SCIMMeta               `json:"meta"`
}

type supportedCapability struct {
	Supported bool `json:"supported"`
}

type bulkCapability struct {
	Supported      bool `json:"supported"`
	MaxOperations  int  `json:"maxOperations"`
	MaxPayloadSize int  `json:"maxPayloadSize"`
}

type filterCapability struct {
	Supported  bool `json:"supported"`
	MaxResults int  `json:"maxResults"`
}

type authenticationScheme struct {
	Type             string `json:"type"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	SpecURI          string `json:"specUri"`
	DocumentationURI string `json:"documentationUri"`
	Primary          bool   `json:"primary"`
}

type schemaResource struct {
	Schemas     []string        `json:"schemas"`
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Attributes  []scimAttribute `json:"attributes"`
	Meta        SCIMMeta        `json:"meta"`
}

type scimAttribute struct {
	Name          string          `json:"name"`
	Type          string          `json:"type"`
	MultiValued   bool            `json:"multiValued"`
	Description   string          `json:"description"`
	Required      bool            `json:"required"`
	CaseExact     *bool           `json:"caseExact,omitempty"`
	Mutability    string          `json:"mutability"`
	Returned      string          `json:"returned"`
	Uniqueness    string          `json:"uniqueness,omitempty"`
	SubAttributes []scimAttribute `json:"subAttributes,omitempty"`
}

type resourceType struct {
	Schemas     []string `json:"schemas"`
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Endpoint    string   `json:"endpoint"`
	Schema      string   `json:"schema"`
	Meta        SCIMMeta `json:"meta"`
}

func (h *Handler) serviceProviderConfig(w http.ResponseWriter, r *http.Request, s *session) {
	config := serviceProviderConfig{
		Schemas:          []string{SPConfigSchema},
		DocumentationURI: "https://tools.ietf.org/html/rfc7644",
		Patch:            supportedCapability{Supported: true},
		Bulk: bulkCapability{
			Supported:      false,
			MaxOperations:  0,
			MaxPayloadSize: 0,
		},
		Filter:         filterCapability{Supported: true, MaxResults: 200},
		ChangePassword: supportedCapability{Supported: false},
		Sort:           supportedCapability{Supported: false},
		ETag:           supportedCapability{Supported: false},
		AuthenticationSchemes: []authenticationScheme{{
			Type:             "oauthbearertoken",
			Name:             "OAuth Bearer Token",
			Description:      "Authentication scheme using the OAuth Bearer Token Standard",
			SpecURI:          "https://tools.ietf.org/html/rfc6750",
			DocumentationURI: "https://tools.ietf.org/html/rfc6750",
			Primary:          true,
		}},
		Meta: SCIMMeta{
			ResourceType: "ServiceProviderConfig",
			Location:     baseURLFromRequest(r, s.provider.Slug) + "/ServiceProviderConfig",
		},
	}

	writeJSON(w, http.StatusOK, config)
}

func (h *Handler) schemas(w http.ResponseWriter, r *http.Request, s *session) {
	baseURL := baseURLFromRequest(r, s.provider.Slug)

	userSchema := schemaResource{
		Schemas:     []string{SchemaSchema},
		ID:          UserSchema,
		Name:        "User",
		Description: "User Account",
		Attributes: []scimAttribute{
			{
				Name:        "userName",
				Type:        "string",
				MultiValued: false,
				Description: "Unique identifier for the User, typically used by the user to directly authenticate.",
				Required:    true,
				CaseExact:   boolPtr(false),
				Mutability:  "readWrite",
				Returned:    "default",
				Uniqueness:  "server",
			},
			{
				Name:        "name",
				Type:        "complex",
				MultiValued: false,
				Description: "The components of the user's name.",
				Required:    false,
				SubAttributes: []scimAttribute{
					{Name: "formatted", Type: "string", MultiValued: false, Description: "The full name.", Required: false, Mutability: "readWrite", Returned: "default"},
					{Name: "familyName", Type: "string", MultiValued: false, Description: "The family name of the user.", Required: false, Mutability: "readWrite", Returned: "default"},
					{Name: "givenName", Type: "string", MultiValued: false, Description: "The given name of the user.", Required: false, Mutability: "readWrite", Returned: "default"},
				},
				Mutability: "readWrite",
				Returned:   "default",
			},
			{
				Name:        "emails",
				Type:        "complex",
				MultiValued: true,
				Description: "Email addresses for the user.",
				Required:    false,
				SubAttributes: []scimAttribute{
					{Name: "value", Type: "string", MultiValued: false, Description: "Email address.", Required: false, Mutability: "readWrite", Returned: "default"},
					{Name: "type", Type: "string", MultiValued: false, Description: "A label indicating the email type.", Required: false, Mutability: "readWrite", Returned: "default"},
					{Name: "primary", Type: "boolean", MultiValued: false, Description: "Indicates if this is the primary email.", Required: false, Mutability: "readWrite", Returned: "default"},
				},
				Mutability: "readWrite",
				Returned:   "default",
			},
			{
				Name:        "active",
				Type:        "boolean",
				MultiValued: false,
				Description: "A Boolean value indicating the user's administrative status.",
				Required:    false,
				Mutability:  "readWrite",
				Returned:    "default",
			},
			{
				Name:        "externalId",
				Type:        "string",
				MultiValued: false,
				Description: "An identifier for the resource as defined by the provisioning client.",
				Required:    false,
				CaseExact:   boolPtr(true),
				Mutability:  "readWrite",
				Returned:    "default",
			},
		},
		Meta: SCIMMeta{ResourceType: "Schema", Location: baseURL + "/Schemas/" + UserSchema},
	}

	groupSchema := schemaResource{
		Schemas:     []string{SchemaSchema},
		ID:          GroupSchema,
		Name:        "Group",
		Description: "Group",
		Attributes: []scimAttribute{
			{
				Name:        "displayName",
				Type:        "string",
				MultiValued: false,
				Description: "A human-readable name for the Group.",
				Required:    true,
				CaseExact:   boolPtr(false),
				Mutability:  "readWrite",
				Returned:    "default",
				Uniqueness:  "none",
			},
			{
				Name:        "members",
				Type:        "complex",
				MultiValued: true,
				Description: "A list of members of the Group.",
				Required:    false,
				SubAttributes: []scimAttribute{
					{Name: "value", Type: "string", MultiValued: false, Description: "Identifier of the member.", Required: false, Mutability: "immutable", Returned: "default"},
					{Name: "$ref", Type: "reference", MultiValued: false, Description: "The URI of the member resource.", Required: false, Mutability: "immutable", Returned: "default"},
					{Name: "display", Type: "string", MultiValued: false, Description: "A human-readable name for the member.", Required: false, Mutability: "readOnly", Returned: "default"},
				},
				Mutability: "readWrite",
				Returned:   "default",
			},
			{
				Name:        "externalId",
				Type:        "string",
				MultiValued: false,
				Description: "An identifier for the resource as defined by the provisioning client.",
				Required:    false,
				CaseExact:   boolPtr(true),
				Mutability:  "readWrite",
				Returned:    "default",
			},
		},
		Meta: SCIMMeta{ResourceType: "Schema", Location: baseURL + "/Schemas/" + GroupSchema},
	}

	writeJSON(w, http.StatusOK, SCIMListResponse{
		Schemas:      []string{ListResponseSchema},
		TotalResults: 2,
		StartIndex:   1,
		ItemsPerPage: 2,
		Resources:    []any{userSchema, groupSchema},
	})
}

func (h *Handler) resourceTypes(w http.ResponseWriter, r *http.Request, s *session) {
	baseURL := baseURLFromRequest(r, s.provider.Slug)

	userResourceType := resourceType{
		Schemas:     []string{ResourceTypeSchema},
		ID:          "User",
		Name:        "User",
		Description: "User Account",
		Endpoint:    "/Users",
		Schema:      UserSchema,
		Meta:        SCIMMeta{ResourceType: "ResourceType", Location: baseURL + "/ResourceTypes/User"},
	}
	groupResourceType := resourceType{
		Schemas:     []string{ResourceTypeSchema},
		ID:          "Group",
		Name:        "Group",
		Description: "Group",
		Endpoint:    "/Groups",
		Schema:      GroupSchema,
		Meta:        SCIMMeta{ResourceType: "ResourceType", Location: baseURL + "/ResourceTypes/Group"},
	}

	writeJSON(w, http.StatusOK, SCIMListResponse{
		Schemas:      []string{ListResponseSchema},
		TotalResults: 2,
		StartIndex:   1,
		ItemsPerPage: 2,
		Resources:    []any{userResourceType, groupResourceType},
	})
}
