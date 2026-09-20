package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/tngxola-code/nfa-planners-tender-workspace/backend/internal/auth"
	"github.com/tngxola-code/nfa-planners-tender-workspace/backend/internal/diagnostics"
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
	for pattern, h := range diagnostics.Routes() {
		if err := apiMux.Handle(pattern, h); err != nil {
			log.Fatal(err)
		}
	}
	rootMux.Handle("/v1/", apiMux)

	handler := idMW(rootMux)

	// Timeouts are set explicitly: http.ListenAndServe leaves them at zero,
	// so a client that opens a connection and sends nothing holds it open
	// forever. ReadHeaderTimeout is the one that bounds that.
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	log.Printf("listening on %s (oidc %s)", addr, oidcStatus())
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
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
	allowed := splitComma(azp)

	// Test clients are admitted separately and announced. nfa-test enables
	// the ROPC grant so a script can mint a user token without a browser;
	// it must never be accepted by a deployed instance. Keeping it out of
	// OIDC_ALLOWED_AZP means a deployment cannot admit it by inheriting a
	// local value, and the log line below makes an accidental one visible.
	if testClients := os.Getenv("OIDC_ALLOWED_AZP_TEST_CLIENTS"); testClients != "" {
		extra := splitComma(testClients)
		log.Printf("WARNING: admitting test client(s) %v — this must not be set outside local development", extra)
		allowed = append(allowed, extra...)
	}
	v, err := auth.NewVerifier(auth.Config{
		IssuerURL:  issuer,
		JWKSURL:    os.Getenv("OIDC_JWKS_URL"),
		AllowedAZP: allowed,
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

func splitComma(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
