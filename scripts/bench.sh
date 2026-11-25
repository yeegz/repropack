#!/usr/bin/env bash
# Reproducible benchmarks. Usage: scripts/bench.sh > docs/bench-raw.txt
set -euo pipefail
cd "$(dirname "$0")/.."
echo "# $(date -u +%Y-%m-%dT%H:%M:%SZ) $(go version) $(uname -sm)"
sysctl -n machdep.cpu.brand_string 2>/dev/null || true
go test ./format ./redact ./sdk ./viewer -run '^$' -bench . -benchmem -count 3 -benchtime 1s
