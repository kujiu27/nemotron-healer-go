# Benchmark RED-State Integrity Matrix

Regenerate: `bash scripts/verify_red.sh` (needs go, python3+pytest, pydantic).
Every case must FAIL its own test command as shipped — a green case means
a healed/answer file leaked into the benchmark. CI enforces this on every push.

- Verified: 2026-10-10T02:40:57Z, commit `3122c2b`

| Case | Sample | Command | State |
| :--- | :--- | :--- | :--- |
| AHB-01 | `samples/fastapi_async_deadlock` | `python3 -m pytest -q` | RED as shipped (exit=1) |
| AHB-02 | `samples/hard_concurrency_cascade` | `python3 -m pytest -q` | RED as shipped (exit=1) |
| AHB-03 | `samples/pydantic_v2_migration` | `python3 -m pytest -q` | RED as shipped (exit=1) |
| AHB-04 | `samples/sql_injection_remediation` | `python3 -m pytest -q` | RED as shipped (exit=1) |
| AHB-05 | `samples/go_concurrency_race` | `go test -race .` | RED as shipped (exit=1) |
| AHB-06 | `samples/external_go_diff` | `go test ./diffmatchpatch/ -run TestDiffLinesToChars` | RED as shipped (exit=1) |
| AHB-07 | `samples/external_go_toml` | `ulimit -t 20; go test . -run TestUnmarshalRecursiveEmbedded -count=1 -timeout 30s` | RED as shipped (exit=1) |
| AHB-08 | `samples/external_gjson` | `go test . -run TestEmptyValueQuery -count=1` | RED as shipped (exit=1) |
| AHB-09 | `samples/external_jwt` | `go test . -run TestMapClaims_GetAudience -count=1` | RED as shipped (exit=1) |
