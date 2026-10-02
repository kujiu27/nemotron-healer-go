# 🏛️ Architecture & System Design Specification: Nemotron-Healer

> **Document Version:** v1.0.0 (Release Candidate)  
> **Target System:** `nemotron-healer-go`  
> **Status:** APPROVED & IMPLEMENTED  
> **Author:** Core Engineering Team  
> **Reference Inspirations:** Published code-review heuristics, TypeSafe Jev Architecture

---

## 1. System Overview & Problem Statement

### 1.1 The Industry Bottleneck
Standard Large Language Model (LLM) coding assistants operate in an **unverified, open-loop paradigm**:
1. **Context Blindness**: Models fixate on the failing stack trace line without calculating the global downstream **Blast Radius** across the repository, causing cascading regressions.
2. **Knowledge Cutoff**: Outdated training data leads models to suggest deprecated methods or hallucinated interfaces when modern library versions introduce breaking changes.
3. **Overfitting & Sycophancy**: Models often "game" unit tests by hardcoding return values, mocking critical invariants, or deleting assertions.
4. **Distribution Friction**: Most agentic prototypes require complex Python runtimes, bloated dependency trees, and slow startup times (>400ms), making them unusable in lightweight CI/CD runners.

### 1.2 The Design Mandate
`nemotron-healer-go` is designed as a **zero-dependency, single-binary, in-situ autonomous code self-healing engine** built in Go:
- **Instant Cold Start**: `< 5ms` binary startup time;
- **Deterministic Pre-Filtering**: Rule-based defect archetype classification before LLM invocation;
- **AST Dependency Graphing**: Quantitative Blast Radius risk scoring;
- **Dynamic Knowledge Grounding**: Tavily Search integration for real-time upstream migration specs;
- **Transactional Rollback**: Zero repository contamination through snapshot checkpoints;
- **Adversarial Falsification**: Mathematical anti-overfitting proof via synthesized stress-test counter-examples.

---

## 2. High-Level System Architecture & Component Topology

```
                                  [Failing Build / CI / Terminal]
                                                │
                                                ▼
                                    ┌───────────────────────┐
                                    │ Cobra CLI Engine &    │
                                    │ Signal Interceptor    │
                                    └───────────┬───────────┘
                                                │
                 ┌──────────────────────────────┴──────────────────────────────┐
                 ▼                                                             ▼
     ┌───────────────────────┐                                     ┌───────────────────────┐
     │ Symbol Graph Engine   │                                     │ Test Runner (in-situ) │
     │ - Symbol Extraction   │                                     │ - Workspace Execution │
     │ - Call Graph Inversion│                                     │ - Output / Trace Parse│
     │ - Blast Radius Calc   │                                     │ - Exit Code Isolation │
     └───────────┬───────────┘                                     └───────────┬───────────┘
                 │                                                             │
                 └──────────────────────────────┬──────────────────────────────┘
                                                │
                                                ▼
                             ┌─────────────────────────────────────┐
                             │ Deterministic Defect Classifier     │
                             │ (Archetype Rule Engine Pipeline)    │
                             └──────────────────┬──────────────────┘
                                                │
                                                ▼
                             ┌─────────────────────────────────────┐
                             │ Dynamic Knowledge Grounding (Tavily)│
                             │ - Breaking change retrieval         │
                             │ - Official migration doc ingestion  │
                             └──────────────────┬──────────────────┘
                                                │
                                                ▼
                             ┌─────────────────────────────────────┐
                             │ NVIDIA Nemotron 3 Ultra (Nebius)    │
                             │ - High-throughput Token Factory     │
                             │ - Surgical Unified Diff Generation  │
                             └──────────────────┬──────────────────┘
                                                │
                                                ▼
                             ┌─────────────────────────────────────┐
                             │ Transactional Sandbox & Patcher     │
                             │ - Checkpoint Snapshot Isolation     │
                             │ - 3-Tier Multi-Strategy Applicator  │
                             │ - Green Pass Verification           │
                             └──────────────────┬──────────────────┘
                                                │
                                                ▼
                             ┌─────────────────────────────────────┐
                             │ Adversarial Falsification Engine    │
                             │ - Edge-case stress test synthesis   │
                             │ - Zero-overfitting proof            │
                             └──────────────────┬──────────────────┘
                                                │
                                                ▼
                             ┌─────────────────────────────────────┐
                             │ Git PR Automation & Audit Card      │
                             │ - Dedicated fix branch creation     │
                             │ - Full telemetry & Token Ledger     │
                             └─────────────────────────────────────┘
```

---

## 3. Core Subsystems Detailed Specification

### 3.1 Deterministic Defect Archetype Classifier (Rule Engine)
Rather than passing raw, unstructured error traces to the model, the hybrid classifier (`internal/engine/archetype.go`) categorizes the failure into a formal **Defect Archetype** and binds **Mandatory Negative Constraints**:

