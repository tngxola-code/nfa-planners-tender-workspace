# ADR 0007: AI reviewer provider is Google Gemini

## Status
Accepted

## Context
The AI reviewer jobs (`.github/ai-reviewers/review.py`) call an external
LLM. They originally used Anthropic's Claude API. That dependency became
untenable: the repository secret was unset, the model id aged out, and the
health check could not distinguish "key missing" from "provider error".

## Decision
Use Google Gemini via the `generativelanguage.googleapis.com` REST API.

- Model id comes from `AI_REVIEW_MODEL`, defaulting to `GEMINI_MODEL`.
- `AI_REVIEW_MODEL_FALLBACKS` is a comma-separated fallback chain used when
  the primary returns a transient error or is unavailable.
- The API key is read from the `GOOGLE_API_KEY` repository secret.
- The key is sent as the `x-goog-api-key` header, never as a query parameter.
- Transient 429 and 5xx responses are retried with exponential backoff; if
  retries are exhausted on one model, the next model in the chain is tried.
- The reviewer is an advisory check, not a merge gate. When the upstream
  provider is unavailable, the workflow warns and exits 0.

## Consequences
- A Google AI Studio account and API key are required for CI.
- Model deprecation becomes a routine maintenance event rather than a code
  change, because the id is a single env-var default.
- `docs/runbook.md` documents rotation for `GOOGLE_API_KEY`.

## Alternatives considered
- Stay on Anthropic. Rejected: the migration was already half-done and
  the vendor showed two failure modes we could not diagnose from logs.
- Run a local model. Rejected: CI runtime and cost.
