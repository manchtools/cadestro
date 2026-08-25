package scim_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/manchtools/cadestro/server/internal/scim"
)

func TestDiscoveryResponsesPreserveWireShape(t *testing.T) {
	f := newFixture(t)
	p := f.seedProvider(nil)
	base := f.server.URL + "/scim/v2/" + p.Slug

	cases := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name: "service provider config",
			path: "/ServiceProviderConfig",
			expected: fmt.Sprintf(`{
  "schemas": ["%s"],
  "documentationUri": "https://tools.ietf.org/html/rfc7644",
  "patch": {"supported": true},
  "bulk": {"supported": false, "maxOperations": 0, "maxPayloadSize": 0},
  "filter": {"supported": true, "maxResults": 200},
  "changePassword": {"supported": false},
  "sort": {"supported": false},
  "etag": {"supported": false},
  "authenticationSchemes": [{
    "type": "oauthbearertoken",
    "name": "OAuth Bearer Token",
    "description": "Authentication scheme using the OAuth Bearer Token Standard",
    "specUri": "https://tools.ietf.org/html/rfc6750",
    "documentationUri": "https://tools.ietf.org/html/rfc6750",
    "primary": true
  }],
  "meta": {"resourceType": "ServiceProviderConfig", "location": "%s/ServiceProviderConfig"}
}`, scim.SPConfigSchema, base),
		},
		{
			name: "schemas",
			path: "/Schemas",
			expected: fmt.Sprintf(`{
  "schemas": ["%s"],
  "totalResults": 2,
  "startIndex": 1,
  "itemsPerPage": 2,
  "Resources": [
    {
      "schemas": ["%s"],
      "id": "%s",
      "name": "User",
      "description": "User Account",
      "attributes": [
        {
          "name": "userName",
          "type": "string",
          "multiValued": false,
          "description": "Unique identifier for the User, typically used by the user to directly authenticate.",
          "required": true,
          "caseExact": false,
          "mutability": "readWrite",
          "returned": "default",
          "uniqueness": "server"
        },
        {
          "name": "name",
          "type": "complex",
          "multiValued": false,
          "description": "The components of the user's name.",
          "required": false,
          "subAttributes": [
            {"name": "formatted", "type": "string", "multiValued": false, "description": "The full name.", "required": false, "mutability": "readWrite", "returned": "default"},
            {"name": "familyName", "type": "string", "multiValued": false, "description": "The family name of the user.", "required": false, "mutability": "readWrite", "returned": "default"},
            {"name": "givenName", "type": "string", "multiValued": false, "description": "The given name of the user.", "required": false, "mutability": "readWrite", "returned": "default"}
          ],
          "mutability": "readWrite",
          "returned": "default"
        },
        {
          "name": "emails",
          "type": "complex",
          "multiValued": true,
          "description": "Email addresses for the user.",
          "required": false,
          "subAttributes": [
            {"name": "value", "type": "string", "multiValued": false, "description": "Email address.", "required": false, "mutability": "readWrite", "returned": "default"},
            {"name": "type", "type": "string", "multiValued": false, "description": "A label indicating the email type.", "required": false, "mutability": "readWrite", "returned": "default"},
            {"name": "primary", "type": "boolean", "multiValued": false, "description": "Indicates if this is the primary email.", "required": false, "mutability": "readWrite", "returned": "default"}
          ],
          "mutability": "readWrite",
          "returned": "default"
        },
        {
          "name": "active",
          "type": "boolean",
          "multiValued": false,
          "description": "A Boolean value indicating the user's administrative status.",
          "required": false,
          "mutability": "readWrite",
          "returned": "default"
        },
        {
          "name": "externalId",
          "type": "string",
          "multiValued": false,
          "description": "An identifier for the resource as defined by the provisioning client.",
          "required": false,
          "caseExact": true,
          "mutability": "readWrite",
          "returned": "default"
        }
      ],
      "meta": {"resourceType": "Schema", "location": "%s/Schemas/%s"}
    },
    {
      "schemas": ["%s"],
      "id": "%s",
      "name": "Group",
      "description": "Group",
      "attributes": [
        {
          "name": "displayName",
          "type": "string",
          "multiValued": false,
          "description": "A human-readable name for the Group.",
          "required": true,
          "caseExact": false,
          "mutability": "readWrite",
          "returned": "default",
          "uniqueness": "none"
        },
        {
          "name": "members",
          "type": "complex",
          "multiValued": true,
          "description": "A list of members of the Group.",
          "required": false,
          "subAttributes": [
            {"name": "value", "type": "string", "multiValued": false, "description": "Identifier of the member.", "required": false, "mutability": "immutable", "returned": "default"},
            {"name": "$ref", "type": "reference", "multiValued": false, "description": "The URI of the member resource.", "required": false, "mutability": "immutable", "returned": "default"},
            {"name": "display", "type": "string", "multiValued": false, "description": "A human-readable name for the member.", "required": false, "mutability": "readOnly", "returned": "default"}
          ],
          "mutability": "readWrite",
          "returned": "default"
        },
        {
          "name": "externalId",
          "type": "string",
          "multiValued": false,
          "description": "An identifier for the resource as defined by the provisioning client.",
          "required": false,
          "caseExact": true,
          "mutability": "readWrite",
          "returned": "default"
        }
      ],
      "meta": {"resourceType": "Schema", "location": "%s/Schemas/%s"}
    }
  ]
}`, scim.ListResponseSchema, scim.SchemaSchema, scim.UserSchema, base, scim.UserSchema, scim.SchemaSchema, scim.GroupSchema, base, scim.GroupSchema),
		},
		{
			name: "resource types",
			path: "/ResourceTypes",
			expected: fmt.Sprintf(`{
  "schemas": ["%s"],
  "totalResults": 2,
  "startIndex": 1,
  "itemsPerPage": 2,
  "Resources": [
    {
      "schemas": ["%s"],
      "id": "User",
      "name": "User",
      "description": "User Account",
      "endpoint": "/Users",
      "schema": "%s",
      "meta": {"resourceType": "ResourceType", "location": "%s/ResourceTypes/User"}
    },
    {
      "schemas": ["%s"],
      "id": "Group",
      "name": "Group",
      "description": "Group",
      "endpoint": "/Groups",
      "schema": "%s",
      "meta": {"resourceType": "ResourceType", "location": "%s/ResourceTypes/Group"}
    }
  ]
}`, scim.ListResponseSchema, scim.ResourceTypeSchema, scim.UserSchema, base, scim.ResourceTypeSchema, scim.GroupSchema, base),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := f.do(http.MethodGet, p.Slug, p.Token, tc.path, nil)
			require.Equal(t, http.StatusOK, resp.Code, "body: %s", resp)
			require.JSONEq(t, tc.expected, resp.String())
		})
	}
}
