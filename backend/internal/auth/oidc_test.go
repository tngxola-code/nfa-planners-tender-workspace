package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

const testRealmPath = "/realms/nfa"

// fixture owns an httptest JWKS server and an RSA keypair. It is read-only
// after construction, so parallel subtests may share it safely.
type fixture struct {
	srv *httptest.Server
	key *rsa.PrivateKey
	kid string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{key: k, kid: "test-kid"}

	mux := http.NewServeMux()
	mux.HandleFunc(testRealmPath+"/protocol/openid-connect/certs",
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"keys": []map[string]any{{
					"kty": "RSA",
					"kid": f.kid,
					"use": "sig",
					"alg": "RS256",
					"n":   base64.RawURLEncoding.EncodeToString(k.N.Bytes()),
					"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(k.E)).Bytes()),
				}},
			})
		})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fixture) issuer() string { return f.srv.URL + testRealmPath }

func (f *fixture) verifier(t *testing.T) *Verifier {
	t.Helper()
	v, err := NewVerifier(Config{
		IssuerURL:  f.issuer(),
		AllowedAZP: []string{"nfa-console"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// baseClaims returns the minimum viable Keycloak-shaped claim set. The
// return type is map[string]any because go-jose's jwt package takes an
// arbitrary JSON-marshalable value via Builder.Claims(any).
func (f *fixture) baseClaims() map[string]any {
	return map[string]any{
		"iss": f.issuer(),
		"sub": "user-1",
		"azp": "nfa-console",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
}

func (f *fixture) signRS256(t *testing.T, claims map[string]any) string {
	t.Helper()
	signer, err := jose.NewSigner(
		jose.SigningKey{
			Algorithm: jose.RS256,
			Key:       jose.JSONWebKey{Key: f.key, KeyID: f.kid},
		},
		(&jose.SignerOptions{}).WithType("JWT"),
	)
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}
	s, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	return s
}

// signHS256 produces an HS256 token. RFC 7518 §3.2 requires an HMAC key at
// least as long as the hash output (32 bytes for SHA-256); go-jose rejects
// shorter keys at NewSigner, so callers must pass a compliant key.
func signHS256(t *testing.T, key []byte, claims map[string]any) string {
	t.Helper()
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.HS256, Key: key},
		(&jose.SignerOptions{}).WithType("JWT"),
	)
	if err != nil {
		t.Fatalf("NewSigner HS256: %v (key length %d)", err, len(key))
	}
	s, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		t.Fatalf("Serialize HS256: %v", err)
	}
	return s
}

func TestVerifyAcceptsAValidToken(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	v := f.verifier(t)
	tok := f.signRS256(t, f.baseClaims())

	claims, err := v.Verify(context.Background(), tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.Subject != "user-1" {
		t.Errorf("Subject = %q, want user-1", claims.Subject)
	}
	if claims.AZP != "nfa-console" {
		t.Errorf("AZP = %q, want nfa-console", claims.AZP)
	}
}

// TestVerifyRejects covers the security-critical rejection paths. The
// hs256_confusion case is a regression test against algorithm confusion:
// a token signed with a symmetric algorithm must never be accepted.
func TestVerifyRejects(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	// RFC 7518 §3.2: HS256 needs >= 32 bytes.
	hmacKey := []byte("0123456789abcdef0123456789abcdef")

	cases := []struct {
		name string
		tok  func(t *testing.T, f *fixture) string
	}{
		{
			name: "expired",
			tok: func(t *testing.T, f *fixture) string {
				c := f.baseClaims()
				c["exp"] = time.Now().Add(-time.Hour).Unix()
				return f.signRS256(t, c)
			},
		},
		{
			name: "wrong_issuer",
			tok: func(t *testing.T, f *fixture) string {
				c := f.baseClaims()
				c["iss"] = "https://evil.example.com/realms/nfa"
				return f.signRS256(t, c)
			},
		},
		{
			name: "unknown_azp",
			tok: func(t *testing.T, f *fixture) string {
				c := f.baseClaims()
				c["azp"] = "other-client"
				return f.signRS256(t, c)
			},
		},
		{
			name: "missing_azp",
			tok: func(t *testing.T, f *fixture) string {
				c := f.baseClaims()
				delete(c, "azp")
				return f.signRS256(t, c)
			},
		},
		{
			name: "hs256_confusion",
			tok: func(_ *testing.T, f *fixture) string {
				return signHS256(t, hmacKey, f.baseClaims())
			},
		},
		{
			name: "alg_none",
			tok: func(_ *testing.T, f *fixture) string {
				// Hand-built alg=none token: header.payload. (trailing dot,
				// empty signature).
				body := base64.RawURLEncoding.EncodeToString(
					[]byte(`{"iss":"` + f.issuer() + `","sub":"u"}`))
				return "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0." + body + "."
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := f.verifier(t).Verify(context.Background(), tc.tok(t, f))
			if err == nil {
				t.Fatal("expected Verify to reject, got nil")
			}
			if !errors.Is(err, ErrUnauthenticated) {
				t.Errorf("err = %v, want ErrUnauthenticated", err)
			}
		})
	}
}

func TestVerifyRejectsGarbage(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	v := f.verifier(t)

	for _, raw := range []string{"", "not-a-jwt", "a.b", "a.b.c.d", "...."} {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			_, err := v.Verify(context.Background(), raw)
			if !errors.Is(err, ErrUnauthenticated) {
				t.Errorf("Verify(%q) err = %v, want ErrUnauthenticated", raw, err)
			}
		})
	}
}

