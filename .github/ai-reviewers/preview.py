#!/usr/bin/env python3
"""Local pre-push preview: one persona, one diff, printed to stdout."""
import argparse
import json
import subprocess


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--diff", required=True)
    p.add_argument("--persona", default="architect")
    a = p.parse_args()

    subprocess.run(
        [
            "python3", ".github/ai-reviewers/review.py",
            "--persona", a.persona,
            "--diff", a.diff,
            "--adrs", "/tmp/adrs.txt",
            "--contract", "/tmp/contract.txt",
            "--questions", "/tmp/questions.txt",
            "--output", "/tmp/preview.json",
        ],
        check=True,
    )
    print(json.load(open("/tmp/preview.json"))["comment"])


if __name__ == "__main__":
    main()
