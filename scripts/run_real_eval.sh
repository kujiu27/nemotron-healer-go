#!/usr/bin/env bash
# Real-API eval: requires NEBIUS_API_KEY + TAVILY_API_KEY.
# Writes timestamped results to docs/EVAL_RESULTS.md (appended).
set -euo pipefail
: "${NEBIUS_API_KEY:?export NEBIUS_API_KEY first}"
: "${TAVILY_API_KEY:?export TAVILY_API_KEY first}"

COMMIT=$(git rev-parse --short HEAD)
STAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
OUT=docs/EVAL_RESULTS.md

go build -o /tmp/nemotron-healer-eval ./cmd/nemotron-healer
{
  echo ""
  echo "## Real API eval — ${STAMP} — commit ${COMMIT} — model $(grep -o 'Nemotron-3-Ultra[^"]*' internal/client/nebius.go | head -1)"
  echo '```'
  /tmp/nemotron-healer-eval eval --repeat "${REPEAT:-3}" 2>&1
  echo '```'
} >> "$OUT"
echo "Appended to $OUT"
