package httpx

import (
	"fmt"
	"net/http"
)

// ScopedMux registers routes only when their scope decision is recorded. A
// pattern absent from RouteScopes, PublicRoutes and AuthenticatedRoutes is a
// registration error, so "forgot to add a scope" fails at wiring time rather
// than serving unprotected.
type ScopedMux struct {
	mux      *http.ServeMux
	authn    func(http.Handler) http.Handler
	patterns []string
}

func NewScopedMux(authn func(http.Handler) http.Handler) *ScopedMux {
	return &ScopedMux{mux: http.NewServeMux(), authn: authn}
}

// Handle registers h at pattern ("GET /v1/tenders"), wrapping it in whatever
// the scope decision for that pattern requires.
func (m *ScopedMux) Handle(pattern string, h http.Handler) error {
	scope, scoped := RouteScopes[pattern]
	public := PublicRoutes[pattern]
	authed := AuthenticatedRoutes[pattern]

	switch {
	case scoped && (public || authed),
		public && authed:
		return fmt.Errorf("httpx: %q has more than one scope decision", pattern)
	case scoped:
		h = m.authn(RequireScope(scope)(h))
	case authed:
		h = m.authn(h)
	case public:
		// served as-is, by decision
	default:
		return fmt.Errorf(
			"httpx: %q has no scope decision; add it to RouteScopes, "+
				"AuthenticatedRoutes or PublicRoutes", pattern)
	}

	m.mux.Handle(pattern, h)
	m.patterns = append(m.patterns, pattern)
	return nil
}

// Patterns returns every registered pattern, for assertions in tests.
func (m *ScopedMux) Patterns() []string {
	out := make([]string, len(m.patterns))
	copy(out, m.patterns)
	return out
}

func (m *ScopedMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mux.ServeHTTP(w, r)
}
