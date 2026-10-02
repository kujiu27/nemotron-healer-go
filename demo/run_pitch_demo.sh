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

echo -e "${YELLOW}[SCENARIO] Production Incident: CI Test Suite Failed on Multi-Module Service!${NC}"
echo -e "Executing test command: pytest samples/hard_concurrency_cascade/test_cascade.py\n"
sleep 1

# Reset broken state
cp samples/hard_concurrency_cascade/engine.py.orig samples/hard_concurrency_cascade/engine.py

# Show the red failure
python3 -m pytest samples/hard_concurrency_cascade/test_cascade.py || true
echo -e "\n${RED}✘ FATAL: 2 failed, 1 passed. Concurrency race condition & re-entrancy deadlock detected!${NC}\n"
sleep 2

echo -e "${BLUE}▶ Launching Nemotron-Healer In-Situ Autonomous Repair Engine (Go Native)...${NC}"
sleep 1

# Build fresh binary from source if missing (bin/ is gitignored)
if [ ! -x ./bin/nemotron-healer ]; then
  echo -e "${YELLOW}[SETUP] Building ./bin/nemotron-healer from source...${NC}"
  go build -o ./bin/nemotron-healer ./cmd/nemotron-healer || { echo -e "${RED}✘ Build failed. Is Go 1.26+ installed?${NC}"; exit 1; }
fi

if [ -z "${NEBIUS_API_KEY}" ] || [ -z "${TAVILY_API_KEY}" ]; then
  echo -e "${RED}✘ NEBIUS_API_KEY and TAVILY_API_KEY must be exported for a real healing run.${NC}"
  exit 1
fi

# Execute self-healing (real run; abort on failure instead of faking GREEN)
./bin/nemotron-healer samples/hard_concurrency_cascade --command "pytest" --turns 4 --no-tui || {
  echo -e "\n${RED}✘ Healing run FAILED — demo aborted honestly. See diagnostics above.${NC}"
  exit 1
}

echo -e "\n${GREEN}✔ CI STATUS: GREEN! Auto-created fix branch and generated full PR Audit Report.${NC}"
echo -e "${PURPLE}========================================================================${NC}"
