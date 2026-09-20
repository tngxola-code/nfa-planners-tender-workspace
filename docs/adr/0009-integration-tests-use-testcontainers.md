# ADR 0009: Integration tests run against a containerised Keycloak

## Status
Accepted

## Context
Unit tests for the auth chain construct `auth.Claims` directly. That
verifies the middleware's logic but assumes the realm issues tokens
shaped the way the tests assume.

That assumption failed. The committed realm referenced Keycloak's
built-in `basic`, `profile`, `email` and `roles` client scopes, which a
full realm import does not create. Keycloak dropped the unresolvable
names silently, so every user token carried no `sub`, no
`preferred_username` and no `realm_access`. Every unit test passed. So
did `deploy/keycloak/verify.sh`, which asserted only that a token was
issued and carried the requested scope.

The defect surfaced only when a real token reached a real handler.

## Decision
Integration tests in `backend/tests/integration` start Keycloak with
`testcontainers-go`, importing the committed `deploy/keycloak/realm.json`,
and drive the same middleware chain `cmd/api` wires.

- The suite is behind the `integration` build tag, so `go test ./...`
  does not require Docker.
- `NFA_TEST_KEYCLOAK_URL` points the suite at an already-running
  Keycloak instead. Local convenience only; CI never sets it.
- Ryuk, testcontainers' reaper, is disabled. It requires the default
  `bridge` network, which is not reachable on every local Docker
  context. `TestMain` terminates its container explicitly.

## Consequences
- `testcontainers-go` (MIT) enters `go.mod`, with the Docker client
  library and its transitive dependencies. They are test-only: no
  production binary links them.
- The suite needs a working Docker daemon and roughly 40 seconds.
- A realm change that breaks token shape now fails a test rather than
  reaching production.

## Alternatives considered
- Test against the compose stack. Rejected: it cannot run in CI without
  the stack, and it tests whatever realm state happens to be present —
  the same class of false confidence this ADR exists to remove.
- Mock Keycloak. Rejected: a mock asserts our beliefs about Keycloak,
  which is precisely what was wrong.
