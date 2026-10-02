# Performance Receipts (offline-verifiable claims)

Regenerate with `make bench` (no API keys needed). Committed so every number
in the README table has a reproducible source.

| Metric | Value | Method |
| :--- | :--- | :--- |
| Stripped static binary size | 7.5 MB | `go build -trimpath -ldflags="-s -w"`, `stat` |
| Peak RSS (`version` cmd) | 11.8 MB avg over 5 runs | `/usr/bin/time -l` (samples: 12320768 12222464 12468224 12533760 12435456) |
| Process wall time (`version` cmd) |  0.00 0.00 0.00 0.00 0.00 s over 5 runs | `/usr/bin/time -p` (10ms resolution) |

- Measured: 2026-10-02T05:42:57Z on Darwin/arm64, commit `8505ad2`