// FuzzVerifyNoPanicOnGarbage asserts that no input can panic the verifier.
// Failing inputs are written to testdata/fuzz and become regression cases.
func FuzzVerifyNoPanicOnGarbage(f *testing.F) {
	f.Add("")
	f.Add("a.b.c")
	f.Add("eyJhbGciOiJSUzI1NiJ9.e30.")
	f.Add("eyJhbGciOiJub25lIn0.e30.")
	f.Add(strings.Repeat(".", 4096))

	v, err := NewVerifier(Config{
		IssuerURL:  "https://kc.invalid/realms/nfa",
		JWKSURL:    "http://127.0.0.1:1/certs",
		AllowedAZP: []string{"nfa-console"},
	})
	if err != nil {
		f.Fatalf("NewVerifier: %v", err)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		_, err := v.Verify(context.Background(), raw)
		if err == nil {
			t.Fatalf("garbage input produced no error: %q", raw)
		}
	})
}

func TestNewVerifierRequiresAnAllowList(t *testing.T) {
	t.Parallel()
	_, err := NewVerifier(Config{
		IssuerURL: "https://kc.example.com/realms/nfa",
	})
	if err == nil {
		t.Fatal("expected error for empty allow-list")
	}
	if !errors.Is(err, ErrMisconfigured) {
		t.Errorf("err = %v, want ErrMisconfigured", err)
	}
}

func TestBearerToken(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"Bearer abc", "abc", true},
		{"bearer abc", "abc", true},
		{"BEARER abc", "abc", true},
		{"BeArEr abc", "abc", true},
		{"Bearer   abc  ", "abc", true},
		{"Bearer ", "", false},
		{"Bearer", "", false},
		{"Basic abc", "", false},
		{"", "", false},
		{"abc", "", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got, ok := BearerToken(tc.in)
			if got != tc.want || ok != tc.ok {
				t.Errorf("BearerToken(%q) = %q,%v want %q,%v",
					tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestClaims_HasScope(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		scopes []string
		probe  string
		want   bool
	}{
		{"present", []string{"openid", "tender:read"}, "tender:read", true},
		{"absent", []string{"openid"}, "tender:write", false},
		{"empty scopes", nil, "anything", false},
		{"exact match only", []string{"tender:reading"}, "tender:read", false},
		{"case sensitive", []string{"OpenID"}, "openid", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := &Claims{Scopes: tc.scopes}
			if got := c.HasScope(tc.probe); got != tc.want {
				t.Errorf("HasScope(%q) = %v, want %v", tc.probe, got, tc.want)
			}
		})
	}
}

func TestClaims_HasRealmRole(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		roles []string
		probe string
		want  bool
	}{
		{"present", []string{"nfa-admin", "nfa-viewer"}, "nfa-admin", true},
		{"absent", []string{"nfa-viewer"}, "nfa-admin", false},
		{"empty roles", nil, "anything", false},
		{"case sensitive", []string{"NFA-Admin"}, "nfa-admin", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := &Claims{RealmRoles: tc.roles}
			if got := c.HasRealmRole(tc.probe); got != tc.want {
				t.Errorf("HasRealmRole(%q) = %v, want %v", tc.probe, got, tc.want)
			}
		})
	}
}

func TestVerifyReturnsErrUnavailableWhenJWKSDown(t *testing.T) {
	t.Parallel()
	v, err := NewVerifier(Config{
		IssuerURL:  "https://kc.invalid/realms/nfa",
		JWKSURL:    "http://127.0.0.1:1/certs",
		AllowedAZP: []string{"nfa-console"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Well-formed RS256 header so checkSupportedAlg passes; verification
	// then attempts a JWKS fetch that fails.
	raw := "eyJhbGciOiJSUzI1NiIsImtpZCI6IngifQ.e30.x"
	_, err = v.Verify(context.Background(), raw)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}
