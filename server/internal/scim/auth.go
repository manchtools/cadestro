package scim

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/store"
)

const bearerScheme = "Bearer "

const absentTokenDigest = "0000000000000000000000000000000000000000000000000000000000000000"

func (h *Handler) withAuth(descriptor string, next routeHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		slug := r.PathValue("slug")
		if slug == "" {
			writeError(w, http.StatusBadRequest, "missing provider slug")
			return
		}
		if !acceptableContentType(r) {
			writeError(w, http.StatusUnsupportedMediaType,
				"Content-Type must be application/scim+json or application/json")
			return
		}

		clientIP := auth.ClientIPFromHTTP(r)

		if !h.providerLimiter.Allow(slug) {
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		if clientIP != "" && !h.providerIPLimit.Allow(slug+"|"+clientIP) {
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}

		token, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok {
			h.refuse(ctx, w, descriptor, reasonMissingCredentials, token, clientIP)
			return
		}
		presented := fingerprint(token)

		expected := absentTokenDigest
		reason := ""
		provider, err := h.store.GetIdentityProviderBySlug(ctx, slug)
		switch {
		case err != nil && store.IsNotFound(err):
			reason = reasonUnknownProvider
		case err != nil:
			h.logger.Error("scim: failed to resolve provider", "error", err)
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		case !provider.Enabled:

			reason = reasonProviderDisabled
		case !provider.ScimEnabled:
			reason = reasonSCIMDisabled
		case provider.ScimTokenHash == "":
			reason = reasonNoTokenConfigured
		default:
			expected = provider.ScimTokenHash
		}

		matched := subtle.ConstantTimeCompare([]byte(presented), []byte(expected)) == 1
		if reason == "" && !matched {
			reason = reasonInvalidToken
		}
		if reason != "" {
			h.refuse(ctx, w, descriptor, reason, token, clientIP)
			return
		}

		next(w, r, &session{
			provider:          provider,
			descriptor:        descriptor,
			tokenFingerprint:  presented,
			originFingerprint: fingerprint(clientIP),
		})
	}
}

func (h *Handler) refuse(ctx context.Context, w http.ResponseWriter, descriptor, reason, credential, clientIP string) {

	h.logger.Warn("scim: refused credential", "route", descriptor, "reason", reason)

	if h.rejectionLimiter.Allow("rej:" + clientIP) {
		_, err := h.store.RecordOperation(ctx, store.AuditOperation{
			Class: store.ClassRejectedAuthentication,

			ActorType:            auth.AnonymousActorType,
			ActorFingerprint:     fingerprint(credential),
			Origin:               Origin,
			OriginFingerprint:    fingerprint(clientIP),
			RequestDescriptor:    descriptor,
			AuthorizationOutcome: store.AuthorizationDenied,
			AuthorizationDetail:  AuthorizationDetail,
			Result:               store.ResultRejected,
			ResultCode:           reason,
		})
		if err != nil {
			h.logger.Error("scim: failed to record rejected authentication",
				"route", descriptor, "reason", reason, "error", err)
		}
	}

	writeError(w, http.StatusUnauthorized, "invalid credentials")
}

func bearerToken(header string) (string, bool) {
	if !strings.HasPrefix(header, bearerScheme) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, bearerScheme))
	return token, token != ""
}

func acceptableContentType(r *http.Request) bool {
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
	default:
		return true
	}
	ct := r.Header.Get("Content-Type")
	return ct == "" ||
		strings.HasPrefix(ct, scimContentType) ||
		strings.HasPrefix(ct, "application/json")
}
