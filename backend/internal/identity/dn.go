package identity

import (
	"errors"
	"strings"
)

// Name is the subset of an RFC 2253 distinguished name this service cares
// about. Unknown attribute types are ignored, not errors: certificates in
// the wild carry attributes we do not model.
type Name struct {
	CommonName   string
	Organization string
	OrgUnits     []string
	Country      string
	Locality     string
	State        string

	// Raw is the input string, unmodified.
	Raw string
}

// ParseDN parses an RFC 2253 distinguished name as produced by Go's
// crypto/x509, OpenSSL, and most TLS terminators.
//
// The parser is deliberately strict — unbalanced quotes, a dangling
// escape, an RDN without '=', or a hex-encoded value all fail. Callers
// must treat a DN string as untrusted input: an untrusted party may have
// chosen the CN, and this parser does not by itself make the DN safe to
// use in shell commands, log lines, SQL, or HTTP headers.
func ParseDN(s string) (Name, error) {
	n := Name{Raw: s}
	s = strings.TrimSpace(s)
	if s == "" {
		return n, errors.New("identity: empty DN")
	}

	for _, rdn := range splitUnescaped(s, ',', ';') {
		rdn = strings.TrimSpace(rdn)
		if rdn == "" {
			continue
		}
		// Multi-valued RDN: split on unescaped '+'.
		for _, ava := range splitUnescaped(rdn, '+') {
			ava = strings.TrimSpace(ava)
			if ava == "" {
				continue
			}
			eq := indexUnescaped(ava, '=')
			if eq < 0 {
				return Name{}, errors.New("identity: RDN missing '=': " + ava)
			}
			attr := strings.ToUpper(strings.TrimSpace(ava[:eq]))
			val, err := unescape(strings.TrimSpace(ava[eq+1:]))
			if err != nil {
				return Name{}, err
			}
			switch attr {
			case "CN":
				n.CommonName = val
			case "O":
				n.Organization = val
			case "OU":
				n.OrgUnits = append(n.OrgUnits, val)
			case "C":
				n.Country = val
			case "L":
				n.Locality = val
			case "ST":
				n.State = val
			}
		}
	}
	return n, nil
}

// splitUnescaped splits s on any of the given separators that appears
// unescaped and outside a quoted string.
func splitUnescaped(s string, seps ...byte) []string {
	inQuote := false
	esc := false
	var parts []string
	var cur strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case esc:
			cur.WriteByte(c)
			esc = false
		case c == '\\':
			cur.WriteByte(c)
			esc = true
		case c == '"':
			inQuote = !inQuote
			cur.WriteByte(c)
		case !inQuote && containsByte(seps, c):
			parts = append(parts, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	parts = append(parts, cur.String())
	return parts
}

// indexUnescaped returns the index of the first unescaped, unquoted
// occurrence of c, or -1.
func indexUnescaped(s string, c byte) int {
	inQuote := false
	esc := false
	for i := 0; i < len(s); i++ {
		switch {
		case esc:
			esc = false
		case s[i] == '\\':
			esc = true
		case s[i] == '"':
			inQuote = !inQuote
		case !inQuote && s[i] == c:
			return i
		}
	}
	return -1
}

// unescape handles RFC 2253 backslash escaping and optional double-quoted
// values. Hex-encoded values ('#' prefix) are rejected: this service deals
// only in text DNs.
func unescape(s string) (string, error) {
	if strings.HasPrefix(s, "#") {
		return "", errors.New("identity: hex-encoded DN values are not supported")
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	var b strings.Builder
	esc := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if esc {
			b.WriteByte(c)
			esc = false
			continue
		}
		if c == '\\' {
			esc = true
			continue
		}
		b.WriteByte(c)
	}
	if esc {
		return "", errors.New("identity: dangling escape in DN")
	}
	return b.String(), nil
}

func containsByte(bs []byte, c byte) bool {
	for _, b := range bs {
		if b == c {
			return true
		}
	}
	return false
}
