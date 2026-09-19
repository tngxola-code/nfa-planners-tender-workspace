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

    model = os.environ.get("AI_REVIEW_MODEL", "gemini-3.6-flash")
    url = (
        "https://generativelanguage.googleapis.com/v1beta/models/"
        + model + ":generateContent?key=" + key
    )

    # Translate the review payload into Gemini's contents shape.
    prompt_text = body["messages"][0]["content"]
    payload = {
        "contents": [{"parts": [{"text": prompt_text}]}],
        "generationConfig": {
            "temperature": 0.2,
            "maxOutputTokens": 4096,
        },
    }

    req = urllib.request.Request(
        url,
        data=json.dumps(payload).encode(),
        headers={"content-type": "application/json"},
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
            if e.code == 429 and attempt < MAX_RETRIES - 1:
                time.sleep((2 ** attempt) * 5)
                continue
            raise RuntimeError(
                "Google API error " + str(e.code) + ": " + body_text
            ) from e
    raise RuntimeError("retries exhausted")

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

    persona = read(".github/ai-reviewers/" + a.persona + ".md")
    text = call({
        "model": GEMINI_GEMINI_MODEL,
        "max_tokens": 4096,
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
    summary = "Verdict: " + verdict
    if escalations:
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
