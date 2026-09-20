//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// The realm must issue tokens that identify someone. A realm whose clients
// reference client scopes it does not declare issues tokens with no sub,
// which verify cleanly and audit to nobody.
func TestRealmIssuesIdentifiableTokens(t *testing.T) {
	srv := newServer(t)
	status, body := get(t, srv, "/v1/whoami", userToken(t, "planner", "tenders:read"))
	if status != http.StatusOK {
		t.Fatalf("status %d: %s", status, body)
	}

	var who struct {
		Subject    string   `json:"subject"`
		Username   string   `json:"username"`
		ClientID   string   `json:"clientId"`
		Scopes     []string `json:"scopes"`
		RealmRoles []string `json:"realmRoles"`
	}
	if err := json.Unmarshal(body, &who); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if who.Subject == "" {
		t.Error("subject is empty: the realm issues tokens that identify nobody")
	}
	if who.Username != "planner@local" {
		t.Errorf("username = %q, want planner@local", who.Username)
	}
	if who.ClientID != "nfa-test" {
		t.Errorf("clientId = %q", who.ClientID)
	}
	if len(who.RealmRoles) == 0 || who.RealmRoles[0] != "planner" {
		t.Errorf("realmRoles = %v, want [planner]", who.RealmRoles)
	}
}

// Every user in the realm must resolve to a distinct subject.
func TestEachUserHasItsOwnSubject(t *testing.T) {
	srv := newServer(t)
	seen := map[string]string{}
	for _, u := range []string{"planner", "analyst", "admin", "officer"} {
		status, body := get(t, srv, "/v1/whoami", userToken(t, u, ""))
		if status != http.StatusOK {
			t.Fatalf("%s: status %d: %s", u, status, body)
		}
		var who struct {
			Subject string `json:"subject"`
		}
		if err := json.Unmarshal(body, &who); err != nil {
			t.Fatal(err)
		}
		if prev, dup := seen[who.Subject]; dup {
			t.Errorf("%s and %s share subject %s", u, prev, who.Subject)
		}
		seen[who.Subject] = u
	}
}

// The scope model, enforced end to end: role → scope grant → route.
func TestScopeEnforcement(t *testing.T) {
	srv := newServer(t)
	cases := []struct {
		user, scope, path string
		want              int
	}{
		{"planner", "admin:users", "/v1/admin/ping", http.StatusForbidden},
		{"analyst", "admin:users", "/v1/admin/ping", http.StatusForbidden},
		{"officer", "admin:users", "/v1/admin/ping", http.StatusForbidden},
		{"admin", "admin:users", "/v1/admin/ping", http.StatusOK},
		{"planner", "ingest:read", "/v1/service/ping", http.StatusForbidden},
		{"admin", "ingest:read", "/v1/service/ping", http.StatusOK},
	}
	for _, tc := range cases {
		name := tc.user + strings.ReplaceAll(tc.path, "/", "_")
		t.Run(name, func(t *testing.T) {
			status, body := get(t, srv, tc.path, userToken(t, tc.user, tc.scope))
			if status != tc.want {
				t.Errorf("status %d, want %d: %s", status, tc.want, body)
			}
		})
	}
}

// A denial must not disclose which scope the route requires.
func TestDenialDoesNotNameTheScope(t *testing.T) {
	srv := newServer(t)
	_, body := get(t, srv, "/v1/admin/ping", userToken(t, "planner", "admin:users"))
	if strings.Contains(string(body), "admin:users") {
		t.Errorf("403 body names the required scope: %s", body)
	}
}

// The ingest worker authenticates as a client, not a person.
func TestServiceClientReachesIngestRoute(t *testing.T) {
	srv := newServer(t)
	status, body := get(t, srv, "/v1/service/ping", serviceToken(t, "ingest:read"))
	if status != http.StatusOK {
		t.Fatalf("status %d: %s", status, body)
	}

	status, body = get(t, srv, "/v1/admin/ping", serviceToken(t, "ingest:read"))
	if status != http.StatusForbidden {
		t.Errorf("service client reached an admin route: %d %s", status, body)
	}
}

func TestUnauthenticatedIsRefused(t *testing.T) {
	srv := newServer(t)
	for _, path := range []string{"/v1/whoami", "/v1/admin/ping", "/v1/service/ping"} {
		if status, _ := get(t, srv, path, ""); status != http.StatusUnauthorized {
			t.Errorf("%s with no token: %d, want 401", path, status)
		}
		if status, _ := get(t, srv, path, "not.a.jwt"); status != http.StatusUnauthorized {
			t.Errorf("%s with garbage token: %d, want 401", path, status)
		}
	}
}
