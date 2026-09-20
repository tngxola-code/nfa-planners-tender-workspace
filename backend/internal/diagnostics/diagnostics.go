// Package diagnostics serves the stage-0a endpoints that prove the auth
// chain end to end. They carry no domain data: whoami reflects the caller's
// own verified claims back, and the two pings assert that a scope is held.
//
// They exist so the chain can be exercised against a real realm before any
// domain code depends on it.
package diagnostics

import (
	"encoding/json"
	"net/http"

	"github.com/tngxola-code/nfa-planners-tender-workspace/backend/internal/auth"
	"github.com/tngxola-code/nfa-planners-tender-workspace/backend/internal/httpx"
)

// WhoamiResponse reflects the caller's verified identity. It carries only
// claims the caller's own token already contains, so it discloses nothing
// they could not read themselves.
type WhoamiResponse struct {
	Subject    string   `json:"subject"`
	Username   string   `json:"username,omitempty"`
	Email      string   `json:"email,omitempty"`
	ClientID   string   `json:"clientId"`
	Scopes     []string `json:"scopes"`
	RealmRoles []string `json:"realmRoles"`
	ExpiresAt  string   `json:"expiresAt"`
}

// PingResponse confirms that a scope was held.
type PingResponse struct {
	OK      bool   `json:"ok"`
	Scope   string `json:"scope"`
	Subject string `json:"subject"`
}

// Routes returns the diagnostic patterns and their handlers, for
// registration through httpx.ScopedMux. The scopes are declared in
// httpx.RouteScopes, not here: this package does not decide authorization.
func Routes() map[string]http.Handler {
	return map[string]http.Handler{
		"GET /v1/whoami":       http.HandlerFunc(Whoami),
		"GET /v1/admin/ping":   http.HandlerFunc(ping("admin:users")),
		"GET /v1/service/ping": http.HandlerFunc(ping("ingest:read")),
	}
}

// Whoami returns the caller's verified claims.
func Whoami(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		// Unreachable behind Authenticate. If it happens the chain is
		// misordered, so deny rather than assume.
		httpx.WriteProblem(w, r, http.StatusUnauthorized,
			"Unauthorized", "A bearer token is required.")
		return
	}

	writeJSON(w, r, http.StatusOK, WhoamiResponse{
		Subject:    claims.Subject,
		Username:   claims.Username,
		Email:      claims.Email,
		ClientID:   claims.AZP,
		Scopes:     nonNil(claims.Scopes),
		RealmRoles: nonNil(claims.RealmRoles),
		ExpiresAt:  claims.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z"),
	})
}

// ping builds a handler that confirms the named scope was held. Enforcement
// happened in RequireScope before this ran; the handler only reports it.
func ping(scope string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := auth.FromContext(r.Context())
		if !ok {
			httpx.WriteProblem(w, r, http.StatusUnauthorized,
				"Unauthorized", "A bearer token is required.")
			return
		}
		writeJSON(w, r, http.StatusOK, PingResponse{
			OK:      true,
			Scope:   scope,
			Subject: claims.Subject,
		})
	}
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		// The status line is already written; nothing useful remains.
		return
	}
}

// nonNil keeps an empty slice from marshalling as null.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
