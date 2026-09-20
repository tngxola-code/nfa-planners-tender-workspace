# ADR 0008: JWT verification uses coreos/go-oidc

## Status
Accepted

## Context
Stage 0a verifies Keycloak-issued RS256 access tokens on every request. The
options were a hand-rolled verifier over `crypto/rsa`, or a maintained
library.

Hand-rolling puts three known failure classes in our own code: algorithm
confusion (`alg: none`, or HS256 signed with the RSA public key), `kid`
selection during key rotation, and JWKS refetch behaviour under an unknown
`kid`. These are the failures that produce a verifier which accepts forged
tokens while all tests pass.

## Decision
Use `github.com/coreos/go-oidc/v3/oidc` for token verification and JWKS
handling.

- `oidc.NewRemoteKeySet` caches keys by `kid` and single-flights a refetch
  when an unknown `kid` arrives, so a Keycloak key rotation does not become
  a refetch per request.
- `oidc.NewVerifier` is constructed from the issuer and key set directly.
  No discovery request is made at startup, so the backend's readiness does
  not depend on Keycloak being up.
- `SupportedSigningAlgs` is pinned to RS256, matching the realm's
  `defaultSignatureAlgorithm`. Any other `alg` is rejected.

## Audience
Keycloak access tokens carry `aud: ["account"]` by default, not the client
id. Rather than add an audience mapper to every client in `realm.json`, the
built-in audience check is disabled and `azp` (authorized party) is checked
against an explicit allow-list from `OIDC_ALLOWED_AZP`. An empty allow-list
is a configuration error and fails closed.

## Consequences
- Two new modules in `backend/go.mod`: `coreos/go-oidc/v3` (Apache-2.0) and
  its dependency `go-jose/go-jose/v4` (Apache-2.0).
- Scope enforcement is not here. This ADR covers authentication only;
  route-to-scope mapping is `feat/auth-scope`.

## Alternatives considered
- Hand-rolled over `crypto/rsa`. Rejected: see Context.
- `golang-jwt/jwt/v5`. Rejected: verification only, JWKS caching and
  rotation would still be ours to write.
