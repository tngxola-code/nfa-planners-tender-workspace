package httpx

import (
	"errors"
	"net/http"

	"github.com/tngxola-code/nfa-planners-tender-workspace/backend/internal/auth"
)

// Authenticator returns middleware that requires a valid bearer token and
// stashes the verified claims on the request context via auth.WithClaims.
//
// Status mapping:
//   - missing/malformed Authorization header        -> 401
//   - ErrUnauthenticated (bad signature/iss/exp/azp) -> 401
//   - ErrUnavailable (JWKS unreachable)              -> 503
//   - anything else                                  -> 503
func Authenticator(v *auth.Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := auth.BearerToken(r.Header.Get("Authorization"))
			if !ok {
				w.Header().Set("WWW-Authenticate", `Bearer realm="nfa"`)
				WriteProblem(w, r, http.StatusUnauthorized,
					"Unauthorized", "A bearer token is required.")
				return
			}

			claims, err := v.Verify(r.Context(), raw)
			if err != nil {
				switch {
				case errors.Is(err, auth.ErrUnauthenticated):
					w.Header().Set("WWW-Authenticate",
						`Bearer realm="nfa", error="invalid_token"`)
					WriteProblem(w, r, http.StatusUnauthorized,
						"Unauthorized", "The bearer token is not valid.")
				case errors.Is(err, auth.ErrUnavailable):
					// The caller's token may be fine; the keyset is down.
					WriteProblem(w, r, http.StatusServiceUnavailable,
						"Service Unavailable", "The token could not be verified.")
				default:
					WriteProblem(w, r, http.StatusServiceUnavailable,
						"Service Unavailable", "The token could not be verified.")
				}
				return
			}

			next.ServeHTTP(w, r.WithContext(auth.WithClaims(r.Context(), claims)))
		})
	}
}
