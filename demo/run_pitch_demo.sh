#!/usr/bin/env bash
# Automated High-Production Demo Runner for Nemotron-Healer
# Simulates a senior engineer diagnosing and healing a complex concurrency deadlock in real-time.

set -eo pipefail

GREEN='\033[0;32m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

clear 2>/dev/null || true
echo -e "${PURPLE}========================================================================${NC}"
echo -e "${BLUE}  ⚡ NEMOTRON-HEALER: Autonomous Code Self-Healing & Verification Agent${NC}"
echo -e "${PURPLE}  Powered by NVIDIA Nemotron on Nebius Token Factory & Tavily Search API${NC}"
echo -e "${PURPLE}========================================================================${NC}\n"
sleep 1
MOCK_ARG=""
if [ "${1:-}" = "--mock" ] || [ -z "${NEBIUS_API_KEY:-}" ] || [ -z "${TAVILY_API_KEY:-}" ]; then
  MOCK_ARG="--mock"
  echo -e "${YELLOW}⚡ Offline Demo Mode: Running with built-in labeled mock (zero API keys required).${NC}\n"
fi

# Probe whether pytest is runnable in this environment
PYTEST_CMD=""
if command -v pytest >/dev/null 2>&1 && pytest --version >/dev/null 2>&1; then
  PYTEST_CMD="pytest"
elif python3 -m pytest --version >/dev/null 2>&1; then
  PYTEST_CMD="python3 -m pytest"
fi

DEMO_DIR=$(mktemp -d /tmp/nemotron-demo-XXXX)
trap 'rm -rf "$DEMO_DIR"' EXIT

HEAL_CMD=""
if [ -n "$PYTEST_CMD" ]; then
  echo -e "${YELLOW}[SCENARIO] Production Incident: CI Test Suite Failed on Multi-Module Service!${NC}"
  echo -e "Executing test command: $PYTEST_CMD test_cascade.py (isolated workspace copy)\n"
  sleep 1

  abs_src=$(cd samples/hard_concurrency_cascade && pwd)
  (cd samples/hard_concurrency_cascade && /usr/bin/find . -type f ! -name '*.orig' | while read -r f; do
     mkdir -p "$DEMO_DIR/$(dirname "$f")"
     if [ -f "$abs_src/$f.orig" ]; then cp "$abs_src/$f.orig" "$DEMO_DIR/$f"; else cp "$abs_src/$f" "$DEMO_DIR/$f"; fi
   done)
  cd "$DEMO_DIR"

  # Show the red failure
  $PYTEST_CMD test_cascade.py || true
  echo -e "\n${RED}✘ FATAL: 2 failed, 1 passed. Concurrency race condition & re-entrancy deadlock detected!${NC}\n"
  HEAL_CMD="$PYTEST_CMD"
else
  echo -e "${YELLOW}[SCENARIO] Zero-Dependency Pure-Go Mode: Real Upstream Bug in tidwall/gjson (#246)!${NC}"
  echo -e "Executing test command: go test . -run TestEmptyValueQuery -count=1 (isolated workspace copy)\n"
  sleep 1

  cp -R samples/external_gjson/. "$DEMO_DIR/"
  cd "$DEMO_DIR"

  # Show the red failure
  go test . -run TestEmptyValueQuery -count=1 || true
  echo -e "\n${RED}✘ FATAL: test failed — array-path parser drops empty quoted string!${NC}\n"
  HEAL_CMD="go test . -run TestEmptyValueQuery -count=1"
fi

sleep 1
echo -e "${BLUE}▶ Launching Nemotron-Healer In-Situ Autonomous Repair Engine (Go Native)...${NC}"
sleep 1

# Build fresh binary from source if missing (bin/ is gitignored)
HEALER="$OLDPWD/bin/nemotron-healer"
REPO_ROOT="$OLDPWD"
if [ ! -x "$HEALER" ]; then
  echo -e "${YELLOW}[SETUP] Building the healer from source...${NC}"
  (cd "$REPO_ROOT" && go build -o bin/nemotron-healer ./cmd/nemotron-healer) || { echo -e "${RED}✘ Build failed. Is Go 1.26+ installed?${NC}"; exit 1; }
fi

# Execute self-healing in the isolated copy (abort on failure)
"$HEALER" . --command "$HEAL_CMD" --turns 3 --no-tui $MOCK_ARG || {
  echo -e "\n${RED}✘ Healing run FAILED — demo aborted honestly. See diagnostics above.${NC}"
  exit 1
}
echo -e "\n${GREEN}✔ CI STATUS: GREEN! Auto-created fix branch and generated full PR Audit Report.${NC}"
echo -e "${PURPLE}========================================================================${NC}"
