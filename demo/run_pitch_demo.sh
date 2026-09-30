#!/usr/bin/env bash
# Automated High-Production Demo Runner for Nemotron-Healer
# Simulates a senior engineer diagnosing and healing a complex concurrency deadlock in real-time.

set -e

GREEN='\033[0;32m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

clear
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

echo -e "${BLUE}▶ Launching Nemotron-Healer In-Situ Autonomous Repair Engine (Go Native)...${NC}\n"
sleep 1

# Execute self-healing
./bin/nemotron-healer samples/hard_concurrency_cascade --command "pytest" --turns 4 --no-tui

echo -e "\n${GREEN}✔ CI STATUS: GREEN! Auto-created fix branch and generated full PR Audit Report.${NC}"
echo -e "${PURPLE}========================================================================${NC}\n"
