# 🎬 120-Second High-Production Video Demo Script

> **Target Duration:** 120 seconds (2:00)  
> **Speaker Persona:** Senior Staff AI Systems Architect (Direct, precise, authoritative)  
> **Video Requirements on Devpost:** < 3 minutes, showing working software.

---

### [0:00 - 0:20] The Problem: Why Current AI Coding Fails
- **Visual:** Terminal split screen showing a messy, multi-file codebase. Running `pytest` outputs red text with race conditions and deadlocks.
- **Voiceover:**  
  *"Modern engineering teams spend over 40% of their time fixing broken builds, tricky concurrency deadlocks, and library migrations. But existing AI assistants operate in a fragile open loop: they hallucinate deprecated methods, break downstream files, and overfit on single test assertions without verification."*

---

### [0:20 - 0:45] The Solution: In-Situ Autonomous Self-Healing
- **Visual:** Terminal clears. Command typed: `./bin/nemotron-healer samples/hard_concurrency_cascade`.
- **Voiceover:**  
  *"Meet Nemotron-Healer: an in-situ autonomous code repair agent written in pure Go. It boots in less than 5 milliseconds as a single static binary with zero runtime dependencies. When a test breaks, Nemotron-Healer intercepts the failure trace, snapshots the workspace, and maps the AST Blast Radius across the entire repository before writing a single line of code."*

---

### [0:45 - 1:15] The Breakthrough: Hybrid Archetypes & Dynamic Grounding
- **Visual:** Terminal highlights `[DIAGNOSING] Archetype Rule Engine: Classified as [ConcurrencyRace]`. Then shows `[SEARCHING_KNOWLEDGE] Searching Tavily...` and streaming Nemotron 3 Ultra output.
- **Voiceover:**  
  *"Unlike naive wrappers, Nemotron-Healer uses a deterministic archetype rule engine: it classifies defects into deterministic archetypes—injecting rigid negative constraints into the prompt. To bypass training cutoffs, it queries the Tavily Search API in real-time for official breaking change migration specs, then unleashes NVIDIA Nemotron 3 Ultra on Nebius Token Factory to synthesize surgical Unified Diffs."*

---

### [1:15 - 1:40] Mathematical Robustness: Anti-Overfitting Arena
- **Visual:** Terminal shows `[VERIFYING_SANDBOX]` and `[ADVERSARIAL FALSIFICATION]`, followed by green pass `[SUCCEEDED]`.
- **Voiceover:**  
  *"A green test isn't enough. Our engine launches an Adversarial Falsification game—synthesizing hostile edge-case counter-examples to stress-test the patch. If the patch overfits or causes regressions, it rolls back atomically. Once proven robust, it automatically cuts a Git PR branch and commits a full enterprise audit card."*

---

### [1:40 - 2:00] Economic Telemetry & Closing
- **Visual:** Zoom in on the Economic & Token Consumption Ledger table showing measured TTFT, TPS, and per-session run cost computed from streamed token usage. Cut to `action.yml`.
- **Voiceover:**  
  *"On Nebius Token Factory, streaming Nemotron 3 Ultra prices a full healing session at cents — computed from the exact streamed token count at catalog prices, printed in every audit card. Packaged as a single static binary and a one-line GitHub Action, Nemotron-Healer turns unlimited compute into autonomous engineering excellence. Thank you."*
