#!/usr/bin/env python3
"""Read changed paths on stdin, emit reviewers and escalations to GITHUB_OUTPUT."""
import fnmatch
import json
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
MATRIX = json.load(open(os.path.join(HERE, "matrix.json")))

changed = [line.strip() for line in sys.stdin if line.strip()]
reviewers = set()
escalations = set()

for rule in MATRIX["rules"]:
    if any(fnmatch.fnmatch(p, pat) for p in changed for pat in rule["match"]):
        reviewers.update(rule.get("reviewers", []))
        if "escalate" in rule:
            escalations.add(rule["escalate"])

floor = MATRIX["defaults"]["min_reviewers"]
if len(reviewers) < floor:
    reviewers.update(MATRIX["defaults"]["fallback"])

reviewers_line = "reviewers=" + json.dumps(sorted(reviewers))
escalations_line = "escalations=" + ",".join(sorted(escalations))

out = os.environ.get("GITHUB_OUTPUT")
if out:
    with open(out, "a") as fh:
        fh.write(reviewers_line + "\n")
        fh.write(escalations_line + "\n")
else:
    print(reviewers_line)
    print(escalations_line)
