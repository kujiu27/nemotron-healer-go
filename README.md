# ⚡ Nemotron-Healer (Go Edition)

> **Autonomous Code Self-Healing & Diagnostic Agent**  
> Powered by **NVIDIA Nemotron 3 Ultra on Nebius Token Factory** & **Tavily Search API**.  
> Built as a single, zero-dependency, ultra-fast static binary in **Go**.

[![Go Version](https://img.shields.io/badge/go-1.26+-00ADD8.svg)](https://golang.org)
[![Nebius Token Factory](https://img.shields.io/badge/Inference-Nebius%20Token%20Factory-7D56F4.svg)](https://tokenfactory.nebius.com)
[![Tavily Search](https://img.shields.io/badge/Grounding-Tavily%20Search%20API-00E599.svg)](https://tavily.com)
[![CI](https://github.com/kujiu27/nemotron-healer-go/actions/workflows/ci.yml/badge.svg)](https://github.com/kujiu27/nemotron-healer-go/actions)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

---

## 🎯 What is Nemotron-Healer?

When CI tests fail or breaking changes break your codebase, developers waste hours reading outdated StackOverflow answers.  
**Nemotron-Healer** is an in-situ autonomous agent that runs directly in your terminal or GitHub Actions — closed-loop where generic coding agents are open-loop: a patch is committed only after an adversarial counter-example FAILS to break it:

1. **Reproduction (Red State)**: Executes the failing test command and isolates the precise execution trace.
2. **Symbol Graph Blast Radius Analysis**: Maps the codebase symbol graph (Go via `go/parser` AST; Python/TS/JS via line grammars) and scores downstream blast radius across files.
3. **Dynamic Knowledge Grounding**: Queries **Tavily Search API** for up-to-the-minute official migration guides and documentation, then pulls full text of the top hit via **Tavily Extract**; the Nemotron Nano triage tier synthesizes the search query from the actual traceback. Bypasses LLM knowledge cutoffs.
4. **Surgical Patch Synthesis**: Prompts **NVIDIA Nemotron 3 Ultra** on **Nebius Token Factory** for minimal, atomic Unified Diff patches.
5. **Transactional Sandbox & Adversarial Falsification**: Applies patches with atomic rollback safeguards, verifies test passes, and generates adversarial edge-case tests to eliminate AI patch overfitting.
6. **Git PR Automation**: Commits verified patches directly into a dedicated fix branch (`fix/nemotron-heal-xxx`).

> **Execution model & trust boundary:** the agent runs your test command as-is inside the workspace
> (plain `sh -c`, no container isolation) and executes model-generated patches and adversarial tests.
> Rollback uses file snapshots plus a manifest so created files are removed exactly. Run it on CI
> runners or disposable worktrees — not on machines holding production secrets.

## 🔍 Evidence & Verification (everything checkable, nothing on faith)

| Claim | Receipt |
| :--- | :--- |
| Performance (8.0MB binary, ~12MB RSS, <10ms start) | [`docs/PERF.md`](docs/PERF.md) — regenerate: `make bench` |
| All 9 benchmark cases ship genuinely RED (no leaked answers) | [`docs/BENCHMARK_RED_MATRIX.md`](docs/BENCHMARK_RED_MATRIX.md) — regenerate: `bash scripts/verify_red.sh`; enforced by CI on every push |
| Real upstream bugs, not self-authored cases | [`samples/external_go_diff/README.md`](samples/external_go_diff/README.md) (sergi/go-diff `6dbe13c`) · [`samples/external_go_toml/README.md`](samples/external_go_toml/README.md) (pelletier/go-toml `6fa69af`) · [`samples/external_gjson/README.md`](samples/external_gjson/README.md) (tidwall/gjson `0b52f9a`) · [`samples/external_jwt/README.md`](samples/external_jwt/README.md) (golang-jwt/jwt `1a11d37`) — red/green proofs included |
| Adversarial self-audit with every finding fixed | [`GRAND_PRIZE_GAP_AUDIT.md`](GRAND_PRIZE_GAP_AUDIT.md) — remediation ledger maps each finding to its fix commit |
| GitHub Action works on a real runner | `action-smoke` job in [CI](.github/workflows/ci.yml) runs the composite action end-to-end on every push |
| Release binaries are live | [Releases](https://github.com/kujiu27/nemotron-healer-go/releases) — auto-published on `v*` tag pushes |
| Benchmark statistics with variance | `nemotron-healer eval --repeat N` — per-case solve rates + mean±std |
| Full pipeline runs end-to-end without API keys | `nemotron-healer --mock` (or `make eval-mock`) — built-in labeled mock endpoints (`X-Nemotron-Healer: MOCK`), pure Go, zero python or API keys |

---

## ⚡ Performance: Python vs Go Architecture

| Metric | Legacy Python Approach | **Nemotron-Healer (Go)** |
| :--- | :--- | :--- |
| **Hot Start Latency** | ~450ms (interpreter + imports) | **< 10ms measured** (`/usr/bin/time -p`, 5 runs — [receipt](docs/PERF.md)) |
| **Memory Footprint** | ~65MB | **~12MB RSS measured** (5-run avg — [receipt](docs/PERF.md)) |
| **Distribution** | Requires Python 3.11+, pip, venv | **Single Static Binary (8.0MB stripped — [receipt](docs/PERF.md))** |
| **CI Setup Time** | 30s ~ 60s (`setup-python`, `pip install`) | **~15s (`actions/setup-go` + `go build`, no releases needed)** |
| **Terminal UX** | Basic text logs | **Cyberpunk TUI (`bubbletea` + `lipgloss`)** |

---

### 1. Install
Zero-404 install paths — build from source, no release binaries required:

```bash
# One-liner (requires Go 1.26+)
go install github.com/kujiu27/nemotron-healer-go/cmd/nemotron-healer@latest

# Or from a clone
git clone https://github.com/kujiu27/nemotron-healer-go.git
cd nemotron-healer-go && make build   # produces ./bin/nemotron-healer
```

Pre-compiled binaries are attached to [Releases](https://github.com/kujiu27/nemotron-healer-go/releases)
(auto-published by CI on every `v*` tag push). The Windows build is provided
for use inside a POSIX shell (Git Bash / WSL); the healer executes test
commands via `sh -c`.
### 0. 60-Second Demo (Zero API keys required)
```bash
make demo   # or: bash demo/run_pitch_demo.sh
```
Auto-detects environment: runs live cloud heal if keys are exported, or launches the built-in mock server offline if keys are absent. Auto-detects Python pytest vs pure Go.


### 2. Verify Environment
```bash
nemotron-healer doctor          # verifies Token Factory & Tavily (fail-closed)
nemotron-healer doctor --mock   # verifies built-in mock environment offline
```

### 3. Heal a Broken Project
Navigate to any failing repository and run:
```bash
# In-situ healing using pytest
nemotron-healer . --command "pytest"

# Or in non-interactive CI mode
nemotron-healer . --command "go test ./..." --no-tui

# Or offline with zero API keys via built-in mock
nemotron-healer samples/external_gjson --mock --command "go test . -run TestEmptyValueQuery -count=1" --no-tui

### 4. Use as a GitHub Action
```yaml
- name: Autonomous Self-Healing CI
  uses: kujiu27/nemotron-healer-go@v0.7.34
  with:
    path: .                    # directory to heal (default: repo root)
    command: go test ./...     # the failing test command
    turns: '5'
    enable_arena: 'true'
```
Requires repo secrets `NEBIUS_API_KEY` (tokenfactory.nebius.com) and
`TAVILY_API_KEY` (tavily.com). The composite action builds the healer from
source — no release dependency — and comments the full audit card on the PR.
This exact action chain is smoke-tested on a real GitHub runner on every push
(see the `action-smoke` job in CI).

---

## 🛠 Architecture & State Machine

```
               [Failing CI / Terminal]
                         │
                 `nemotron-healer`
                         │
        ┌────────────────┴────────────────┐
        ▼                                 ▼
   [AST CodeGraph]               [Reproduction Test]
   - Symbol extraction            - Capture exit code & trace
   - Blast Radius computation
        │                                 │
        └────────────────┬────────────────┘
                         ▼
             [Tavily Search Grounding]
             - Official docs & breaking changes
                         ▼
             [Nebius Token Factory]
             - NVIDIA Nemotron 3 Ultra
             - Surgical Unified Diff
                         ▼
             [Transactional Sandbox]
             - Atomic snapshot & rollback
             - Verify: Red ➔ Green
                         ▼
             [Adversarial Falsification]
             - Stress test edge cases
                         ▼
             [Git PR Branch Created]
```

---

## 📊 Economic & Token Consumption Ledger

At the end of every healing session, `nemotron-healer` prints an auditable telemetry ledger:
- **Prompt / Completion Tokens**: Exact count streamed via Nebius Token Factory.
- **TTFT (Time To First Token)**: Measured and reported per session.
- **Measured TPS (Tokens Per Second)**: Real-time throughput.
- **Cost Reduction**: Per-session run cost computed from the exact streamed token usage at Token Factory catalog prices ($1.00/$3.00 per 1M); savings vs an assumed $25 human-triage baseline are printed per session.

---

## 📄 License
MIT License. Built for the **Nebius x NVIDIA Global AI Hackathon 2026**.
