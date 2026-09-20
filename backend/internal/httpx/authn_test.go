package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tngxola-code/nfa-planners-tender-workspace/backend/internal/auth"
)

// newTestVerifier builds a verifier pointed at an unreachable issuer/JWKS so
// every test exercises the failure paths without network. It is safe to
// share across parallel subtests.
func newTestVerifier(t *testing.T) *auth.Verifier {
	t.Helper()
	v, err := auth.NewVerifier(auth.Config{
		IssuerURL:  "http://127.0.0.1:1/realms/nfa",
		AllowedAZP: []string{"nfa-console"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestAuthenticator_MissingHeader(t *testing.T) {
	t.Parallel()
	v := newTestVerifier(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/whoami", nil)

	Authenticator(v)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the next handler ran")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if got := rec.Header().Get("WWW-Authenticate"); !strings.Contains(got, `realm="nfa"`) {
		t.Errorf("WWW-Authenticate = %q, want it to contain realm=nfa", got)
	}
}

func TestAuthenticator_WrongScheme(t *testing.T) {
	t.Parallel()
	v := newTestVerifier(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/whoami", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")

	Authenticator(v)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the next handler ran")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

// TestAuthenticator_JWKSUnreachable is a regression test: an unreachable
// JWKS must produce 503, not 401. A 401 here would tell the caller their
// token is bad when in fact the verifier simply could not check it.
func TestAuthenticator_JWKSUnreachable(t *testing.T) {
	t.Parallel()
	v := newTestVerifier(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/whoami", nil)
	req.Header.Set("Authorization", "Bearer eyJhbGciOiJSUzI1NiIsImtpZCI6IngifQ.e30.x")

	Authenticator(v)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the next handler ran")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 — an unreachable JWKS must not present as a bad token",
			rec.Code)
	}
}

// TestAuthenticator_AlgNoneIs401 asserts the alg=none path is treated as a
// bad token (401), not as an infrastructure failure (503).
func TestAuthenticator_AlgNoneIs401(t *testing.T) {
	t.Parallel()
	v := newTestVerifier(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/whoami", nil)
	// header={"alg":"none","typ":"JWT"} payload={"sub":"u"} empty signature
	req.Header.Set("Authorization",
		"Bearer eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJzdWIiOiJ1In0.")

	Authenticator(v)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the next handler ran")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if got := rec.Header().Get("WWW-Authenticate"); !strings.Contains(got, `error="invalid_token"`) {
		t.Errorf("WWW-Authenticate = %q, want error=invalid_token", got)
	}
}
