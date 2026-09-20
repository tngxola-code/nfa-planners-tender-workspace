package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/tngxola-code/nfa-planners-tender-workspace/backend/internal/auth"
	"github.com/tngxola-code/nfa-planners-tender-workspace/backend/internal/httpx"
	"github.com/tngxola-code/nfa-planners-tender-workspace/backend/internal/identity"
)

func main() {
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		// Bind to loopback: the Caddyfile reverse-proxies from 127.0.0.1.
		// Do not change this to a public bind without also removing the
		// TrustedProxies check below, because a public bind would let
		// anyone forge the X-Client-Cert-* headers.
		addr = "127.0.0.1:8080"
	}

	// mTLS identity middleware runs first: it never rejects, it just
	// attaches an identity (possibly anonymous) to the request context.
	idMW, err := buildIdentityMiddleware()
	if err != nil {
		log.Fatalf("identity: %v", err)
	}

	// OIDC is optional: unset OIDC_ISSUER disables bearer-token auth and
	// leaves only mTLS identity.
	oidcMW, err := buildOIDCMiddleware()
	if err != nil {
		log.Fatalf("auth: %v", err)
	}

	// Public routes: no OIDC, no mTLS. Health probes and metrics must be
	// reachable without a bearer token.
	rootMux := http.NewServeMux()
	rootMux.HandleFunc("/healthz", handleHealthz)

	// Protected routes: OIDC bearer-token enforcement. mTLS identity is
	// still attached (identity runs at the outer layer) but the OIDC
	// verifier rejects unauthenticated requests before they reach a
	// handler. Whether an mTLS identity alone is sufficient is decided by
	// subsequent authorization (feat/auth-scope).
	// Every route is registered through ScopedMux, which refuses a pattern
	// with no recorded scope decision. An unprotected route is therefore a
	// startup failure, not something a reviewer has to notice.
	apiMux := httpx.NewScopedMux(oidcMW)
	if err := apiMux.Handle("GET /v1/whoami", http.HandlerFunc(handleWhoami)); err != nil {
		log.Fatal(err)
	}
	rootMux.Handle("/v1/", apiMux)

	handler := idMW(rootMux)

	log.Printf("listening on %s (oidc %s)", addr, oidcStatus())
	if err := http.ListenAndServe(addr, handler); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func buildIdentityMiddleware() (func(http.Handler) http.Handler, error) {
	trustedProxies := os.Getenv("IDENTITY_TRUSTED_PROXIES")
	if trustedProxies == "" {
		trustedProxies = "127.0.0.1/32,::1/128"
	}
	cidrs, err := identity.ParseCIDRs(trustedProxies)
	if err != nil {
		return nil, err
	}
	return identity.Middleware(identity.Config{
		TrustedProxies: cidrs,
		Logf:           log.Printf,
	})
}

// buildOIDCMiddleware returns the OIDC authenticator. OIDC_ISSUER is
// required: there is no unauthenticated mode.
func buildOIDCMiddleware() (func(http.Handler) http.Handler, error) {
	issuer := os.Getenv("OIDC_ISSUER")
	if issuer == "" {
		// Fail closed. A passthrough here would serve every /v1/ route
		// unauthenticated, and the absence of a variable is not consent.
		return nil, errors.New("OIDC_ISSUER must be set; refusing to serve unauthenticated")
	}
	azp := os.Getenv("OIDC_ALLOWED_AZP")
	if azp == "" {
		return nil, errors.New("OIDC_ALLOWED_AZP must be set when OIDC_ISSUER is set")
	}
	v, err := auth.NewVerifier(auth.Config{
		IssuerURL:  issuer,
		JWKSURL:    os.Getenv("OIDC_JWKS_URL"),
		AllowedAZP: splitComma(azp),
	})
	if err != nil {
		return nil, err
	}
	return httpx.Authenticator(v), nil
}

func oidcStatus() string {
	if os.Getenv("OIDC_ISSUER") == "" {
		return "disabled"
	}
	return "enabled"
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func handleWhoami(w http.ResponseWriter, r *http.Request) {
	id, _ := identity.FromContext(r.Context())
	claims, _ := auth.FromContext(r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"identity": map[string]any{
			"source":      string(id.Source),
			"subject":     id.Subject,
			"common_name": id.CommonName,
			"org":         id.Org,
		},
		"oidc": map[string]any{
			"subject":  claimSubject(claims),
			"username": claimUsername(claims),
		},
	})
}

func splitComma(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func claimSubject(c *auth.Claims) string {
	if c == nil {
		return ""
	}
	return c.Subject
}

func claimUsername(c *auth.Claims) string {
	if c == nil {
		return ""
	}
	return c.Username
}
