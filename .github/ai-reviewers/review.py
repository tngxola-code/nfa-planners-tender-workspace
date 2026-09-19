#!/usr/bin/env python3
"""Run one persona against a diff. Emit verdict and comment to JSON."""
import argparse
import json
import os
import re
import time
import urllib.error
import urllib.request

MODEL = os.environ.get("AI_REVIEW_MODEL", "claude-sonnet-4-5")
API = "https://api.anthropic.com/v1/messages"
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
    key = os.environ["ANTHROPIC_API_KEY"]
    req = urllib.request.Request(
        API,
        data=json.dumps(body).encode(),
        headers={
            "x-api-key": key,
            "anthropic-version": "2023-06-01",
            "content-type": "application/json",
        },
    )
    for attempt in range(MAX_RETRIES):
        try:
            with urllib.request.urlopen(req, timeout=180) as r:
                return json.load(r)["content"][0]["text"]
        except urllib.error.HTTPError as e:
            if e.code == 429 and attempt < MAX_RETRIES - 1:
                time.sleep((2 ** attempt) * 5)
                continue
            raise
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
        "model": MODEL,
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
