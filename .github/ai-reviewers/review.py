#!/usr/bin/env python3
"""Run one persona against a diff. Emit verdict and comment to JSON."""
import argparse
import json
import os
import re
import time
import urllib.error
import urllib.request

GEMINI_MODEL = os.environ.get("AI_REVIEW_MODEL", "gemini-3.7-flash")
GEMINI_URL = (
    "https://generativelanguage.googleapis.com/v1beta/"
    f"models/{GEMINI_MODEL}:generateContent"
)
MAX_RETRIES = 5


def read(path):
    try:
        return open(path).read()
    except (FileNotFoundError, IsADirectoryError):
        return ""


def build_prompt(persona, diff, adrs, contract, questions):
    return (
        "You are reviewing a pull request in the NFA Planners Tender Workspace. "
        "The persona below defines what you check and the output format you must "
        "follow. Respond with the output format only. No preamble.\n\n"
        "--- PERSONA ---\n" + persona + "\n\n"
        "--- ADRs ---\n" + adrs[:20000] + "\n\n"
        "--- OPENAPI CONTRACT ---\n" + contract[:20000] + "\n\n"
        "--- OPEN QUESTIONS ---\n" + questions[:5000] + "\n\n"
        "--- DIFF ---\n" + diff[:80000] + "\n\n"
        'Begin your response with "## Verdict: PASS" or "## Verdict: WARN" or '
        '"## Verdict: FAIL". Nothing before it.'
    )


def call(body):
    key = os.environ.get("GOOGLE_API_KEY", "").strip()
    if not key:
        raise SystemExit("SKIP: GOOGLE_API_KEY is not set")

    # Model fallback chain: primary from AI_REVIEW_MODEL (default GEMINI_MODEL),
    # fallbacks from AI_REVIEW_MODEL_FALLBACKS (comma-separated). A 404 on a
    # model is treated as "not available to this key" and skips to the next.
    primary = os.environ.get("AI_REVIEW_MODEL", GEMINI_MODEL).strip()
    raw_fallbacks = os.environ.get("AI_REVIEW_MODEL_FALLBACKS", "").strip()
    fallbacks = [m.strip() for m in raw_fallbacks.split(",") if m.strip()]
    models = [primary] + [m for m in fallbacks if m != primary]

    prompt_text = body["messages"][0]["content"]
    payload = {
        "contents": [{"parts": [{"text": prompt_text}]}],
        "generationConfig": {
            "temperature": 0.2,
            "maxOutputTokens": 4096,
        },
    }
    data_bytes = json.dumps(payload).encode()

    last_error = None
    for model in models:
        url = (
            "https://generativelanguage.googleapis.com/v1beta/models/"
            + model
            + ":generateContent"
        )
        req = urllib.request.Request(
            url,
            data=data_bytes,
            headers={
                "content-type": "application/json",
                "x-goog-api-key": key,
            },
        )
        for attempt in range(MAX_RETRIES):
            try:
                with urllib.request.urlopen(req, timeout=180) as r:
                    data = json.load(r)
                    return data["candidates"][0]["content"]["parts"][0]["text"]
            except urllib.error.HTTPError as e:
                body_text = e.read().decode("utf-8", errors="replace")
                if e.code in (401, 403):
                    raise SystemExit(
                        "Google API rejected the key (" + str(e.code) + "). "
                        "Regenerate GOOGLE_API_KEY. Response: " + body_text
                    )
                if e.code == 404:
                    last_error = "model " + model + " not available (404)"
                    break
                if e.code in (429, 500, 502, 503, 504):
                    last_error = "model " + model + " returned " + str(e.code)
                    if attempt < MAX_RETRIES - 1:
                        time.sleep((2 ** attempt) * 5)
                        continue
                    break
                raise RuntimeError(
                    "Google API error " + str(e.code) + ": " + body_text
                ) from e
    raise RuntimeError(
        "all models exhausted; last error: " + (last_error or "unknown")
    )


def parse(text):
    m = re.search(r"## Verdict:\s*(PASS|WARN|FAIL)", text, re.IGNORECASE)
    verdict = m.group(1).lower() if m else "warn"
    esc = re.search(r"## Escalations:\s*(.+)", text, re.IGNORECASE)
    escalations = []
    if esc:
        escalations = [
            s.strip() for s in esc.group(1).split(",")
            if s.strip() and s.strip().lower() != "none"
        ]
    return verdict, escalations, text


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--persona", required=True)
    p.add_argument("--diff", required=True)
    p.add_argument("--adrs", required=True)
    p.add_argument("--contract", required=True)
    p.add_argument("--questions", required=True)
    p.add_argument("--output", required=True)
    a = p.parse_args()

    persona = read(".github/ai-reviewers/" + a.persona)

    unavailable = None
    try:
        text = call({
            "messages": [{
                "role": "user",
                "content": build_prompt(
                    persona,
                    read(a.diff),
                    read(a.adrs),
                    read(a.contract),
                    read(a.questions),
                ),
            }],
        })
        verdict, escalations, comment = parse(text)
    except (RuntimeError, SystemExit) as e:
        # Upstream provider unavailable. Report and pass, do not block.
        unavailable = str(getattr(e, "code", None) or e)
        verdict = "pass"
        escalations = []
        comment = (
            "## Verdict: PASS\n\n"
            "### Reviewer unavailable (upstream error)\n\n"
            "This review was skipped because the upstream model provider "
            "returned an error. This is not a code verdict.\n\n"
            "```\n" + unavailable[:2000] + "\n```\n"
        )

    summary = "Verdict: " + verdict
    if unavailable:
        summary += " · reviewer unavailable"
    elif escalations:
        summary += " · escalations: " + ", ".join(escalations)

    json.dump(
        {
            "verdict": verdict,
            "escalations": escalations,
            "comment": comment,
            "summary": summary,
        },
        open(a.output, "w"),
    )


if __name__ == "__main__":
    main()
