package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

var (
	// ErrUnauthenticated means the token was present but did not verify.
	// Callers should respond 401.
	ErrUnauthenticated = errors.New("auth: unauthenticated")

	// ErrMisconfigured means the verifier cannot operate. Callers should
	// refuse to start, or respond 503 if it slips through.
	ErrMisconfigured = errors.New("auth: misconfigured")

	// ErrUnavailable means verification could not be attempted because a
	// downstream dependency (JWKS) was unreachable. Callers should respond
	// 503, not 401: the token may be fine.
	ErrUnavailable = errors.New("auth: unavailable")
)

// jwksProbeTTL bounds how often we re-check JWKS reachability after a
// verification failure. Without it, a flood of invalid tokens becomes a
// flood of JWKS probes.
const jwksProbeTTL = 30 * time.Second

// Config configures the OIDC verifier.
type Config struct {
	IssuerURL  string
	JWKSURL    string
	AllowedAZP []string
	HTTPClient *http.Client
}

// Claims is the subset of Keycloak access-token claims the service uses.
type Claims struct {
	Subject    string
	Username   string
	Email      string
	AZP        string
	Scopes     []string
	RealmRoles []string
	ExpiresAt  time.Time
}

func (c *Claims) HasScope(scope string) bool {
	for _, s := range c.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

func (c *Claims) HasRealmRole(role string) bool {
	for _, r := range c.RealmRoles {
		if r == role {
			return true
		}
	}
	return false
}

type realmClaims struct {
	PreferredUsername string `json:"preferred_username"`
	Email             string `json:"email"`
	AZP               string `json:"azp"`
	Scope             string `json:"scope"`
	RealmAccess       struct {
		Roles []string `json:"roles"`
	} `json:"realm_access"`
}

// Verifier validates bearer tokens against a Keycloak realm.
type Verifier struct {
	cfg        Config
	verifier   *oidc.IDTokenVerifier
	allowAZP   map[string]struct{}
	httpClient *http.Client
	jwksURL    string

	// probeMu guards every field below. It is NEVER held across network
	// I/O: probeJWKS does single-flight coordination so concurrent callers
	// share one in-flight probe rather than serializing on a lock.
	probeMu   sync.Mutex
	probeOK   bool
	probeAt   time.Time
	probing   bool
	probeDone chan struct{}
}

// NewVerifier builds a Verifier. No network I/O at construction: the
// underlying remote key set fetches JWKS lazily on first use.
func NewVerifier(cfg Config) (*Verifier, error) {
	if cfg.IssuerURL == "" {
		return nil, fmt.Errorf("%w: IssuerURL is required", ErrMisconfigured)
	}
	cfg.IssuerURL = strings.TrimRight(cfg.IssuerURL, "/")
	if cfg.JWKSURL == "" {
		// Keycloak convention.
		cfg.JWKSURL = cfg.IssuerURL + "/protocol/openid-connect/certs"
	}
	if len(cfg.AllowedAZP) == 0 {
		return nil, fmt.Errorf("%w: AllowedAZP must not be empty", ErrMisconfigured)
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	ctx := oidc.ClientContext(context.Background(), httpClient)
	keySet := oidc.NewRemoteKeySet(ctx, cfg.JWKSURL)

	allow := make(map[string]struct{}, len(cfg.AllowedAZP))
	for _, a := range cfg.AllowedAZP {
		allow[a] = struct{}{}
	}

	return &Verifier{
		cfg:        cfg,
		httpClient: httpClient,
		jwksURL:    cfg.JWKSURL,
		verifier: oidc.NewVerifier(cfg.IssuerURL, keySet, &oidc.Config{
			SkipClientIDCheck:    true,
			SkipIssuerCheck:      false,
			SkipExpiryCheck:      false,
			SupportedSigningAlgs: []string{"RS256", "RS384", "RS512"},
		}),
		allowAZP: allow,
	}, nil
}

func (v *Verifier) IssuerURL() string { return v.cfg.IssuerURL }

// checkSupportedAlg parses the JOSE header and rejects anything outside the
// RS* family. Runs before coreos/go-oidc so the error message is ours, and so
// an attacker-supplied alg never reaches go-jose.
func checkSupportedAlg(raw string) error {
	parts := strings.SplitN(raw, ".", 3)
	if len(parts) != 3 {
		return errors.New("malformed token")
	}
	hdrBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return errors.New("malformed header")
	}
	var hdr struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(hdrBytes, &hdr); err != nil {
		return errors.New("malformed header json")
	}
	switch hdr.Alg {
	case "RS256", "RS384", "RS512":
		return nil
	default:
		return fmt.Errorf("unsupported signing algorithm %q", hdr.Alg)
	}
}

// probeJWKS reports whether the JWKS endpoint is reachable right now.
//
// Concurrency: callers either observe a recent cached result, wait on a
// single in-flight probe, or become the prober themselves. The network
// request happens outside probeMu, so no goroutine is ever blocked on
// another goroutine's HTTP I/O.
func (v *Verifier) probeJWKS(ctx context.Context) error {
	v.probeMu.Lock()

	// Fast path: recent result still fresh.
	if !v.probeAt.IsZero() && time.Since(v.probeAt) < jwksProbeTTL {
		ok := v.probeOK
		v.probeMu.Unlock()
		if ok {
			return nil
		}
		return errors.New("jwks recently observed unreachable")
	}

	// Someone else is probing: wait for their result without holding the
	// lock, and without racing the context.
	if v.probing {
		done := v.probeDone
		v.probeMu.Unlock()
		select {
		case <-done:
			v.probeMu.Lock()
			ok := v.probeOK
			v.probeMu.Unlock()
			if ok {
				return nil
			}
			return errors.New("jwks observed unreachable")
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// We are the prober. Publish a channel for waiters, drop the lock,
	// then do the HTTP request.
	v.probing = true
	done := make(chan struct{})
	v.probeDone = done
	v.probeMu.Unlock()

	err := v.doProbe(ctx)

	v.probeMu.Lock()
	v.probeOK = err == nil
	v.probeAt = time.Now()
	v.probing = false
	close(done)
	v.probeMu.Unlock()

	return err
}

// doProbe performs the actual HTTP GET. It must be called without probeMu
// held.
func (v *Verifier) doProbe(ctx context.Context) error {
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return err
	}
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks returned status %d", resp.StatusCode)
	}
	return nil
}

// Verify checks alg, signature, issuer, and expiry, then enforces the AZP
// allow-list. Errors are classified:
//   - bad token            -> wraps ErrUnauthenticated (401)
//   - JWKS unreachable     -> wraps ErrUnavailable     (503)
func (v *Verifier) Verify(ctx context.Context, raw string) (*Claims, error) {
	if err := checkSupportedAlg(raw); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnauthenticated, err)
	}

	tok, err := v.verifier.Verify(ctx, raw)
	if err != nil {
		// coreos/go-oidc does not expose a typed error for "couldn't reach
		// the keyset". Probe the endpoint to distinguish bad-token from
		// dependency-down.
		if probeErr := v.probeJWKS(ctx); probeErr != nil {
			return nil, fmt.Errorf("%w: jwks unreachable: %v (verify err: %v)",
				ErrUnavailable, probeErr, err)
		}
		return nil, fmt.Errorf("%w: %v", ErrUnauthenticated, err)
	}

	var rc realmClaims
	if err := tok.Claims(&rc); err != nil {
		return nil, fmt.Errorf("%w: claims decode: %v", ErrUnauthenticated, err)
	}

	if _, ok := v.allowAZP[rc.AZP]; !ok {
		return nil, fmt.Errorf("%w: azp %q not allowed", ErrUnauthenticated, rc.AZP)
	}

	return &Claims{
		Subject:    tok.Subject,
		Username:   rc.PreferredUsername,
		Email:      rc.Email,
		AZP:        rc.AZP,
		Scopes:     strings.Fields(rc.Scope),
		RealmRoles: rc.RealmAccess.Roles,
		ExpiresAt:  tok.Expiry,
	}, nil
}

// BearerToken extracts the token from an Authorization header. The scheme
// match is case-insensitive per RFC 7235; the token is returned verbatim.
func BearerToken(h string) (string, bool) {
	const prefix = "bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	tok := strings.TrimSpace(h[len(prefix):])
	return tok, tok != ""
}
