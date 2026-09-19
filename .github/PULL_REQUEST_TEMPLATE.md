## What changed

<!-- One paragraph. What does this PR do? -->

## Why

<!-- Link the ADR, the open question, or the bug. -->

## Contract

- [ ] No change to `api/openapi.yaml`
- [ ] Change is additive only
- [ ] Change adds a new path segment (breaking)

## Tests

- [ ] Unit tests added or updated
- [ ] Integration tests added or updated (if touching DB, OpenSearch, MinIO)
- [ ] Contract test still passes
- [ ] End-to-end tests added or updated (if user-facing behaviour changed)

## Checklist

- [ ] `make check` passes locally
- [ ] Generated code regenerated and committed
- [ ] Diagrams regenerated if topology or flow changed
- [ ] README or docs updated if a fact changed
- [ ] Open questions updated if a question was answered
- [ ] If this PR changes `.github/workflows/` or `.github/ai-reviewers/`,
      I have run the affected workflow in a draft PR or via `workflow_dispatch`
      before requesting review.
