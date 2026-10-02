#!/usr/bin/env bash
# Measures the offline-verifiable performance claims from the README table and
# writes/refreshes the receipt at docs/PERF.md. No API keys required.
# Method: stripped static binary (-s -w), /usr/bin/time -l (RSS) and -p (wall),
# 5 runs each, darwin/arm64 unless run elsewhere.
set -euo pipefail
cd "$(dirname "$0")/.."

BIN=$(mktemp -d)/nemotron-healer
trap 'rm -rf "$(dirname "$BIN")"' EXIT
go build -trimpath -ldflags="-s -w" -o "$BIN" ./cmd/nemotron-healer

SIZE=$(stat -f%z "$BIN" 2>/dev/null || stat -c%s "$BIN")
SIZE_MB=$(python3 -c "print(f'{$SIZE/1048576:.1f}')")

RSS_LIST=""
for _ in 1 2 3 4 5; do
  /usr/bin/time -l "$BIN" version >/dev/null 2>/tmp/nh_bench_t.txt || true
  RSS=$(grep 'maximum resident set size' /tmp/nh_bench_t.txt | awk '{print $1}')
  RSS_LIST="$RSS_LIST $RSS"
done
RSS_AVG_MB=$(python3 -c "vs='''$RSS_LIST'''.split(); print(f'{sum(map(int,vs))/len(vs)/1048576:.1f}')")

REAL_LIST=""
for _ in 1 2 3 4 5; do
  /usr/bin/time -p "$BIN" version >/dev/null 2>/tmp/nh_bench_t.txt || true
  REAL=$(grep '^real' /tmp/nh_bench_t.txt | awk '{print $2}')
  REAL_LIST="$REAL_LIST $REAL"
done

STAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
COMMIT=$(git rev-parse --short HEAD)
OSARCH="$(uname -s)/$(uname -m)"

mkdir -p docs
cat > docs/PERF.md <<EOF
# Performance Receipts (offline-verifiable claims)

Regenerate with \`make bench\` (no API keys needed). Committed so every number
in the README table has a reproducible source.

| Metric | Value | Method |
| :--- | :--- | :--- |
| Stripped static binary size | ${SIZE_MB} MB | \`go build -trimpath -ldflags="-s -w"\`, \`stat\` |
| Peak RSS (\`version\` cmd) | ${RSS_AVG_MB} MB avg over 5 runs | \`/usr/bin/time -l\` (samples:${RSS_LIST}) |
| Process wall time (\`version\` cmd) | ${REAL_LIST} s over 5 runs | \`/usr/bin/time -p\` (10ms resolution) |

- Measured: ${STAMP} on ${OSARCH}, commit \`${COMMIT}\`
EOF

echo "Wrote docs/PERF.md:"
cat docs/PERF.md
