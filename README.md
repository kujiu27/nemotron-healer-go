# ⚡ Nemotron-Healer (Go Edition)

> **Autonomous Code Self-Healing & Diagnostic Agent**  
> Powered by **NVIDIA Nemotron 3 Ultra on Nebius Token Factory** & **Tavily Search API**.  
> Built as a single, zero-dependency, ultra-fast static binary in **Go**.

[![Go Version](https://img.shields.io/badge/go-1.26+-00ADD8.svg)](https://golang.org)
[![Nebius Token Factory](https://img.shields.io/badge/Inference-Nebius%20Token%20Factory-7D56F4.svg)](https://tokenfactory.nebius.com)
[![Tavily Search](https://img.shields.io/badge/Grounding-Tavily%20Search%20API-00E599.svg)](https://tavily.com)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

---

## 🎯 What is Nemotron-Healer?

When CI tests fail or breaking changes break your codebase, developers waste hours reading outdated StackOverflow answers.  
**Nemotron-Healer** is an in-situ autonomous agent that runs directly in your terminal or GitHub Actions:

1. **Reproduction (Red State)**: Executes the failing test command and isolates the precise execution trace.
2. **AST Blast Radius Analysis**: Statically maps the codebase symbol graph and evaluates the downstream blast radius across files.
3. **Dynamic Knowledge Grounding**: Queries **Tavily Search API** for up-to-the-minute official migration guides and documentation, bypassing LLM knowledge cutoffs.
4. **Surgical Patch Synthesis**: Prompts **NVIDIA Nemotron 3 Ultra** on **Nebius Token Factory** for minimal, atomic Unified Diff patches.
5. **Transactional Sandbox & Adversarial Falsification**: Applies patches with atomic rollback safeguards, verifies test passes, and generates adversarial edge-case tests to eliminate AI patch overfitting.
6. **Git PR Automation**: Commits verified patches directly into a dedicated fix branch (`fix/nemotron-heal-xxx`).

---

## ⚡ Performance: Python vs Go Architecture

| Metric | Legacy Python Approach | **Nemotron-Healer (Go)** |
| :--- | :--- | :--- |
| **Cold Start Latency** | ~450ms (interpreter + imports) | **< 5ms (Instant native wake)** |
| **Memory Footprint** | ~65MB | **~14MB** |
| **Distribution** | Requires Python 3.11+, pip, venv | **Single Static Binary (6.9MB)** |
| **CI Setup Time** | 30s ~ 60s (`setup-python`, `pip install`) | **< 1s (`wget` release binary)** |
| **Terminal UX** | Basic text logs | **Cyberpunk TUI (`bubbletea` + `lipgloss`)** |

---

## 🚀 Quick Start

### 1. Download Binary
Download the pre-compiled binary for your architecture from [Releases](https://github.com/nemotron-healer/nemotron-healer-go/releases):

```bash
# macOS Apple Silicon (M1/M2/M3/M4)
curl -fsSL https://github.com/nemotron-healer/nemotron-healer-go/releases/download/v0.2.0/nemotron-healer_darwin_arm64 -o /usr/local/bin/nemotron-healer
chmod +x /usr/local/bin/nemotron-healer

# Linux x86_64
curl -fsSL https://github.com/nemotron-healer/nemotron-healer-go/releases/download/v0.2.0/nemotron-healer_linux_amd64 -o /usr/local/bin/nemotron-healer
chmod +x /usr/local/bin/nemotron-healer
```

### 2. Verify Environment
```bash
nemotron-healer doctor
```

### 3. Heal a Broken Project
Navigate to any failing repository and run:
```bash
# In-situ healing using pytest
nemotron-healer . --command "pytest"

# Or in non-interactive CI mode
nemotron-healer . --command "go test ./..." --no-tui
```

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
- **TTFT (Time To First Token)**: Sub-second streaming responsiveness.
- **Measured TPS (Tokens Per Second)**: Real-time throughput.
- **Cost Reduction**: Over 99.8% dollar savings compared to human engineering triage.

---

## 📄 License
MIT License. Built for the **Nebius x NVIDIA Global AI Hackathon 2026**.
