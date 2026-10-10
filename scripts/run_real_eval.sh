#!/usr/bin/env bash
# Real-API eval: requires NEBIUS_API_KEY + TAVILY_API_KEY (or pass --mock / EVAL_MOCK=true).
# Writes timestamped results to docs/EVAL_RESULTS.md (appended),
# docs/EVAL_RESULTS.json (structured benchmark data), and docs/EVAL_SCORECARD.md.
# EVAL_OUT, EVAL_JSON, and EVAL_MD override destinations (testing only — mock runs
# must never touch the committed receipt).
set -euo pipefail

MOCK_FLAG=""
if [ "${EVAL_MOCK:-false}" = "true" ] || [ "${1:-}" = "--mock" ]; then
  MOCK_FLAG="--mock"
  export NEBIUS_API_KEY="${NEBIUS_API_KEY:-mock}"
  export TAVILY_API_KEY="${TAVILY_API_KEY:-mock}"
fi

: "${NEBIUS_API_KEY:?export NEBIUS_API_KEY first (or pass --mock for offline run)}"
: "${TAVILY_API_KEY:?export TAVILY_API_KEY first (or pass --mock for offline run)}"

COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
STAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
OUT="${EVAL_OUT:-docs/EVAL_RESULTS.md}"
JSON_OUT="${EVAL_JSON:-docs/EVAL_RESULTS.json}"
MD_OUT="${EVAL_MD:-docs/EVAL_SCORECARD.md}"

BIN=$(mktemp /tmp/nh-eval-bin-XXXX)
trap 'rm -f "$BIN"' EXIT

go build -trimpath -ldflags="-s -w" -o "$BIN" ./cmd/nemotron-healer

CASE_FLAG=""
if [ -n "${EVAL_CASE:-}" ]; then
  CASE_FLAG="--case $EVAL_CASE"
fi

mkdir -p "$(dirname "$OUT")" "$(dirname "$JSON_OUT")" "$(dirname "$MD_OUT")"

{
  echo ""
  echo "## Real API eval — ${STAMP} — commit ${COMMIT} — model $(grep -o 'Nemotron-3-Ultra[^"]*' internal/client/nebius.go | head -1)"
  echo '```'
  "$BIN" eval --repeat "${REPEAT:-3}" $MOCK_FLAG $CASE_FLAG --export-json "$JSON_OUT" --export-md "$MD_OUT" 2>&1
  echo '```'
} >> "$OUT"

echo "Appended terminal log to $OUT"
echo "Exported benchmark JSON to $JSON_OUT"
echo "Exported scorecard table to $MD_OUT"
