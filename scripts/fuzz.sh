#!/usr/bin/env bash
# Runs every fuzzer for $1 (default 90s). Usage: scripts/fuzz.sh 90s > docs/fuzz-raw.txt
set -uo pipefail
cd "$(dirname "$0")/.."
T=${1:-90s}
echo "# $(date -u +%Y-%m-%dT%H:%M:%SZ) $(go version) fuzztime=$T"
for pair in "format FuzzReader" "format FuzzEventLine" "redact FuzzRedactString" "otlp FuzzImport"; do
  read -r pkg fn <<<"$pair"
  echo "== $pkg $fn"
  go test "./$pkg" -run '^$' -fuzz "^${fn}\$" -fuzztime "$T" 2>&1 | tail -4
done
