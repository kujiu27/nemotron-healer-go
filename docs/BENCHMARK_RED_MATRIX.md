# Benchmark RED-State Integrity Matrix

Regenerate: `bash scripts/verify_red.sh` (needs go, python3+pytest, pydantic).
Every case must FAIL its own test command as shipped — a green case means
a healed/answer file leaked into the benchmark. CI enforces this on every push.

- Verified: 2026-10-02T04:11:27Z, commit ``

| Case | Sample | Command | State |
| :--- | :--- | :--- | :--- |
| AHB-01 | `samples/fastapi_async_deadlock` | `pytest -q` | RED as shipped (exit=1) |
| AHB-02 | `samples/hard_concurrency_cascade` | `pytest -q` | RED as shipped (exit=1) |
| AHB-03 | `samples/pydantic_v2_migration` | `pytest -q` | RED as shipped (exit=1) |
| AHB-04 | `samples/sql_injection_remediation` | `pytest -q` | RED as shipped (exit=1) |
| AHB-05 | `samples/go_concurrency_race` | `go test -race .` | RED as shipped (exit=1) |
| AHB-06 | `samples/external_go_diff` | `go test ./diffmatchpatch/ -run TestDiffLinesToChars` | RED as shipped (exit=1) |
| AHB-07 | `samples/external_go_toml` | `go test . -run TestUnmarshalRecursiveEmbedded -count=1` | RED as shipped (exit=1) |
