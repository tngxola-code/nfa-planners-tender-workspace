#!/usr/bin/env bash
# Build what exists. Skips cmd/* targets that haven't landed yet so the
# script stays green while the project is being scaffolded.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root/backend"

mkdir -p bin

for cmd in api worker migrate; do
  if compgen -G "cmd/${cmd}/*.go" > /dev/null; then
    echo "building cmd/${cmd}"
    go build -o "bin/${cmd}" "./cmd/${cmd}"
  else
    echo "skipping cmd/${cmd}: no Go files yet"
  fi
done
