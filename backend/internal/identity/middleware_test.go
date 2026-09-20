package identity

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func mustCIDRs(t *testing.T, s string) []*net.IPNet {
	t.Helper()
	n, err := ParseCIDRs(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func newMW(t *testing.T, cidrs string) func(http.Handler) http.Handler {
	t.Helper()
	mw, err := Middleware(Config{TrustedProxies: mustCIDRs(t, cidrs)})
	if err != nil {
		t.Fatal(err)
	}
	return mw
}

func doRequest(t *testing.T, mw func(http.Handler) http.Handler,
	remote string, hdrs map[string][]string) Identity {
	t.Helper()
	var captured Identity
	h := mw(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		id, ok := FromContext(r.Context())
		if !ok {
			t.Fatal("FromContext returned no identity; middleware must always attach one")
		}
		captured = id
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remote
	for k, vs := range hdrs {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	h.ServeHTTP(httptest.NewRecorder(), req)
	return captured
}

func TestMiddleware_NoHeaders(t *testing.T) {
	t.Parallel()
	mw := newMW(t, "127.0.0.1/32")
	got := doRequest(t, mw, "127.0.0.1:1234", nil)
	if !got.IsAnonymous() {
		t.Errorf("want anonymous, got %+v", got)
	}
}

func TestMiddleware_TrustedPeerWithIdentity(t *testing.T) {
	t.Parallel()
	mw := newMW(t, "127.0.0.1/32")
	got := doRequest(t, mw, "127.0.0.1:1234", map[string][]string{
		HeaderSubject:     {"CN=alice,O=NFA,C=ZA"},
		HeaderSerial:      {"0a1b2c"},
		HeaderFingerprint: {"deadbeef"},
	})
	if got.IsAnonymous() {
		t.Fatal("want identity, got anonymous")
	}
	if got.Source != SourceMTLS {
		t.Errorf("Source = %q, want %q", got.Source, SourceMTLS)
	}
	if got.CommonName != "alice" || got.Org != "NFA" || got.Country != "ZA" {
		t.Errorf("parsed fields wrong: %+v", got)
	}
	if got.Serial != "0a1b2c" || got.Fingerprint != "deadbeef" {
		t.Errorf("serial/fingerprint wrong: %+v", got)
	}
}

func TestMiddleware_UntrustedPeerHeadersAreIgnored(t *testing.T) {
	t.Parallel()
	mw := newMW(t, "127.0.0.1/32")
	got := doRequest(t, mw, "203.0.113.5:9999", map[string][]string{
		HeaderSubject: {"CN=admin,O=NFA"},
	})
	if !got.IsAnonymous() {
		t.Fatalf("untrusted peer's headers were honored: %+v", got)
	}
}

func TestMiddleware_DuplicateSubjectIsRejected(t *testing.T) {
	t.Parallel()
	mw := newMW(t, "127.0.0.1/32")
	got := doRequest(t, mw, "127.0.0.1:1234", map[string][]string{
		HeaderSubject: {"CN=alice,O=NFA", "CN=admin,O=NFA"},
	})
	if !got.IsAnonymous() {
		t.Fatalf("duplicate subject headers were honored: %+v", got)
	}
}

func TestMiddleware_EmptySubjectIsAnonymous(t *testing.T) {
	t.Parallel()
	mw := newMW(t, "127.0.0.1/32")
	got := doRequest(t, mw, "127.0.0.1:1234", map[string][]string{
		HeaderSubject: {""},
	})
	if !got.IsAnonymous() {
		t.Fatalf("empty subject produced an identity: %+v", got)
	}
}

func TestMiddleware_UnparseableDNStillReportsMTLS(t *testing.T) {
	t.Parallel()
	mw := newMW(t, "127.0.0.1/32")
	got := doRequest(t, mw, "127.0.0.1:1234", map[string][]string{
		HeaderSubject: {"not a DN"},
	})
	if got.IsAnonymous() {
		t.Fatal("unparseable DN should still yield SourceMTLS with raw Subject")
	}
	if got.Subject != "not a DN" {
		t.Errorf("Subject = %q, want raw preserved", got.Subject)
	}
	if got.CommonName != "" {
		t.Errorf("CommonName = %q, want empty for unparseable DN", got.CommonName)
	}
}

func TestMiddleware_RejectsEmptyTrustedProxies(t *testing.T) {
	t.Parallel()
	if _, err := Middleware(Config{}); err == nil {
		t.Fatal("empty TrustedProxies should be a configuration error")
	}
}

func TestParseCIDRs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"127.0.0.1/32", false},
		{"127.0.0.1", false},
		{"10.0.0.0/8,::1/128", false},
		{"192.168.0.0/16 , 127.0.0.1/32", false},
		{"", true},
		{",,", true},
		{"not-an-ip", true},
		{"10.0.0.0/33", true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			_, err := ParseCIDRs(tc.in)
			if (err != nil) != tc.wantErr {
				t.Errorf("ParseCIDRs(%q) err = %v, wantErr = %v", tc.in, err, tc.wantErr)
			}
		})
	}
}

// In production the trusted proxy is Caddy on a container network, not
// loopback. This exercises the rejection path with a realistic range: a
// peer inside 10.0.0.0/8 is trusted, and loopback — which is trusted
// everywhere else in this suite — is not.
func TestMiddleware_TrustsOnlyTheConfiguredRange(t *testing.T) {
	mw := newMW(t, "10.0.0.0/8")
	hdrs := map[string][]string{
		HeaderSubject: {"CN=worker,O=NFA"},
	}

	trusted := doRequest(t, mw, "10.1.2.3:44321", hdrs)
	if trusted.Subject != "CN=worker,O=NFA" {
		t.Errorf("peer inside the trusted range: subject = %q, want the forwarded DN",
			trusted.Subject)
	}

	// Loopback is outside 10.0.0.0/8 here. Its headers must be ignored,
	// or any process on the host could assert an identity.
	untrusted := doRequest(t, mw, "127.0.0.1:44321", hdrs)
	if untrusted.Subject != "" {
		t.Errorf("peer outside the trusted range asserted an identity: %q",
			untrusted.Subject)
	}
}
