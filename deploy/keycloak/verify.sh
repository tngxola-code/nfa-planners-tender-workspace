#!/usr/bin/env bash
# Asserts the NFA realm is loaded and the scope model behaves.
# Exits 0 on success, 1 on any failure.

set -euo pipefail

BASE="${KEYCLOAK_URL:-http://localhost:8081}"
REALM="${KEYCLOAK_REALM:-nfa}"
FAIL=0

log()  { printf '  %s\n' "$*"; }
ok()   { printf '  ✓ %s\n' "$*"; }
fail() { printf '  ✗ %s\n' "$*"; FAIL=1; }

require() {
  command -v "$1" >/dev/null || { echo "missing: $1"; exit 2; }
}
require curl
require jq

echo "verifying realm ${REALM} at ${BASE}"

# ---------------------------------------------------------------- discovery
DISCO="$(curl -sf "${BASE}/realms/${REALM}/.well-known/openid-configuration" || true)"
if [ -z "$DISCO" ]; then
  fail "discovery document not reachable"
  exit 1
fi
ok "discovery document reachable"

for field in authorization_endpoint token_endpoint jwks_uri issuer; do
  if echo "$DISCO" | jq -e ".${field}" > /dev/null; then
    ok "discovery has ${field}"
  else
    fail "discovery missing ${field}"
  fi
done

# ---------------------------------------------------------------- issue tokens
issue_token() {
  local user="$1" pass="$2" scope="$3"
  # nfa-test is a local-only public client with direct access grants enabled
  # so this script can issue tokens without a browser. Never enable direct
  # access grants on nfa-console or nfa-mobile.
  # curl without -f: a failed grant yields an empty body, not exit 22.
  curl -s -X POST "${BASE}/realms/${REALM}/protocol/openid-connect/token" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "grant_type=password" \
    -d "client_id=nfa-test" \
    -d "username=${user}" \
    -d "password=${pass}" \
    -d "scope=openid ${scope}" \
    | jq -r '.access_token // empty'
}

decode_scopes() {
  local token="$1"
  local payload padded
  payload=$(echo "$token" | cut -d. -f2 | tr '_-' '/+')
  # base64url omits padding; macOS base64 -d requires it.
  case $((${#payload} % 4)) in
    2) payload="${payload}==" ;;
    3) payload="${payload}=" ;;
  esac
  payload=$(echo "$payload" | base64 -d 2>/dev/null || true)
  echo "$payload" | jq -r '.scope // ""'
}

assert_scope() {
  local user="$1" pass="$2" want="$3" request="$4"
  local token scopes
  token=$(issue_token "$user" "$pass" "$request")
  if [ -z "$token" ]; then
    fail "${user}: could not issue token"
    return
  fi
  scopes=$(decode_scopes "$token")
  if echo " $scopes " | grep -q " ${want} "; then
    ok "${user}: granted ${want}"
  else
    fail "${user}: did not grant ${want} (got: ${scopes})"
  fi
}

assert_denied() {
  local user="$1" pass="$2" forbidden="$3" request="$4"
  local token scopes
  token=$(issue_token "$user" "$pass" "$request")
  if [ -z "$token" ]; then
    # A denial may surface as an outright token failure. That is a pass.
    ok "${user}: denied ${forbidden} (token refused)"
    return
  fi
  scopes=$(decode_scopes "$token")
  if echo " $scopes " | grep -q " ${forbidden} "; then
    fail "${user}: unexpectedly granted ${forbidden}"
  else
    ok "${user}: denied ${forbidden}"
  fi
}

echo "checking scope grants"

assert_scope   planner@local planner tenders:read  "tenders:read"
assert_scope   planner@local planner searches:write "searches:write"
assert_denied  planner@local planner admin:users    "admin:users"
assert_denied  planner@local planner audit:read     "audit:read"

assert_scope   analyst@local analyst audit:read      "audit:read"
assert_denied  analyst@local analyst admin:users     "admin:users"

assert_scope   admin@local admin admin:users         "admin:users"
assert_scope   admin@local admin ingest:read         "ingest:read"

assert_scope   officer@local officer privacy:read    "privacy:read"
assert_denied  officer@local officer admin:users     "admin:users"

# ---------------------------------------------------------------- service client
echo "checking service client"

SERVICE_TOKEN="$(curl -sf -X POST "${BASE}/realms/${REALM}/protocol/openid-connect/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=client_credentials" \
  -d "client_id=nfa-service" \
  -d "client_secret=local-service-secret-replace-in-prod" \
  -d "scope=ingest:read" \
  | jq -r '.access_token // empty')"

if [ -z "$SERVICE_TOKEN" ]; then
  fail "service client: could not issue token"
else
  ok "service client: token issued"
  SERVICE_SCOPES="$(decode_scopes "$SERVICE_TOKEN")"
  if echo " $SERVICE_SCOPES " | grep -q " ingest:read "; then
    ok "service client: granted ingest:read"
  else
    fail "service client: did not grant ingest:read (got: ${SERVICE_SCOPES})"
  fi
fi

echo
if [ "$FAIL" -eq 0 ]; then
  echo "all checks passed"
  exit 0
else
  echo "verification failed"
  exit 1
fi
