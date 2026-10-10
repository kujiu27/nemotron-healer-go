# Performance Receipts (offline-verifiable claims)

Regenerate with `make bench` (no API keys needed). Committed so every number
in the README table has a reproducible source.

| Metric | Value | Method |
| :--- | :--- | :--- |
| Stripped static binary size | 8.0 MB | `go build -trimpath -ldflags="-s -w"`, `stat` |
| Peak RSS (`version` cmd) | 11.9 MB avg over 5 runs | `/usr/bin/time -l` (samples: 12271616 12451840 12435456 12533760 12468224) |
| Process wall time (`version` cmd) |  0.00 0.00 0.00 0.00 0.00 s over 5 runs | `/usr/bin/time -p` (10ms resolution) |

- Measured: 2026-10-10T08:34:29Z on Darwin/arm64, commit `0bf65ca`
