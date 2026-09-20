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
		// Do not change this to :8080 without also removing the
		// TrustedProxies check below, because a public bind would let
		// anyone forge the X-Client-Cert-* headers.
		addr = "127.0.0.1:8080"
	}

	trustedProxies := os.Getenv("IDENTITY_TRUSTED_PROXIES")
	if trustedProxies == "" {
		trustedProxies = "127.0.0.1/32,::1/128"
	}
	cidrs, err := identity.ParseCIDRs(trustedProxies)
	if err != nil {
		log.Fatalf("identity: %v", err)
	}
	idMW, err := identity.Middleware(identity.Config{
		TrustedProxies: cidrs,
		Logf:           log.Printf,
	})
	if err != nil {
		log.Fatalf("identity: %v", err)
	}

	// OIDC is optional: unset OIDC_ISSUER disables bearer-token auth and
	// leaves only mTLS identity. Useful during bring-up before Keycloak
	// is wired.
	var oidcMW func(http.Handler) http.Handler
	if issuer := os.Getenv("OIDC_ISSUER"); issuer != "" {
		azp := os.Getenv("OIDC_ALLOWED_AZP")
		if azp == "" {
			log.Fatal("OIDC_ALLOWED_AZP must be set when OIDC_ISSUER is set")
		}
		v, err := auth.NewVerifier(auth.Config{
			IssuerURL:  issuer,
			JWKSURL:    os.Getenv("OIDC_JWKS_URL"),
			AllowedAZP: splitComma(azp),
		})
		if err != nil {
			log.Fatalf("auth: %v", err)
		}
		oidcMW = httpx.Authenticator(v)
	} else {
		oidcMW = func(next http.Handler) http.Handler { return next }
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/v1/whoami", func(w http.ResponseWriter, r *http.Request) {
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
	})

	handler := idMW(oidcMW(mux))

	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
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
