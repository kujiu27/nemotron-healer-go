#!/usr/bin/env bash
# Runs full heal loops against the built-in labeled mock — zero API keys.
# Three modes: plain, --search (parallel DHS), --arena (Red-Blue self-play).
# Demonstrates wiring only: mock responses are marked X-Nemotron-Healer: MOCK
# and no receipts are written.
set -uo pipefail
REPO=$(cd "$(dirname "$0")/.." && pwd)
cd "$REPO"

go build -o bin/nemotron-healer ./cmd/nemotron-healer
run_mode() { # label extra-flags...
  local label="$1"; shift
  echo "=== MOCK pipeline run [$label]: heal samples/external_gjson (AHB-08) ==="
  local tmp rc=0
  tmp=$(mktemp -d)
  cp -R samples/external_gjson/. "$tmp/"
  (
    cd "$tmp"
    "$REPO/bin/nemotron-healer" . --mock --command "go test . -run TestEmptyValueQuery -count=1" --turns 2 --no-tui "$@"
  ) || rc=$?
  echo "=== [$label] mock heal exit code: $rc (workspace was $tmp) ==="
  return $rc
}

# --json stdout must be a single parseable JSON document (machine contract).
run_json_mode() {
  echo "=== MOCK pipeline run [json-purity]: stdout must be pure JSON ==="
  local tmp rc=0
  tmp=$(mktemp -d)
  cp -R samples/external_gjson/. "$tmp/"
  (
    cd "$tmp"
    "$REPO/bin/nemotron-healer" . --mock --command "go test . -run TestEmptyValueQuery -count=1" --turns 1 --no-tui --yes --json 2>/dev/null \
      | python3 -c "import json,sys; j=json.load(sys.stdin); print('pure JSON, resolved:', j['is_resolved'])"
  ) || rc=$?
  echo "=== [json-purity] exit code: $rc ==="
  return $rc
}
run_eval_benchmark_mode() {
  echo "=== MOCK pipeline run [eval-ablation]: AHB-9 benchmark ablation ==="
  local rc=0
  "$REPO/bin/nemotron-healer" eval --mock --case AHB-08 || rc=$?
  echo "=== [eval-ablation] exit code: $rc ==="
  return $rc
}
run_real_eval_smoke() {
  echo "=== MOCK pipeline run [real-eval-script-smoke]: verify scripts/run_real_eval.sh mock path ==="
  local tmp rc=0
  tmp=$(mktemp -d)
  EVAL_OUT="$tmp/eval.md" EVAL_JSON="$tmp/eval.json" EVAL_MD="$tmp/scorecard.md" EVAL_CASE="AHB-08" REPEAT=1 \
    bash "$REPO/scripts/run_real_eval.sh" --mock || rc=$?
  if [ ! -f "$tmp/eval.md" ] || [ ! -f "$tmp/eval.json" ] || [ ! -f "$tmp/scorecard.md" ]; then
    echo "ERROR: scripts/run_real_eval.sh did not produce expected output files in mock mode"
    rc=1
  fi
  rm -rf "$tmp"
  echo "=== [real-eval-script-smoke] exit code: $rc ==="
  return $rc
}

rc=0
run_mode plain || rc=$?
run_mode search --search || rc=$?
run_mode arena --arena || rc=$?
run_json_mode || rc=$?
run_eval_benchmark_mode || rc=$?
run_real_eval_smoke || rc=$?
exit $rc
