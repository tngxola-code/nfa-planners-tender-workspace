
# Contributing

## Branching

Trunk-based. `main` is always green. Branches live for one to three days.

```
feat/<phase>-<slug>     feat/1-tenders-list
fix/<slug>              fix/scope-check-order
chore/<slug>            chore/ci-timeout
docs/<slug>             docs/adr-0009
contract/<slug>         contract/add-provenance-endpoint
```

Commits follow [Conventional Commits](https://www.conventionalcommits.org/).
Squash-merge into `main`.

## Before you push

If CI fails with an environmental error (401, 403, missing secret, runner
image change), do not fix it in your PR. Open an issue, label it `ops`, and
fix it on a `chore/` branch. Your PR is not the place for infrastructure
repairs.

## Adding an endpoint

1. Edit `api/openapi.yaml`.
2. `make generate`.
3. Write the migration and the queries. `make generate` again.
4. Implement the handler.
5. Write the tests.
6. `make check`. Push. Open the PR.

## Quality gates

Local quality gates run through Lefthook. Install them once per clone:

```
lefthook install
```

Pre-commit runs against staged files only, under 15 seconds. Pre-push runs
the full project, under 2 minutes. Both mirror CI.

Full local check before pushing:

```
make check
```

## Contract

`api/openapi.yaml` is the contract of record. Every endpoint starts there.
Generated Go stubs, the TypeScript client, the Dart client, and the Postman
collection are produced from it. Never edit generated code by hand.

To change an endpoint:

1. Edit `api/openapi.yaml`.
2. Run `make generate`.
3. Commit the generated output in the same commit as the spec change.

CI fails on drift between the spec and the generated output.

## Testing

| Layer | Command | Runs against |
|---|---|---|
| Backend unit | `make test-backend-unit` | in-process, no containers |
| Backend integration | `make test-backend-integration` | Testcontainers |
| Backend contract | `make test-backend-contract` | served vs committed OpenAPI |
| Console unit | `make test-console-unit` | Vitest |
| Console integration | `make test-console-integration` | Vitest with local API |
| Console e2e | `make test-console-e2e` | Playwright |
| Mobile unit | `make test-mobile-unit` | flutter test |
| Mobile integration | `make test-mobile-integration` | flutter integration_test |
| Newman | `make contract` | generated Postman collection |
| All | `make test-all` | everything |

Merge gates: `make lint`, `make test-backend-unit`, `make contract`.

## Decisions

Decisions go in `docs/adr/`. The README and this file state what is. ADRs
state why.

Open questions live in `docs/open-questions.md`. When a question is answered,
remove it from there and record the answer in the README or an ADR.

## Diagrams

Diagrams under `docs/img/` are generated. Edit the Python source, re-run it,
and commit both the script and the SVG. Never edit the SVG directly.

```
make diagrams
```

## Secrets

Never commit a secret. `.env` is gitignored. Deployed secrets are
SOPS-encrypted under `deploy/secrets/`. See `docs/runbook.md` for rotation.
