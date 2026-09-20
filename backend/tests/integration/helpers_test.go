//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/tngxola-code/nfa-planners-tender-workspace/backend/internal/auth"
	"github.com/tngxola-code/nfa-planners-tender-workspace/backend/internal/diagnostics"
	"github.com/tngxola-code/nfa-planners-tender-workspace/backend/internal/httpx"
)

// userToken issues a token for one of the realm's test users via the ROPC
// test client. Test users and their passwords are declared in realm.json.
func userToken(t *testing.T, user, scope string) string {
	t.Helper()
	form := url.Values{
		"grant_type": {"password"},
		"client_id":  {"nfa-test"},
		"username":   {user + "@local"},
		"password":   {user},
		"scope":      {strings.TrimSpace("openid " + scope)},
	}
	return postToken(t, form)
}

// serviceToken issues a client-credentials token for the internal service
// client. Its secret is the local-only one in realm.json.
func serviceToken(t *testing.T, scope string) string {
	t.Helper()
	return postToken(t, url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {"nfa-service"},
		"client_secret": {"local-service-secret-replace-in-prod"},
		"scope":         {scope},
	})
}

func postToken(t *testing.T, form url.Values) string {
	t.Helper()
	resp, err := http.PostForm(issuer+"/protocol/openid-connect/token", form)
	if err != nil {
		t.Fatalf("token request: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		AccessToken      string `json:"access_token"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	if body.AccessToken == "" {
		t.Fatalf("no access token (status %d): %s %s",
			resp.StatusCode, body.Error, body.ErrorDescription)
	}
	return body.AccessToken
}

// newServer builds the real middleware chain — the same verifier, the same
// ScopedMux, the same handlers cmd/api wires — in process.
//
// nfa-test is admitted here because the suite has no browser to run a PKCE
// flow with. A deployed instance admits it only via
// OIDC_ALLOWED_AZP_TEST_CLIENTS, which is empty by default.
func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	v, err := auth.NewVerifier(auth.Config{
		IssuerURL:  issuer,
		AllowedAZP: []string{"nfa-console", "nfa-mobile", "nfa-service", "nfa-test"},
	})
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	mux := httpx.NewScopedMux(httpx.Authenticator(v))
	for pattern, h := range diagnostics.Routes() {
		if err := mux.Handle(pattern, h); err != nil {
			t.Fatalf("register %s: %v", pattern, err)
		}
	}

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// get issues a request with an optional bearer token and returns the status
// and body.
func get(t *testing.T, srv *httptest.Server, path, token string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()

	buf := make([]byte, 8192)
	n, _ := resp.Body.Read(buf)
	return resp.StatusCode, buf[:n]
}
