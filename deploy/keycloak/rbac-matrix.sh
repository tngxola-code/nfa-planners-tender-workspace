#!/usr/bin/env bash
# Exercise Authenticate + RequireScope end to end against the local realm.
# Rows: username  scope  path  expected_status
set -u

ISSUER=${OIDC_ISSUER:-http://localhost:8081/realms/nfa}
API=${API_BASE:-http://127.0.0.1:8080}
CLIENT=${OIDC_TEST_CLIENT:-nfa-test}

tok() {
  local user=$1 scope=$2 t
  t=$(curl -s -X POST "$ISSUER/protocol/openid-connect/token" \
        -d grant_type=password -d "client_id=$CLIENT" \
        -d "username=$user@local" -d "password=$user" \
        -d "scope=openid $scope" | jq -r '.access_token // empty')
  [ -n "$t" ] || { echo "FAIL: no token for $user / $scope" >&2; return 1; }
  printf '%s' "$t"
}

fail=0
while read -r user scope path want; do
  [ -z "${user:-}" ] && continue
  case "$user" in \#*) continue ;; esac
  got=$(curl -s -o /dev/null -w '%{http_code}' "$API$path" \
          -H "Authorization: Bearer $(tok "$user" "$scope")")
  if [ "$got" = "$want" ]; then
    printf '  ✓ %-8s %-14s %-16s %s\n' "$user" "$scope" "$path" "$got"
  else
    printf '  ✗ %-8s %-14s %-16s got %s want %s\n' "$user" "$scope" "$path" "$got" "$want"
    fail=1
  fi
done <<'ROWS'
planner admin:users   /v1/admin/ping   403
admin   admin:users   /v1/admin/ping   200
admin   ingest:read   /v1/service/ping 200
planner ingest:read   /v1/service/ping 403
ROWS

exit $fail
