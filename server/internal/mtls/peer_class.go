package mtls

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

type peerCertificateKey struct{}
type deviceIDKey struct{}

func WithDeviceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, deviceIDKey{}, id)
}

func DeviceIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	id, ok := ctx.Value(deviceIDKey{}).(string)
	return id, ok && id != ""
}

func WithPeerCertificate(ctx context.Context, cert *x509.Certificate) context.Context {
	return context.WithValue(ctx, peerCertificateKey{}, cert)
}

func PeerCertificateFromContext(ctx context.Context) (*x509.Certificate, bool) {
	if ctx == nil {
		return nil, false
	}
	cert, ok := ctx.Value(peerCertificateKey{}).(*x509.Certificate)
	return cert, ok && cert != nil
}

func PeerSerialFromContext(ctx context.Context) (string, bool) {
	cert, ok := PeerCertificateFromContext(ctx)
	if !ok || cert.SerialNumber == nil || cert.SerialNumber.Sign() < 0 {
		return "", false
	}
	return cert.SerialNumber.Text(16), true
}

type PeerClass string

const (
	PeerClassAgent PeerClass = "agent"

	PeerClassControl PeerClass = "control"
)

const (
	peerClassURIScheme = "spiffe"
	peerClassURIHost   = "cadestro"
)

func peerClassURI(class PeerClass) string {
	return fmt.Sprintf("%s://%s/%s", peerClassURIScheme, peerClassURIHost, class)
}

func PeerClassFromCert(cert *x509.Certificate) (PeerClass, error) {
	if cert == nil {
		return "", errors.New("nil certificate")
	}
	var found PeerClass
	for _, u := range cert.URIs {
		if u == nil {
			continue
		}
		if u.Scheme != peerClassURIScheme || u.Host != peerClassURIHost {
			continue
		}
		class := PeerClass(strings.TrimPrefix(u.Path, "/"))
		if class == "" {
			continue
		}
		if found != "" && found != class {
			return "", fmt.Errorf("certificate carries multiple peer-class URIs (%q and %q)", found, class)
		}
		found = class
	}
	if found == "" {
		return "", errors.New("certificate has no peer-class URI SAN")
	}
	switch found {
	case PeerClassAgent, PeerClassControl:
		return found, nil
	default:
		return "", fmt.Errorf("unknown peer class %q", found)
	}
}

func PeerClassFromTLS(state *tls.ConnectionState) (PeerClass, error) {
	if state == nil {
		return "", errors.New("no TLS connection state")
	}
	if len(state.PeerCertificates) == 0 {
		return "", errors.New("no peer certificate")
	}
	return PeerClassFromCert(state.PeerCertificates[0])
}

func RequirePeerClass(logger *slog.Logger, allowed ...PeerClass) func(http.Handler) http.Handler {
	allowSet := make(map[PeerClass]struct{}, len(allowed))
	for _, c := range allowed {
		allowSet[c] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" || r.URL.Path == "/ready" {
				next.ServeHTTP(w, r)
				return
			}
			if r.TLS == nil {
				http.Error(w, "mTLS required", http.StatusUnauthorized)
				return
			}
			class, err := PeerClassFromTLS(r.TLS)
			if err != nil {
				if logger != nil {
					logger.Warn("peer-class check failed: cert missing class",
						"remote_addr", r.RemoteAddr,
						"path", r.URL.Path,
						"error", err,
					)
				}
				http.Error(w, "peer class required", http.StatusForbidden)
				return
			}
			if _, ok := allowSet[class]; !ok {
				if logger != nil {
					logger.Warn("peer-class check failed: wrong class",
						"remote_addr", r.RemoteAddr,
						"path", r.URL.Path,
						"presented", class,
						"allowed", allowedClassString(allowed),
					)
				}
				http.Error(w, "peer class not allowed", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func allowedClassString(classes []PeerClass) string {
	out := make([]string, 0, len(classes))
	for _, c := range classes {
		out = append(out, string(c))
	}
	return strings.Join(out, ",")
}

func PeerClassURI(class PeerClass) (*url.URL, error) {
	switch class {
	case PeerClassAgent, PeerClassControl:
	default:
		return nil, fmt.Errorf("unknown peer class %q", class)
	}
	return url.Parse(peerClassURI(class))
}
