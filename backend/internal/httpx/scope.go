package httpx

import (
	"net/http"

	"github.com/tngxola-code/nfa-planners-tender-workspace/backend/internal/auth"
)

// RouteScopes is the route-to-scope map. It is the single declaration of what
// each operation requires, and it is authoritative: a route absent from this
// map cannot be registered (see ScopedMux.Handle) and, if one were reached
// anyway, RequireScope denies it.
//
// Keys are "METHOD /pattern" using the same patterns passed to the mux.
//
// Two entries grant a read scope to an operation that creates work:
// POST /v1/ingest/runs and POST /v1/reports/export. That follows the build
// checklist, and realm.json declares no ingest:write or reports:write to
// promote them to. Revisit when it does.
var RouteScopes = map[string]string{
	// Stage 0a — diagnostics
	"GET /v1/admin/ping":   "admin:users",
	"GET /v1/service/ping": "ingest:read",

	// Stage 0b / 1 — tenders
	"GET /v1/tenders":                   "tenders:read",
	"GET /v1/tenders/{ocid}":            "tenders:read",
	"GET /v1/tenders/{ocid}/history":    "tenders:read",
	"GET /v1/tenders/{ocid}/documents":  "tenders:read",
	"GET /v1/tenders/{ocid}/awards":     "tenders:read",
	"GET /v1/tenders/{ocid}/contracts":  "tenders:read",
	"GET /v1/tenders/{ocid}/provenance": "tenders:read",

	// Stage 1 — awards, contracts, buyers, parties
	"GET /v1/awards":                                "awards:read",
	"GET /v1/awards/{awardId}":                      "awards:read",
	"GET /v1/awards/{awardId}/items":                "awards:read",
	"GET /v1/contracts":                             "contracts:read",
	"GET /v1/contracts/{contractId}":                "contracts:read",
	"GET /v1/contracts/{contractId}/implementation": "contracts:read",
	"GET /v1/buyers":                                "buyers:read",
	"GET /v1/buyers/{buyerId}":                      "buyers:read",
	"GET /v1/parties":                               "parties:read",
	"GET /v1/parties/{partyId}":                     "parties:read",

	// Stage 1 — documents and search
	"GET /v1/documents/{documentId}":          "documents:read",
	"GET /v1/documents/{documentId}/content":  "documents:read",
	"HEAD /v1/documents/{documentId}/content": "documents:read",
	"GET /v1/documents/{documentId}/text":     "documents:read",
	"GET /v1/search":                          "search:read",
	"GET /v1/search/suggest":                  "search:read",

	// Stage 2 — saved searches, alerts, workflows, reports
	"POST /v1/saved-searches":               "searches:write",
	"GET /v1/saved-searches":                "searches:read",
	"GET /v1/saved-searches/{searchId}":     "searches:read",
	"PATCH /v1/saved-searches/{searchId}":   "searches:write",
	"DELETE /v1/saved-searches/{searchId}":  "searches:write",
	"GET /v1/alerts":                        "alerts:read",
	"POST /v1/alerts":                       "alerts:write",
	"DELETE /v1/alerts/{alertId}":           "alerts:write",
	"POST /v1/workflows":                    "workflows:write",
	"GET /v1/workflows":                     "workflows:read",
	"GET /v1/workflows/{workflowId}":        "workflows:read",
	"PATCH /v1/workflows/{workflowId}":      "workflows:write",
	"POST /v1/workflows/{workflowId}/tasks": "workflows:write",
	"GET /v1/reports/pipeline":              "reports:read",
	"POST /v1/reports/export":               "reports:read",
	"GET /v1/reports/export/{jobId}":        "reports:read",

	// Stage 3 — ingest
	"GET /v1/ingest/runs":         "ingest:read",
	"POST /v1/ingest/runs":        "ingest:read",
	"GET /v1/ingest/runs/{runId}": "ingest:read",

	// Stage 4 — audit, privacy, admin, webhooks
	"GET /v1/audit/events":            "audit:read",
	"GET /v1/audit/events/{eventId}":  "audit:read",
	"GET /v1/privacy/me":              "privacy:read",
	"POST /v1/privacy/me/erase":       "privacy:read",
	"GET /v1/admin/users":             "admin:users",
	"POST /v1/admin/users":            "admin:users",
	"GET /v1/admin/users/{userId}":    "admin:users",
	"PATCH /v1/admin/users/{userId}":  "admin:users",
	"GET /v1/webhooks":                "admin:users",
	"POST /v1/webhooks":               "admin:users",
	"DELETE /v1/webhooks/{webhookId}": "admin:users",
}

// PublicRoutes are the routes served without authentication. Deny by default
// means public is a decision, made here, not an omission.
//
// /v1/whoami is NOT public: it requires a valid token and returns the caller's
// own claims. It carries no scope, so it is listed in AuthenticatedRoutes.
var PublicRoutes = map[string]bool{
	"GET /openapi.json": true,
	"GET /docs":         true,
	"GET /docs/{path}":  true,
	"GET /healthz":      true,
	"GET /readyz":       true,
}

// AuthenticatedRoutes require a verified token but no particular scope.
var AuthenticatedRoutes = map[string]bool{
	"GET /v1/whoami": true,
}

// RequireScope denies the request unless the verified claims carry scope s.
//
// Chain position: after Authenticate. If it runs without claims in the
// context the chain is misordered; it denies rather than assuming.
func RequireScope(s string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := auth.FromContext(r.Context())
			if !ok {
				// Authenticate did not run ahead of this handler. Fail closed.
				w.Header().Set("WWW-Authenticate", `Bearer realm="nfa"`)
				WriteProblem(w, r, http.StatusUnauthorized,
					"Unauthorized", "A bearer token is required.")
				return
			}
			if !claims.HasScope(s) {
				// The scope is not named in the response: which scope a route
				// wants is not something an unauthorised caller needs told.
				WriteProblem(w, r, http.StatusForbidden,
					"Forbidden", "This token does not permit that operation.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Deny is the fallback for a route with no scope decision. Reaching it is a
// wiring bug, so it denies.
func Deny() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteProblem(w, r, http.StatusForbidden,
			"Forbidden", "This token does not permit that operation.")
	})
}
