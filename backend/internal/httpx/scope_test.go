package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tngxola-code/nfa-planners-tender-workspace/backend/internal/auth"
)

func okHandler(t *testing.T, ran *bool) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*ran = true
		w.WriteHeader(http.StatusOK)
	})
}

func withClaims(r *http.Request, scopes ...string) *http.Request {
	return r.WithContext(auth.WithClaims(r.Context(), &auth.Claims{
		Subject: "8f1a-user",
		AZP:     "nfa-console",
		Scopes:  scopes,
	}))
}

func TestRequireScopeAllowsTheGrantedScope(t *testing.T) {
	var ran bool
	rec := httptest.NewRecorder()
	req := withClaims(httptest.NewRequest(http.MethodGet, "/v1/tenders", nil),
		"openid", "tenders:read")

	RequireScope("tenders:read")(okHandler(t, &ran)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if !ran {
		t.Error("the handler did not run")
	}
}

func TestRequireScopeDeniesAnUngrantedScope(t *testing.T) {
	var ran bool
	rec := httptest.NewRecorder()
	// A planner token: realm.json grants it tenders:read, never admin:users.
	req := withClaims(httptest.NewRequest(http.MethodGet, "/v1/admin/users", nil),
		"openid", "tenders:read", "searches:write")

	RequireScope("admin:users")(okHandler(t, &ran)).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
	if ran {
		t.Error("the handler ran without the required scope")
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("content-type = %q", ct)
	}
	if body := rec.Body.String(); contains(body, "admin:users") {
		t.Error("the response names the required scope")
	}
}

func TestRequireScopeDeniesWhenClaimsAreAbsent(t *testing.T) {
	var ran bool
	rec := httptest.NewRecorder()
	// No Authenticate ahead of it: a misordered chain must not serve.
	req := httptest.NewRequest(http.MethodGet, "/v1/tenders", nil)

	RequireScope("tenders:read")(okHandler(t, &ran)).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if ran {
		t.Error("the handler ran with no claims in the context")
	}
}

func TestRequireScopeIsExactNotPrefix(t *testing.T) {
	var ran bool
	rec := httptest.NewRecorder()
	req := withClaims(httptest.NewRequest(http.MethodGet, "/v1/tenders", nil),
		"tenders:readonly", "tenders")

	RequireScope("tenders:read")(okHandler(t, &ran)).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 — scope matching must be exact", rec.Code)
	}
}

func TestScopedMuxRefusesAnUnmappedRoute(t *testing.T) {
	m := NewScopedMux(func(h http.Handler) http.Handler { return h })
	err := m.Handle("GET /v1/not-in-the-map", http.NotFoundHandler())
	if err == nil {
		t.Fatal("expected a registration error for a route with no scope decision")
	}
}

func TestScopedMuxRefusesADoubleDecision(t *testing.T) {
	PublicRoutes["GET /v1/tenders"] = true
	defer delete(PublicRoutes, "GET /v1/tenders")

	m := NewScopedMux(func(h http.Handler) http.Handler { return h })
	if err := m.Handle("GET /v1/tenders", http.NotFoundHandler()); err == nil {
		t.Fatal("expected an error when a route is both scoped and public")
	}
}

func TestScopedMuxWrapsAScopedRoute(t *testing.T) {
	var authnRan bool
	m := NewScopedMux(func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authnRan = true
			h.ServeHTTP(w, withClaims(r, "tenders:read"))
		})
	})
	var ran bool
	if err := m.Handle("GET /v1/tenders", okHandler(t, &ran)); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/tenders", nil))

	if !authnRan {
		t.Error("the authenticator was not applied to a scoped route")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestScopedMuxServesAPublicRouteWithoutAuthn(t *testing.T) {
	var authnRan bool
	m := NewScopedMux(func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authnRan = true
			h.ServeHTTP(w, r)
		})
	})
	var ran bool
	if err := m.Handle("GET /healthz", okHandler(t, &ran)); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if authnRan {
		t.Error("a public route was wrapped in the authenticator")
	}
	if !ran {
		t.Error("the public handler did not run")
	}
}

// Every scope in the map must exist as a client scope in realm.json.
func TestEveryMappedScopeExistsInTheRealm(t *testing.T) {
	realmScopes := map[string]bool{
		"tenders:read": true, "tenders:write": true, "awards:read": true,
		"contracts:read": true, "buyers:read": true, "parties:read": true,
		"documents:read": true, "search:read": true, "searches:read": true,
		"searches:write": true, "alerts:read": true, "alerts:write": true,
		"workflows:read": true, "workflows:write": true, "reports:read": true,
		"audit:read": true, "privacy:read": true, "ingest:read": true,
		"admin:users": true,
	}
	for route, scope := range RouteScopes {
		if !realmScopes[scope] {
			t.Errorf("%s requires %q, which realm.json does not declare", route, scope)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		func() bool {
			for i := 0; i+len(needle) <= len(haystack); i++ {
				if haystack[i:i+len(needle)] == needle {
					return true
				}
			}
			return false
		}()
}
