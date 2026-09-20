package diagnostics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tngxola-code/nfa-planners-tender-workspace/backend/internal/auth"
	"github.com/tngxola-code/nfa-planners-tender-workspace/backend/internal/httpx"
)

func request(scopes []string, roles []string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/v1/whoami", nil)
	return r.WithContext(auth.WithClaims(r.Context(), &auth.Claims{
		Subject:    "8f1a-user",
		Username:   "planner@local",
		Email:      "planner@local",
		AZP:        "nfa-console",
		Scopes:     scopes,
		RealmRoles: roles,
		ExpiresAt:  time.Now().Add(15 * time.Minute),
	}))
}

func TestWhoamiReflectsTheCallersClaims(t *testing.T) {
	rec := httptest.NewRecorder()
	Whoami(rec, request([]string{"openid", "tenders:read"}, []string{"planner"}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("cache-control = %q, want no-store", got)
	}

	var body WhoamiResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Subject != "8f1a-user" {
		t.Errorf("subject = %q", body.Subject)
	}
	if body.ClientID != "nfa-console" {
		t.Errorf("clientId = %q", body.ClientID)
	}
	if len(body.Scopes) != 2 || body.Scopes[1] != "tenders:read" {
		t.Errorf("scopes = %v", body.Scopes)
	}
	if len(body.RealmRoles) != 1 || body.RealmRoles[0] != "planner" {
		t.Errorf("realmRoles = %v", body.RealmRoles)
	}
}

func TestWhoamiEmitsEmptyArraysNotNull(t *testing.T) {
	rec := httptest.NewRecorder()
	Whoami(rec, request(nil, nil))

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"scopes", "realmRoles"} {
		if string(raw[field]) != "[]" {
			t.Errorf("%s = %s, want []", field, raw[field])
		}
	}
}

func TestWhoamiDeniesWithoutClaims(t *testing.T) {
	rec := httptest.NewRecorder()
	Whoami(rec, httptest.NewRequest(http.MethodGet, "/v1/whoami", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("content-type = %q", ct)
	}
}

func TestPingReportsTheScope(t *testing.T) {
	rec := httptest.NewRecorder()
	ping("admin:users")(rec, request([]string{"admin:users"}, []string{"admin"}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body PingResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.OK || body.Scope != "admin:users" {
		t.Errorf("body = %+v", body)
	}
}

// Every diagnostic route must be registrable: ScopedMux refuses a pattern
// with no scope decision, so this fails if a route and its decision drift.
func TestEveryRouteIsRegistrable(t *testing.T) {
	m := httpx.NewScopedMux(func(h http.Handler) http.Handler { return h })
	for pattern, h := range Routes() {
		if err := m.Handle(pattern, h); err != nil {
			t.Errorf("%s: %v", pattern, err)
		}
	}
}

// The scope each ping reports must match what RouteScopes requires of it.
// A mismatch would have the endpoint claim one thing while the mux enforced
// another.
func TestPingScopesMatchTheRouteMap(t *testing.T) {
	cases := map[string]string{
		"GET /v1/admin/ping":   "admin:users",
		"GET /v1/service/ping": "ingest:read",
	}
	for pattern, want := range cases {
		if got := httpx.RouteScopes[pattern]; got != want {
			t.Errorf("%s: RouteScopes says %q, handler reports %q", pattern, got, want)
		}
	}
}
