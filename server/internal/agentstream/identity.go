package agentstream

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/manchtools/cadestro/server/internal/mtls"
)

type deviceIdentityKey struct{}

func WithDeviceID(ctx context.Context, deviceID string) context.Context {
	return context.WithValue(ctx, deviceIdentityKey{}, deviceID)
}

func DeviceIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	deviceID, ok := ctx.Value(deviceIdentityKey{}).(string)
	return deviceID, ok
}

type writeDeadlinerKey struct{}

type writeDeadliner interface {
	SetWriteDeadline(time.Time) error
}

func withWriteDeadliner(ctx context.Context, deadliner writeDeadliner) context.Context {
	return context.WithValue(ctx, writeDeadlinerKey{}, deadliner)
}

func writeDeadlinerFrom(ctx context.Context) writeDeadliner {
	if ctx == nil {
		return nil
	}
	deadliner, _ := ctx.Value(writeDeadlinerKey{}).(writeDeadliner)
	return deadliner
}

func MTLSMiddleware(next http.Handler) http.Handler {
	if next == nil {
		panic("agentstream: mTLS middleware requires a handler")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" || request.URL.Path == "/ready" {
			next.ServeHTTP(w, request)
			return
		}
		deviceID, err := mtls.DeviceIDFromRequest(request)
		if err != nil {
			http.Error(w, "client certificate required", http.StatusUnauthorized)
			return
		}
		peerClass, err := mtls.PeerClassFromTLS(request.TLS)
		if err != nil || peerClass != mtls.PeerClassAgent {
			http.Error(w, "agent certificate required", http.StatusForbidden)
			return
		}
		if len(request.TLS.PeerCertificates) == 0 {
			http.Error(w, "client certificate required", http.StatusUnauthorized)
			return
		}
		ctx := WithDeviceID(request.Context(), deviceID)
		ctx = mtls.WithDeviceID(ctx, deviceID)
		ctx = mtls.WithPeerCertificate(ctx, request.TLS.PeerCertificates[0])
		controller := http.NewResponseController(w)
		if err := controller.SetWriteDeadline(time.Time{}); err == nil || !errors.Is(err, http.ErrNotSupported) {
			ctx = withWriteDeadliner(ctx, controller)
		}
		next.ServeHTTP(w, request.WithContext(ctx))
	})
}
