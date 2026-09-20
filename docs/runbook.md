# Runbook

Operational procedures for the NFA Planners Tender Workspace. One section per
external dependency and per automated check.

When something in CI is red and it is not a code defect, the fix belongs here.
Update this file as part of the fix so the next person has the answer.

---

## GOOGLE_API_KEY

The AI reviewer jobs call the Google Gemini API. Without this secret they fail
with a 401.

| | |
|---|---|
| Set in | Repo Settings > Secrets and variables > Actions |
| Scope | Repository. Not environment. |
| Rotation | Quarterly, or immediately on suspected leak |
| Health check | `.github/workflows/ai-review-health.yml`, daily at 06:00 UTC |
| Failure symptom | `review (go)` and `review (architect)` fail with HTTP 401/403 |
| Fix | Rotate the key, update the secret, re-run the health workflow |

### How to rotate

1. Create a new API key in Google AI Studio. It must be a long-lived
   `AIzaSy...` key, 39 characters. An `AQ.`-prefixed value is a short-lived
   session token: it passes a manual `curl` and then expires in CI, which
   presents as an intermittent 401 with no code change to explain it.
2. Revoke the old key.
3. Update `GOOGLE_API_KEY` in repo settings.
4. Run the health workflow manually to confirm.
5. Note the rotation date in the team's password manager.
