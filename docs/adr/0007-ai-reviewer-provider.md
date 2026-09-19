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

- The model id is read from `AI_REVIEW_MODEL`, defaulting to `GEMINI_MODEL`.
- The API key is read from the `GOOGLE_API_KEY` repository secret.
- The key is sent as the `x-goog-api-key` header, never as a query parameter.
- Transient 429 and 5xx responses are retried with exponential backoff.

## Consequences
- A Google AI Studio account and API key are required for CI.
- Model deprecation becomes a routine maintenance event rather than a code
  change, because the id is a single env-var default.
- `docs/runbook.md` documents rotation for `GOOGLE_API_KEY`.

## Alternatives considered
- Stay on Anthropic. Rejected: migration already half-done, and the vendor
  showed two failure modes we could not diagnose from logs.
- Run a local model. Rejected: CI runtime and cost.
