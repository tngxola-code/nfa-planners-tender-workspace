package identity

import "context"

// Source describes where a caller identity was observed.
type Source string

const (
	SourceNone Source = "none"
	SourceMTLS Source = "mtls"
)

// Identity is the caller identity as observed at the transport layer. The
// zero value is not meaningful; construct anonymous identities with
// Anonymous().
type Identity struct {
	Source Source

	// Subject is the raw RFC 2253 DN from the client certificate, exactly
	// as the TLS terminator reported it. It is authoritative when
	// Source == SourceMTLS: the cert was verified by the proxy.
	Subject string

	// Structured fields parsed out of Subject. They are best-effort: if
	// Subject cannot be parsed, these remain empty and callers should
	// fall back to Subject. Never trust these fields to be present just
	// because Source == SourceMTLS.
	CommonName string
	Org        string
	OrgUnit    []string
	Country    string

	// Serial and Fingerprint are optional; the terminator may not emit
	// them. When present they are useful for audit logs and revocation
	// lookups.
	Serial      string
	Fingerprint string
}

// Anonymous returns the identity of a caller that presented no credential.
func Anonymous() Identity { return Identity{Source: SourceNone} }

// IsAnonymous reports whether the caller presented no credential. A zero
// Source is treated as anonymous so the zero value is safe to hand to
// downstream code by accident.
func (i Identity) IsAnonymous() bool {
	return i.Source == "" || i.Source == SourceNone
}

type ctxKey struct{}

// With returns a context carrying the identity.
func With(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext returns the identity attached to ctx. The second return
// value is false if no identity was attached; callers should prefer
// Anonymous() in that case so they handle "no identity" uniformly.
func FromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(ctxKey{}).(Identity)
	return id, ok
}
