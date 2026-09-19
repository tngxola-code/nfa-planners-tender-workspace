# Keycloak realm

The NFA realm is defined by `realm.json`. It is the only source of truth for
clients, roles, client scopes, role-to-scope mappings, and test users.

Nothing is configured through the Keycloak admin UI. Any change to the realm is
a change to `realm.json`, on a branch, through the reviewers, like everything
else.

## What lives here

| File | Purpose |
|---|---|
| `realm.json` | The realm. Imported by Keycloak on first start. |
| `verify.sh` | Asserts the realm is loaded and the scope model behaves. |
| `README.md` | This file. |

## How the realm is loaded

`docker-compose.yml` mounts this folder read-only into the Keycloak container
at `/opt/keycloak/data/import/`. Keycloak runs with `--import-realm`, which
reads every `.json` file in that folder on start.

The import only runs if the realm does not already exist. To force a fresh
import after editing `realm.json`, run `make keycloak-reset`. That stops the
container, removes the container and its data volume, and starts a new one.

## The scope model

Every API scope from `api/openapi.yaml` is a Keycloak client scope. Every
client sets `fullScopeAllowed: false`, so a token contains only the scopes the
caller's role was explicitly granted. A `planner` cannot obtain
`admin:users` even if the client asks for it.

Role to scope mappings are declared on each client. The table:

| Role | Scopes |
|---|---|
| planner | tenders:read, awards:read, contracts:read, buyers:read, parties:read, documents:read, search:read, searches:read, searches:write, alerts:read, alerts:write, workflows:read, workflows:write, reports:read |
| analyst | planner plus audit:read |
| admin | analyst plus tenders:write, ingest:read, admin:users |
| information_officer | reports:read, audit:read, privacy:read |
| service_ingest | ingest:read |
| service_dispatch | none, internal scopes only |

## Test users

For local and UAT only. Each has a fixed password and a role.

| Username | Password | Role |
|---|---|---|
| planner@local | planner | planner |
| analyst@local | analyst | analyst |
| admin@local | admin | admin |
| officer@local | officer | information_officer |

These users are in `realm.json`. They are not provisioned in production. The
production realm is generated from this file with the `users` array stripped
and the `nfa-service` client secret replaced from SOPS. That transform is a
deploy-time step, not part of this branch.

## The clients

| Client | Type | Flow | Purpose |
|---|---|---|---|
| nfa-console | public | Authorization Code + PKCE (S256) | Web console |
| nfa-mobile | public | Authorization Code + PKCE (S256) | Flutter app |
| nfa-service | confidential | Client Credentials | Worker and internal callers |

The console and mobile clients allow Vercel preview redirect URIs. The
production realm removes the wildcard.

## Iterating on the realm

The preferred workflow:

1. Edit `realm.json`.
2. `make keycloak-reset`.
3. `make keycloak-verify`.
4. Commit the changed `realm.json` on a branch.

Do not use the admin UI to make changes and export them. The export includes
UUIDs that change every time and produce noisy diffs. The file is hand-edited,
reviewed, and imported.
