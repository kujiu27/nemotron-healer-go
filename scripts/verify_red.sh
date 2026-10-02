#!/usr/bin/env bash
# Benchmark RED-state integrity check.
#
# Every AHB case must ship in a genuinely failing (red) state. This once bit
# us: a demo run healed a sample in place and the healed version got
# committed, silently turning AHB-02 into a free pass (caught in a later
# audit; see GRAND_PRIZE_GAP_AUDIT.md remediation ledger). This script makes
# that class of corruption a hard, reproducible failure.
#
# Sandbox semantics mirror internal/cli/eval.go createIsolatedSandbox():
#   - if `x.orig` exists, the `.orig` (broken) version becomes `x`
#   - the fixed `x` and the `.orig` file itself are NOT shipped
#
# Cases and commands MUST stay in sync with the table in internal/cli/eval.go.
# Exit 0 = every case is red (its test command fails). Any green case = exit 1.
set -uo pipefail
cd "$(dirname "$0")/.."

PYTEST="pytest"
command -v pytest >/dev/null 2>&1 || PYTEST="python3 -m pytest"
# Portable 120s watchdog: GNU timeout if present, else perl alarm (macOS).
with_watchdog() { # cmd...
  if command -v timeout >/dev/null 2>&1; then
    timeout 120 "$@"
  else
    perl -e 'alarm 120; exec @ARGV' "$@"
  fi
}
run_case() { # id dir command
  local id="$1" src="$2" cmd="$3"
  local tmp abs
  tmp=$(mktemp -d)
  abs=$(cd "$src" && pwd)
  # Copy with .orig restore semantics; never ship answer files.
  (cd "$src" && /usr/bin/find . -type f ! -name '*.orig' | while read -r f; do
     mkdir -p "$tmp/$(dirname "$f")"
     if [ -f "$abs/$f.orig" ]; then cp "$abs/$f.orig" "$tmp/$f"; else cp "$abs/$f" "$tmp/$f"; fi
   done)
  (cd "$tmp" && with_watchdog sh -c "$cmd" >/dev/null 2>&1)
  local code=$?
  rm -rf "$tmp"
  # A watchdog kill (124 / SIGALRM) still counts as red for hang-type bugs.
  if [ "$code" -eq 0 ]; then
    echo "GREEN|$id|$src|$cmd|"
  else
    echo "RED|$id|$src|$cmd|exit=$code"
  fi
}

ALL_RED=1
{
  echo "| Case | Sample | Command | State |"
  echo "| :--- | :--- | :--- | :--- |"
  while IFS='|' read -r state id src cmd rest; do
    if [ "$state" = "RED" ]; then
      echo "| $id | \`$src\` | \`$cmd\` | RED as shipped ($rest) |"
    else
      echo "| $id | \`$src\` | \`$cmd\` | GREEN — CORRUPTED (answer shipped?) |"
      ALL_RED=0
    fi
  done
} < <(
  run_case AHB-01 samples/fastapi_async_deadlock    "$PYTEST -q"
  run_case AHB-02 samples/hard_concurrency_cascade  "$PYTEST -q"
  run_case AHB-03 samples/pydantic_v2_migration     "$PYTEST -q"
  run_case AHB-04 samples/sql_injection_remediation "$PYTEST -q"
  run_case AHB-05 samples/go_concurrency_race       "go test -race ."
  run_case AHB-06 samples/external_go_diff          "go test ./diffmatchpatch/ -run TestDiffLinesToChars"
  run_case AHB-07 samples/external_go_toml          "go test . -run TestUnmarshalRecursiveEmbedded -count=1"
) > /tmp/red_matrix_table.md

mkdir -p docs
{
  echo "# Benchmark RED-State Integrity Matrix"
  echo
  echo "Regenerate: \`bash scripts/verify_red.sh\` (needs go, python3+pytest, pydantic)."
  echo "Every case must FAIL its own test command as shipped — a green case means"
  echo "a healed/answer file leaked into the benchmark. CI enforces this on every push."
  echo
  echo "- Verified: $(date -u +"%Y-%m-%dT%H:%M:%SZ"), commit \`$(git rev-parse --short HEAD)\`"
  echo
  cat /tmp/red_matrix_table.md
} > docs/BENCHMARK_RED_MATRIX.md

cat docs/BENCHMARK_RED_MATRIX.md
[ "$ALL_RED" -eq 1 ] || { echo "BENCHMARK CORRUPTION: at least one case is GREEN"; exit 1; }
echo "All cases RED — benchmark integrity verified."
