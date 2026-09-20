#!/usr/bin/env bash
# Exercises every endpoint the API currently serves, authenticated and not.
# Requires: the API on $API, Keycloak on $KC with the nfa realm imported,
# and OIDC_ALLOWED_AZP_TEST_CLIENTS=nfa-test on the API process.
set -uo pipefail

API="${API:-http://127.0.0.1:8080}"
KC="${KC:-http://localhost:8081}"
REALM="${REALM:-nfa}"
fail=0

user_token() {
  curl -s -X POST "$KC/realms/$REALM/protocol/openid-connect/token" \
    -d grant_type=password -d client_id=nfa-test \
    -d "username=$1@local" -d "password=$1" \
    -d "scope=openid $2" | jq -r '.access_token // empty'
}

service_token() {
  curl -s -X POST "$KC/realms/$REALM/protocol/openid-connect/token" \
    -d grant_type=client_credentials -d client_id=nfa-service \
    -d client_secret=local-service-secret-replace-in-prod \
    -d "scope=$1" | jq -r '.access_token // empty'
}

check() {
  local label="$1" want="$2" got="$3"
  if [ "$got" = "$want" ]; then
    printf '  ✓ %-46s %s\n' "$label" "$got"
  else
    printf '  ✗ %-46s got %s want %s\n' "$label" "$got" "$want"
    fail=1
  fi
}

hit() {  # hit <path> [token]
  if [ -n "${2:-}" ]; then
    curl -s -o /dev/null -w '%{http_code}' "$API$1" -H "Authorization: Bearer $2"
  else
    curl -s -o /dev/null -w '%{http_code}' "$API$1"
  fi
}

echo "public"
check "GET /healthz (no auth)"            200 "$(hit /healthz)"

echo "unauthenticated must be refused"
check "GET /v1/whoami no token"           401 "$(hit /v1/whoami)"
check "GET /v1/admin/ping no token"       401 "$(hit /v1/admin/ping)"
check "GET /v1/service/ping no token"     401 "$(hit /v1/service/ping)"
check "GET /v1/whoami garbage token"      401 "$(hit /v1/whoami not.a.jwt)"

echo "whoami: any authenticated caller"
for u in planner analyst admin officer; do
  check "GET /v1/whoami as $u"            200 "$(hit /v1/whoami "$(user_token "$u" '')")"
done

echo "admin:users"
check "GET /v1/admin/ping as planner"     403 "$(hit /v1/admin/ping "$(user_token planner admin:users)")"
check "GET /v1/admin/ping as analyst"     403 "$(hit /v1/admin/ping "$(user_token analyst admin:users)")"
check "GET /v1/admin/ping as officer"     403 "$(hit /v1/admin/ping "$(user_token officer admin:users)")"
check "GET /v1/admin/ping as admin"       200 "$(hit /v1/admin/ping "$(user_token admin admin:users)")"

echo "ingest:read"
check "GET /v1/service/ping as planner"   403 "$(hit /v1/service/ping "$(user_token planner ingest:read)")"
check "GET /v1/service/ping as admin"     200 "$(hit /v1/service/ping "$(user_token admin ingest:read)")"
check "GET /v1/service/ping as nfa-service" 200 "$(hit /v1/service/ping "$(service_token ingest:read)")"

echo "routes that do not exist"
check "GET /v1/tenders (not built)"       404 "$(hit /v1/tenders "$(user_token planner tenders:read)")"

echo
if [ "$fail" -eq 0 ]; then echo "all checks passed"; else echo "FAILURES"; fi
exit $fail
