#!/usr/bin/env bash
# Runs full heal loops against the labeled mock endpoints — zero API keys.
# Three modes: plain, --search (parallel DHS), --arena (Red-Blue self-play).
# Demonstrates wiring only: mock responses are marked X-Nemotron-Healer: MOCK
# and no receipts are written.
set -uo pipefail
REPO=$(cd "$(dirname "$0")/.." && pwd)
cd "$REPO"

PORT=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')

go build -o bin/nemotron-healer ./cmd/nemotron-healer

python3 scripts/mock_nebius.py "$PORT" &
MOCK_PID=$!
trap 'kill $MOCK_PID 2>/dev/null || true' EXIT
for _ in $(seq 1 30); do
  curl -s -o /dev/null "http://127.0.0.1:$PORT/" && break
  sleep 0.2
done

run_mode() { # label extra-flags...
  local label="$1"; shift
  echo "=== MOCK pipeline run [$label]: heal samples/external_gjson (AHB-08) ==="
  local tmp rc=0
  tmp=$(mktemp -d)
  cp -R samples/external_gjson/. "$tmp/"
  (
    cd "$tmp"
    NEBIUS_API_KEY=mock TAVILY_API_KEY=mock \
    NEBIUS_BASE_URL="http://127.0.0.1:$PORT/v1" \
    TAVILY_BASE_URL="http://127.0.0.1:$PORT" \
    "$REPO/bin/nemotron-healer" . --command "go test . -run TestEmptyValueQuery -count=1" --turns 2 --no-tui "$@"
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
    NEBIUS_API_KEY=mock TAVILY_API_KEY=mock \
    NEBIUS_BASE_URL="http://127.0.0.1:$PORT/v1" \
    TAVILY_BASE_URL="http://127.0.0.1:$PORT" \
    "$REPO/bin/nemotron-healer" . --command "go test . -run TestEmptyValueQuery -count=1" --turns 1 --no-tui --yes --json 2>/dev/null \
      | python3 -c "import json,sys; j=json.load(sys.stdin); print('pure JSON, resolved:', j['is_resolved'])"
  ) || rc=$?
  echo "=== [json-purity] exit code: $rc ==="
  return $rc
}

rc=0
run_mode plain || rc=$?
run_mode search --search || rc=$?
run_mode arena --arena || rc=$?
run_json_mode || rc=$?
exit $rc
