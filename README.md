# NFA Planners Tender Workspace

A single-tenant tender planning system for NFA Town and Regional Planners. A Go API ingests an external procurement data feed into PostgreSQL, indexes it in OpenSearch, and serves it to a web console and a mobile app. Every read and write is audited.

## Status

```
Phase           build
Environments    local only
Contract        v1, evolving
Deferred        Kubernetes, Terraform, HashiCorp Vault, Harbor,
                ArgoCD, OpenSearch cluster  (see Growth path)
Open questions  docs/open-questions.md
```

Nothing below describes deferred work. Anything not yet decided is a question in `docs/open-questions.md`, not a claim here.

## Contents

- [Quickstart](#quickstart)
- [Architecture](#architecture)
- [Repository layout](#repository-layout)
- [Common tasks](#common-tasks)
- [Configuration](#configuration)
- [Environments](#environments)
- [Testing](#testing)
- [Build and release](#build-and-release)
- [Deployment](#deployment)
- [Growth path](#growth-path)
- [Compliance](#compliance)
- [Operations](#operations)
- [Documentation](#documentation)
- [Contributing](#contributing)
- [Security](#security)
- [License](#license)

## Quickstart

Prerequisites: Go 1.24+, Node 22+ with pnpm, Flutter 3.29+, Docker and Docker Compose, GNU Make. `make bootstrap` installs the rest.

```bash
git clone git@github.com:tngxola-code/nfa-planners-tender-workspace.git
cd nfa-planners-tender-workspace
make bootstrap && make init && make up && make migrate && make seed && make run-api
```

`make bootstrap` installs tooling and changes no state. `make init` creates `.env` from `.env.example` and generates local certificates. `make up` needs `.env` to exist.

`curl -s localhost:8000/healthz` returns `{"status":"ok"}`. The health endpoint is unversioned.

Then `make run-console` for the web console, or `cd mobile && flutter run` for the mobile app. Ports are in `docker-compose.yml`; `make ports` prints them.

Tear down with `make down`, or `make clean` to drop volumes too.

## Architecture

Both clients talk only to the API over HTTPS. The API is the only component holding database credentials. Caddy is the only container with a published port.

### Trust zones
![nfa-trust-zones.svg](../../Downloads/nfa-trust-zones.svg)

### Request path
![request-path.svg](../../Downloads/request-path.svg)

The diagrams are the source of truth for the chain and the topology. They are generated from `docs/img/*.py`: edit the script, re-run it, commit both.

### Data layers

| Layer | Contents | Mutability |
|---|---|---|
| Source | external feed releases | external |
| Staging | `stg_releases`, raw JSONB as received | append-only |
| Canonical | `tenders`, `awards`, `contracts`, `buyers`, `parties`, `documents`, `planning` | versioned |
| API | OpenAPI 3.1 responses, `application/json` | derived |

The API reads only from the canonical layer. Field lineage is in `docs/data-mapping-tables.html`.

### Stack

| Layer | Choice |
|---|---|
| Backend | Go 1.24, Chi, pgx, sqlc, goose, river |
| Web console | Next.js 15, TypeScript, static export |
| Mobile | Flutter 3.29, Riverpod, drift |
| Identity | Keycloak 26 |
| Database | PostgreSQL 17 |
| Search | OpenSearch 2.19 |
| Object storage | MinIO (WORM bucket policy) |
| Cache and idempotency | Redis 7.4 |
| Ingress | Caddy 2.8 |
| Secrets | SOPS + age |
| Images | GitHub Container Registry |
| Console hosting | Vercel (static assets only) |

Pinned versions and the full component inventory are in `docs/technology-stack.html`.

## Repository layout

```
├── api/openapi.yaml                    contract of record, served at /openapi.json
├── backend/                            Go 1.24
│   ├── cmd/                            api, worker, migrate
│   ├── internal/
│   │   ├── api/                        generated, do not edit
│   │   ├── auth/                       OIDC, JWT, client identity, scope
│   │   ├── httpx/                      problem+json, request-id, idempotency
│   │   ├── db/queries/  db/sqlc/       .sql inputs, generated, do not edit
│   │   ├── domain/                     tender, award, contract, buyer, party,
│   │   │                               document, search, workflow, report,
│   │   │                               audit, privacy
│   │   ├── ingest/  workers/  platform/
│   └── migrations/                     goose .sql
├── console/                            Next.js 15, static export
│   ├── src/app/  src/features/  src/components/
│   ├── src/lib/api/                    generated, do not edit
│   └── e2e/                            Playwright
├── mobile/                             Flutter 3.29
│   ├── lib/core/  lib/data/  lib/features/  lib/shared/
│   └── lib/data/api/                   generated, do not edit
├── deploy/
│   ├── compose/                        uat.yml, prod.yml
│   ├── caddy/                          Caddyfile per environment
│   └── secrets/                        SOPS-encrypted; recipients in .sops.yaml
├── docs/
│   ├── adr/                            architecture decision records
│   ├── img/                            diagram scripts and SVG output
│   ├── configuration.md                environment variable reference
│   ├── open-questions.md               everything not yet decided
│   └── *.html                          architecture, stack, data mapping, roadmap
├── tools/postman/                      executable contract
├── .env.example  docker-compose.yml  Makefile
├── CONTRIBUTING.md  SECURITY.md  CHANGELOG.md
└── .github/workflows/                  backend, console, mobile, contract
```

Generated directories are committed. CI regenerates and compares; a difference fails the build.

## Common tasks

| Command | Does |
|---|---|
| `make bootstrap` | Install toolchain and dependencies (idempotent, no state change) |
| `make init` | Create `.env` from `.env.example`, generate local certs |
| `make up` / `make down` / `make clean` | Start / stop the local stack / drop volumes |
| `make migrate` / `make seed` | Apply migrations / load synthetic reference data |
| `make run-api` / `make run-worker` / `make run-console` | Run a component against the local stack |
| `make generate` | Regenerate sqlc, oapi-codegen, TypeScript and Dart clients |
| `make test-backend` | Backend unit tests, in-process |
| `make test-backend TAGS=integration` | Backend integration tests, Testcontainers |
| `make test-console` / `make test-mobile` | Console and mobile test suites |
| `make contract` | Newman against the local stack |
| `make lint` | golangci-lint, Spectral, eslint, `dart analyze` |
| `make load-test` / `make dast` | k6 and OWASP ZAP, both against the local stack |
| `make build` / `make docker` | Binaries and bundles / container images |
| `make diagrams` | Re-run `docs/img/*.py` |
| `make secrets-edit ENV=uat` | Open the SOPS-encrypted secrets file for an environment |
| `make ports` | Print the local port map |

## Configuration

The API reads configuration from environment variables and refuses to start if a required one is unset. There are no defaults for credentials.

Full reference: `docs/configuration.md`. Types: `backend/internal/config/config.go`. Local starting point: `.env.example`.

In deployed environments, values come from a SOPS-encrypted file decrypted on the VM at deploy time.

## Environments

| | local | UAT | production |
|---|---|---|---|
| Runs on | developer machine | SA VM | SA VM |
| Console | `localhost:3000` | `uat.console.nfaplanners.com` | `console.nfaplanners.com` |
| API | `localhost:8000` | `uat.api.nfaplanners.com` | `api.nfaplanners.com` |
| Identity | `localhost:8081` | `uat.sso.nfaplanners.com` | `sso.nfaplanners.com` |

Production data is never copied to any other environment. Test fixtures are synthetic and contain no real planner names, addresses or credentials.

## Testing

| Suite | Command | Runs against |
|---|---|---|
| Backend unit | `make test-backend` | in-process, no containers |
| Backend integration | `make test-backend TAGS=integration` | Testcontainers: PostgreSQL, OpenSearch, MinIO |
| Console | `make test-console` | Vitest, then Playwright against a local static build |
| Mobile | `make test-mobile` | widget and unit tests, then `integration_test` on a simulator |
| Contract | `make contract` | Newman against the local stack |
| Load | `make load-test` | k6 against the local stack |
| Security | `make dast` | OWASP ZAP against the local stack |

Merge gates: `make lint`, `make test-backend`, `make contract`. Console pull requests additionally run Playwright against the Vercel preview deployment.

Coverage target: 80 percent line coverage on `internal/domain`.

## Build and release

Three artefacts on independent pipelines, sharing one contract: `api/openapi.yaml`.

**Backend.** Every merge to `main`: build, `golangci-lint`, `go test -race`, `sqlc diff` and `oapi-codegen diff`, Spectral lint, Semgrep and gosec, Docker build with Trivy, Cosign signing, push to GHCR, deploy to UAT over SSH, Newman smoke run against UAT.

**Console.** Vercel builds from the same repository. A pull request produces a preview deployment; a merge to `main` deploys to production.

**Mobile.** Codemagic builds signed iOS and Android artefacts and publishes to TestFlight and the Play internal track. Store submission is manual.

A breaking contract change gets a new path segment, not an override. Versioning follows [Semantic Versioning](https://semver.org/).

## Deployment

Production and UAT run as separate Docker Compose projects on one South African VM.

```
SA VM
├── Caddy             the only published ports (80, 443)
├── Go API            private network
├── Worker            private network
├── Keycloak          published through Caddy
├── PostgreSQL 17     private network, volume-backed
├── OpenSearch 2.19   private network, single node
├── MinIO             private network, WORM bucket policy
├── Redis 7.4         private network
└── OTel collector    private network
```

Deploy: `docker compose pull && docker compose up -d` against the target project. Rollback: the same command pinned to the previous image tag. Production promotion is manual and requires a second approver from the NFA delivery team.

Secrets are SOPS-encrypted with age recipients, decrypted on the VM at deploy time, never written to disk in plaintext. No secret manager runs at runtime.

Why Compose rather than Kubernetes has not been written up yet. ADR backlog is in `docs/open-questions.md`.

## Growth path

Deferred. None of these is in use.

| Tool | Replaces | Adopt when |
|---|---|---|
| Kubernetes | Docker Compose on one VM | A second VM is needed for availability, or a second tenant |
| Terraform | Manual VM provisioning | The VM is rebuilt more than twice a year |
| HashiCorp Vault | SOPS + age | Secrets need dynamic issuance or per-request leasing |
| Harbor | GHCR | Images must be mirrored inside the SA boundary |
| ArgoCD | SSH deploy step | Reconciliation is needed across more than one cluster |
| OpenSearch cluster | Single node | Index size or query load exceeds one node |

## Compliance

| Requirement | Control |
|---|---|
| POPIA (Protection of Personal Information Act) | All personal information is stored and processed on the SA VM, except what the sub-processors table below lists as leaving it. Encryption in transit and at rest. Access and erasure endpoints under `/v1/privacy`. |
| PFMA (Public Finance Management Act) | Audit row written synchronously before every response; retention `AUDIT_RETENTION_DAYS`, default seven years. Documents SHA-256 hashed in WORM storage. |
| National Treasury SCM regulations | Source tender identifier and reference preserved on every record. No source value is rewritten. |
| B-BBEE Act | B-BBEE level stored as a local extension on awards and parties, exposed as a search facet. |
| CIDB Act | CIDB grading stored as a local extension on tenders and parties. |

Section-level mapping is in `docs/system-architecture.html`.

**Zero-trust**, as used here, means: every endpoint requires a valid token with the correct scope, and there are no unauthenticated endpoints except `/healthz` and `/openapi.json`. It does not currently claim network micro-segmentation or least-privilege database roles.

### Sub-processors

| Processor | Jurisdiction | Processes | Basis |
|---|---|---|---|
| Vercel | offshore | Console static assets. Edge access logs contain planner IP addresses. | DPA and SOC 2 Type 2 pending, see C5 and C6 |
| Firebase Cloud Messaging | offshore | Mobile device tokens and push notification payloads. | see `docs/open-questions.md` |

The console is a **static export**: no Server Actions, no API routes, no server-side data fetching. API and authentication calls go from the client directly to `api.nfaplanners.com` and `sso.nfaplanners.com` and do not transit Vercel. Access tokens are never sent to Vercel.

`regions: ["cpt1"]` in `vercel.json` places serverless and edge functions. This deployment has none, so it is not a residency control and is not relied on as one.

Adding offshore compute to any NFA data path (a Server Action, an API route, another processor) requires Information Officer sign-off and a ROPA update before merge.

### Erasure

Erasure pseudonymises personal fields in audit rows rather than deleting them. Event, action, resource and timestamp are retained. Legal basis is recorded in the ROPA.

### Third-party licences

MinIO is AGPL-3.0. Syncfusion's Flutter PDF viewer is commercially licensed with a community tier. Both need a recorded decision. See `docs/open-questions.md`. An SBOM is published with each backend release.

## Operations

There is no runbook. Service levels, backup and restore, log retention, on-call, the incident severity ladder and the behaviour of the API when a dependency is unavailable are all unset. See `docs/open-questions.md`.

## Documentation

| Path | Covers |
|---|---|
| `api/openapi.yaml` | Contract of record. Served at `/openapi.json`. CI compares served against committed. |
| `tools/postman/` | Executable form of the contract. If it disagrees with the OpenAPI file, the collection is wrong. |
| `docs/system-architecture.html` | Context, containers, components, data flow, deployment topology, observability |
| `docs/technology-stack.html` | Component inventory, pinned versions, licensing |
| `docs/data-mapping-tables.html` | Source to canonical to API field lineage, code lists, quality flags |
| `docs/features-build-roadmap.html` | Feature catalogue, roadmap, milestone gates, risk register |
| `docs/configuration.md` | Environment variable reference |
| `docs/open-questions.md` | Everything not yet decided |
| `docs/adr/` | Architecture decision records. Currently empty; the backlog of ADRs still to write is in `docs/open-questions.md`. |

## Contributing

Branch from `main` as `feat/`, `fix/`, `chore/` or `docs/` plus a slug. Commits follow [Conventional Commits](https://www.conventionalcommits.org/).

Before opening a pull request: `make lint && make test-backend && make contract`.

Contract changes start in `api/openapi.yaml`. Run `make generate` and commit the generated output in the same change. Schema changes ship as a goose migration.

Decisions go in `docs/adr/`, not in this README. This README states what is; ADRs state why.

See `CONTRIBUTING.md`.

## Security

Do not open a public issue for a security finding. Reporting address, PGP key and acknowledgement window are in `SECURITY.md`.

Findings touching personal information or the audit trail are P1 regardless of exploitability.

## License

Perpetual, single-tenant licence. No metering, no feature gates.

Copyright (c) 2026 NFA Town and Regional Planners. All rights reserved.
