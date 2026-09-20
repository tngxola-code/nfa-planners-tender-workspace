package identity

import (
	"errors"
	"net"
	"net/http"
	"strings"
)

// Header names injected by the TLS terminator. Case-insensitive; Go's
// http.Header canonicalizes on Set/Get.
const (
	HeaderSubject     = "X-Client-Cert-Subject"
	HeaderIssuer      = "X-Client-Cert-Issuer"
	HeaderSerial      = "X-Client-Cert-Serial"
	HeaderFingerprint = "X-Client-Cert-Fingerprint"
)

// Config configures the identity middleware.
type Config struct {
	// TrustedProxies is the set of CIDR ranges whose identity headers are
	// honored. Required and must be non-empty: an empty list would mean
	// "trust nobody", which the middleware refuses to accept silently.
	TrustedProxies []*net.IPNet

	// Logf, if non-nil, is called whenever a request carries identity
	// headers from an untrusted peer, or carries duplicate identity
	// headers. Both are security-relevant and worth surfacing.
	Logf func(format string, args ...any)
}

// Middleware returns an http.Handler wrapper that attaches an
// identity.Identity to the request context.
//
// The middleware is permissive: a request without identity headers still
// reaches the next handler, carrying an anonymous identity. It is the
// caller's job to decide whether anonymous access is acceptable.
func Middleware(cfg Config) (func(http.Handler) http.Handler, error) {
	if len(cfg.TrustedProxies) == 0 {
		return nil, errors.New("identity: TrustedProxies must not be empty")
	}
	logf := cfg.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := Anonymous()

			if hasIdentityHeaders(r.Header) {
				if !peerTrusted(r.RemoteAddr, cfg.TrustedProxies) {
					logf("identity: dropping client-cert headers from untrusted peer %s",
						r.RemoteAddr)
					next.ServeHTTP(w, r.WithContext(With(r.Context(), id)))
					return
				}
				parsed, ok := fromHeaders(r.Header)
				if !ok {
					logf("identity: malformed or duplicate client-cert headers from %s",
						r.RemoteAddr)
				} else {
					id = parsed
				}
			}

			next.ServeHTTP(w, r.WithContext(With(r.Context(), id)))
		})
	}, nil
}

// hasIdentityHeaders reports whether any of the identity headers are present.
// It is cheap and runs on every request; the four names are what Caddy
// would set if it had a verified client cert.
func hasIdentityHeaders(h http.Header) bool {
	return len(h.Values(HeaderSubject)) > 0 ||
		len(h.Values(HeaderIssuer)) > 0 ||
		len(h.Values(HeaderSerial)) > 0 ||
		len(h.Values(HeaderFingerprint)) > 0
}

// peerTrusted reports whether remoteAddr lies within any of nets.
func peerTrusted(remoteAddr string, nets []*net.IPNet) bool {
	host := remoteAddr
	if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = h
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// fromHeaders extracts an Identity from the terminator-injected headers.
// Returns false if the subject header is missing, empty, or duplicated;
// duplicate subject headers signal a misbehaving proxy or a header-
// smuggling attempt.
func fromHeaders(h http.Header) (Identity, bool) {
	subjects := h.Values(HeaderSubject)
	if len(subjects) != 1 {
		return Identity{}, false
	}
	raw := strings.TrimSpace(subjects[0])
	if raw == "" {
		return Identity{}, false
	}

	id := Identity{
		Source:      SourceMTLS,
		Subject:     raw,
		Serial:      singleValue(h, HeaderSerial),
		Fingerprint: singleValue(h, HeaderFingerprint),
	}

	// Parse the DN. If it fails, we still report SourceMTLS with the raw
	// Subject intact: the certificate itself was verified by the proxy,
	// we just cannot extract structured fields from its DN.
	if name, err := ParseDN(raw); err == nil {
		id.CommonName = name.CommonName
		id.Org = name.Organization
		id.OrgUnit = name.OrgUnits
		id.Country = name.Country
	}
	return id, true
}

// singleValue returns the sole value of a header, or "" if the header is
// absent or duplicated.
func singleValue(h http.Header, name string) string {
	vs := h.Values(name)
	if len(vs) != 1 {
		return ""
	}
	return strings.TrimSpace(vs[0])
}
