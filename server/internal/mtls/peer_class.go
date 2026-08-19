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

// WithPeerCertificate binds the certificate verified by the TLS stack to a
// request. Application identity must come from this value, never from a PEM
// field in an agent request.
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

// PeerClass identifies the role of a mTLS peer. The internal CA
// issues every non-CA certificate with exactly one URI SAN of the
// form `spiffe://cadestro/<class>`, where `<class>` is one of
// the constants below. Middleware on each listener requires a
// specific class so a leaked cert of one class (e.g. an agent
// cert pulled from a compromised host) cannot be used to reach
// a listener intended for another class (e.g. the control
// agent listener, which accepts only agent peers).
//
// The SPIFFE URI shape is standard, machine-readable, and puts the
// class in a field (SAN URI) that X.509 parsers treat as structured
// data — unlike the CN, which is a free-form string reused for
// device IDs on agent certs.
type PeerClass string

const (
	// PeerClassAgent identifies a managed-device cert issued by the
	// control server's Register / RenewCertificate RPC. Agents
	// present this on control's own mTLS listener.
	PeerClassAgent PeerClass = "agent"
	// PeerClassControl identifies the control server's own cert,
	// issued out of band by setup.sh and presented on its agent
	// listener.
	PeerClassControl PeerClass = "control"
)

// peerClassURIScheme and peerClassURIHost match the URI SAN layout
// that ca.IssueCertificateFromCSR emits for agent certs and that
// setup.sh emits for the control cert. Keeping them in one
// place makes it obvious where to add a new class.
const (
	peerClassURIScheme = "spiffe"
	peerClassURIHost   = "cadestro"
)

// peerClassURI builds the canonical SPIFFE URI for a class. Kept in
// one place so emitters (CA + setup.sh) and verifiers agree.
func peerClassURI(class PeerClass) string {
	return fmt.Sprintf("%s://%s/%s", peerClassURIScheme, peerClassURIHost, class)
}

// PeerClassFromCert inspects the URI SANs on a peer certificate and
// returns the identified class, or an error if the cert carries no
// `spiffe://cadestro/<class>` URI or carries more than one such
// URI (ambiguous class is a hard error — the CA MUST emit exactly
// one).
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

// PeerClassFromTLS extracts the peer class from the first peer
// certificate of a TLS connection state. Callers that already have
// an *x509.Certificate should use PeerClassFromCert directly.
func PeerClassFromTLS(state *tls.ConnectionState) (PeerClass, error) {
	if state == nil {
		return "", errors.New("no TLS connection state")
	}
	if len(state.PeerCertificates) == 0 {
		return "", errors.New("no peer certificate")
	}
	return PeerClassFromCert(state.PeerCertificates[0])
}

// RequirePeerClass returns middleware that extracts the peer class
// from the client certificate and rejects requests whose peer does
// not match one of allowed. Health endpoints (/health, /ready) are
// passed through untouched so they work without mTLS on the ops
// listener.
//
// The classes are allowed as a set (variadic) rather than a single
// class so a listener that serves multiple peer populations (not
// currently needed, but possible — e.g. an endpoint
// reachable by both control and admin CLI peers) does not need to
// be wrapped twice.
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

// PeerClassURI returns the SPIFFE URI shape a CA emitter should
// stamp onto a newly-issued certificate for the given class. Kept
// exported so ca.IssueCertificateFromCSR can use it without
// duplicating the format literal.
func PeerClassURI(class PeerClass) (*url.URL, error) {
	switch class {
	case PeerClassAgent, PeerClassControl:
	default:
		return nil, fmt.Errorf("unknown peer class %q", class)
	}
	return url.Parse(peerClassURI(class))
}
