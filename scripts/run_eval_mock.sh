#!/usr/bin/env bash
# Runs one full heal loop against the labeled mock endpoints — zero API keys.
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

echo "=== MOCK pipeline run: heal samples/external_gjson (AHB-08) with labeled mock endpoints ==="
tmp=$(mktemp -d)
cp -R samples/external_gjson/. "$tmp/"
rc=0
(
  cd "$tmp"
  NEBIUS_API_KEY=mock TAVILY_API_KEY=mock \
  NEBIUS_BASE_URL="http://127.0.0.1:$PORT/v1" \
  TAVILY_BASE_URL="http://127.0.0.1:$PORT" \
  "$REPO/bin/nemotron-healer" . --command "go test . -run TestEmptyValueQuery -count=1" --turns 2 --no-tui
) || rc=$?
echo "=== mock heal exit code: $rc (workspace was $tmp) ==="
exit $rc
