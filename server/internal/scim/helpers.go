package scim

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
)

func baseURLFromRequest(r *http.Request, slug string) string {
	scheme := "https"
	if r.TLS == nil {
		if fwd := r.Header.Get("X-Forwarded-Proto"); fwd == "https" || fwd == "http" {
			scheme = fwd
		} else {
			scheme = "http"
		}
	}
	return fmt.Sprintf("%s://%s/scim/v2/%s", scheme, r.Host, slug)
}

func fingerprint(v string) string {
	if v == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}

func normalizeEmail(v string) string { return strings.ToLower(strings.TrimSpace(v)) }

func formatExternalName(name *SCIMName) string {
	if name == nil {
		return ""
	}
	if name.Formatted != "" {
		return name.Formatted
	}
	parts := make([]string, 0, 2)
	if name.GivenName != "" {
		parts = append(parts, name.GivenName)
	}
	if name.FamilyName != "" {
		parts = append(parts, name.FamilyName)
	}
	return strings.Join(parts, " ")
}