| Archetype | Detection Signatures | Injected Engineering Constraints |
| :--- | :--- | :--- |
| **`ConcurrencyRace`** | `AssertionError`, `race`, `corrupted`, `asyncio.gather` | • MUST use RAII synchronization (e.g. `asyncio.Lock`, `sync.Mutex`).<br>• DO NOT remove concurrent tasks to fake a pass. |
| **`InterfaceBreaking`** | `deprecated`, `removed in`, `ValidationError`, `renamed` | • MUST conform to Tavily-retrieved modern specs.<br>• DO NOT suppress deprecation warnings. |
| **`SecurityDefect`** | `injection`, `syntax error at or near`, unescaped queries | • MUST enforce parameterized queries / prepared statements.<br>• NEVER use raw string formatting. |
| **`ResourceLeak`** | `ResourceWarning`, `unclosed`, `connection pool exhausted` | • MUST enforce context managers or defer cleanup blocks. |
| **`NullTypeError`** | `NoneType`, `nil pointer dereference`, `AttributeError` | • MUST introduce defensive guard clauses with safe defaults. |

### 3.2 Symbol Graph & Quantitative Blast Radius
The graph engine (`internal/ast/blast_radius.go`) scans workspace source files (`.py`, `.go`, `.ts`, `.js`)
to extract symbols and caller→callee reference edges: Go files are parsed with the standard `go/parser`
AST; Python/TS/JS use per-language line grammars. It builds a directed graph $G = (V, E)$ where:
- $V$: Extracted class definitions and functions.
- $E$: Caller $\to$ Callee dependency edges.

The **Blast Radius Risk Score ($R$)** is formulated as:
$$R = \min\left(1.0, \; \frac{|\text{TransitiveDependents}(s)| + |\text{AffectedFiles}(s)|}{0.5 \times |V|}\right)$$
Where $s$ is the target symbol undergoing mutation. When $R > 0.7$, a `CRITICAL BLAST CONSTRAINT`
is injected into the patch-synthesis prompt demanding non-signature-breaking repairs; $R$ is also
penalized in the search reward function (weight 0.10), biasing selection toward smaller-blast-radius fixes.

### 3.3 Dynamic Knowledge Grounding (Tavily API)
When dealing with `InterfaceBreaking` or external library failures, relying on static LLM weights guarantees hallucinations. The agent:
1. Derives an enriched semantic search query: `fix <target_file> <archetype> <error_summary>`;
2. Executes a real-time query against the Tavily Search API (`internal/client/tavily.go`);
3. Ingests raw markdown context and formats it into the prompt under `[OFFICIAL DOCUMENTATION (TAVILY GROUNDING)]`.

### 3.4 Transactional Sandbox & Multi-Tier Patcher
To ensure zero workspace corruption, modifications follow an atomic transaction protocol (`internal/sandbox/`):
1. **Checkpoint Phase**: Snapshot uncommitted working directory state into an isolated temporary store.
2. **Application Phase**: Execute resilient 3-tier patch application:
   - **Tier 1 (`git apply`)**: Tries `-p1` and `-p0` with `--whitespace=fix`;
   - **Tier 2 (`patch`)**: Falls back to POSIX patch binary;
   - **Tier 3 (Fuzzy Hunk Replacer)**: Pure Go in-memory context matcher for offset-shifted files.
3. **Rollback Phase**: On any failure (exit code $\neq 0$ or adversarial failure), automatically invokes `Rollback()` to restore pristine state.

### 3.5 Adversarial Falsification (Anti-Overfitting Verification)
A green test run is necessary but insufficient. The `Falsifier` (`internal/falsify/falsify.go`):
1. Prompts Nemotron 3 Ultra to adopt an adversarial "Red-Teamer" persona;
2. Synthesizes an edge-case test case targeting unexpected inputs (e.g. `None`, boundary numbers, empty sequences);
3. Executes the synthesized test against the patched codebase in the sandbox;
4. Only marks the session as `SUCCEEDED` if both original and adversarial suites pass with high confidence.

---

## 4. Economic Telemetry & Token Ledger Model

All token consumption is tracked in real-time (`internal/engine/fsm.go`):
$$\text{Cost}_{\text{Nebius}} = \frac{P_{\text{tokens}} \times \$0.10 + C_{\text{tokens}} \times \$0.30}{1,000,000}$$
$$\text{Cost Savings} = \frac{\$25.00 - \text{Cost}_{\text{Nebius}}}{\$25.00} \times 100\%$$
*Baseline: 30 minutes of human triage at \$50.00/hour.*

Every verified fix automatically commits a **Verification Audit Card** (`HEAL_AUDIT_REPORT.md`) directly into the created PR branch for complete auditability.

---

## 5. State Machine & Transition Matrix

```
       [IDLE]
         │
         ▼
    [REPRODUCING] ────────(Tests Pass)───────► [SUCCEEDED (Clean)]
         │
         ▼ (Tests Fail)
    [DIAGNOSING] (AST Graph + Archetype Filter)
         │
         ▼
 [SEARCHING_KNOWLEDGE] (Tavily Real-time Grounding)
         │
         ▼
 [SYNTHESIZING_PATCH] (Nebius Nemotron 3 Ultra)
         │
         ▼
 [VERIFYING_SANDBOX] ◄───────────────┐
         │                           │
         ├──────(Verify Fail)──► [ROLLING_BACK] ──► (Next Turn Retry)
         │
         ▼ (Verify Pass)
 [ADVERSARIAL_FALSIFY]
         │
         ├──────(Falsify Fail)─► [ROLLING_BACK] ──► (Counter-example added to Retry)
         │
         ▼ (Falsify Pass)
    [SUCCEEDED] ──► (Create Git PR Branch with Audit Report)
```
