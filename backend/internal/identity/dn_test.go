package identity

import "testing"

func TestParseDN(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want Name
	}{
		{
			name: "common keycloak-ish cert",
			in:   "CN=alice,O=NFA,OU=Planners,C=ZA",
			want: Name{CommonName: "alice", Organization: "NFA",
				OrgUnits: []string{"Planners"}, Country: "ZA"},
		},
		{
			name: "with spaces after commas",
			in:   "CN=alice, O=NFA, C=ZA",
			want: Name{CommonName: "alice", Organization: "NFA", Country: "ZA"},
		},
		{
			name: "lowercase attributes",
			in:   "cn=alice,o=NFA,c=ZA",
			want: Name{CommonName: "alice", Organization: "NFA", Country: "ZA"},
		},
		{
			name: "escaped comma in value",
			in:   `CN=Doe\, John,O=NFA`,
			want: Name{CommonName: "Doe, John", Organization: "NFA"},
		},
		{
			name: "quoted value",
			in:   `CN="Doe, John",O=NFA`,
			want: Name{CommonName: "Doe, John", Organization: "NFA"},
		},
		{
			name: "multiple OUs preserve order",
			in:   "CN=x,OU=One,OU=Two,O=Y",
			want: Name{CommonName: "x", OrgUnits: []string{"One", "Two"}, Organization: "Y"},
		},
		{
			name: "semicolon separator",
			in:   "CN=alice;O=NFA",
			want: Name{CommonName: "alice", Organization: "NFA"},
		},
		{
			name: "unknown attribute ignored",
			in:   "CN=alice,UID=1000,O=NFA",
			want: Name{CommonName: "alice", Organization: "NFA"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseDN(tc.in)
			if err != nil {
				t.Fatalf("ParseDN(%q): %v", tc.in, err)
			}
			if got.CommonName != tc.want.CommonName ||
				got.Organization != tc.want.Organization ||
				got.Country != tc.want.Country {
				t.Errorf("ParseDN(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
			if len(got.OrgUnits) != len(tc.want.OrgUnits) {
				t.Fatalf("OrgUnits = %v, want %v", got.OrgUnits, tc.want.OrgUnits)
			}
			for i, ou := range tc.want.OrgUnits {
				if got.OrgUnits[i] != ou {
					t.Errorf("OrgUnits[%d] = %q, want %q", i, got.OrgUnits[i], ou)
				}
			}
		})
	}
}

func TestParseDNRejects(t *testing.T) {
	t.Parallel()
	bad := []string{
		"",                   // empty DN
		"CN=alice,O",         // RDN without '='
		`CN=alice\`,          // dangling escape
		"#04024869",          // hex-encoded DN
		"CN=#04024869,O=NFA", // hex-encoded value
	}
	for _, in := range bad {
		in := in
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseDN(in); err == nil {
				t.Errorf("ParseDN(%q) accepted, want error", in)
			}
		})
	}
}
