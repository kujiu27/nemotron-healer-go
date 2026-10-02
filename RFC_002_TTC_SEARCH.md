# 📄 RFC-002: Test-Time Compute (TTC) Tree Search & Adversarial Self-Play Engine

> **Document Status:** ORIGINAL DESIGN RFC — the shipped implementation is a subset: breadth-first archetype-guided
> hypothesis expansion with sandbox rollouts, reward-based branch selection, and feedback-guided depth-2 adversarial
> refinement (see `internal/engine/mcts_search.go`). Full tree selection iterations and minimax self-play are NOT
> implemented; user-facing naming says "Divergent Hypothesis Search (DHS)", not MCTS.
> **Theoretical Grounding:**  
> 1. *Large Language Monkeys: Scaling Inference Compute with Repeated Sampling* (Brown et al., Stanford, arXiv:2407.21787)  
> 2. *Scaling LLM Test-Time Compute Optimally can be More Effective than Scaling Pre-training* (Snell et al., UC Berkeley / Google DeepMind, arXiv:2408.03314)  
> 3. *Nemotron-4 340B Technical Report: Reward Models & Test-Time Rejection Sampling* (NVIDIA, 2024)  
> 4. *AlphaCode 2 Technical Report: Execution-Feedback Driven Program Search* (Google DeepMind, 2023)  

---

## 1. Executive Motivation & The Scaling Paradigm Shift

### 1.1 Why Single-Turn Healing Fails
Traditional autonomous coding agents follow a greedy, single-turn trial-and-error loop:
$$\text{Error} \longrightarrow \text{LLM}(\text{Prompt}) \longrightarrow \text{Patch}_1 \overset{\text{Fail}}{\longrightarrow} \text{LLM}(\text{Prompt} + \text{Error}_1) \longrightarrow \text{Patch}_2 \dots$$
As proven by Brown et al. (Stanford, 2024), single-attempt coding agents plateau around **15.9%** on real-world engineering benchmarks (SWE-bench Lite). When an agent encounters hard multi-file concurrency, memory corruption, or architectural breaking changes, single-turn prompting almost universally degenerates into hallucination or overfitting.

### 1.2 The Frontier Solution: Test-Time Compute (TTC) Scaling
In domains with **deterministic execution verifiers (Compilers, AST Linters, Sandboxed Unit Tests)**, scaling inference-time compute via parallel branching and search dramatically shifts the Pareto frontier. Stanford's empirical results prove that repeated verifiable sampling elevates solve rates from **15.9% to 56.0%**.

`nemotron-healer-go` converts the developer's **unlimited token advantage** into an insurmountable algorithmic moat by replacing linear trial-and-error with:
1. **Monte Carlo Tree Search (MCTS) over Code Mutations**: Structured exploration of patch hypotheses guided by UCB1.
2. **Verifiable Reward Function (RLVR)**: Multi-objective objective scoring (Compilation $\to$ Base Tests $\to$ Blast Radius Penalty $\to$ Adversarial Robustness).
3. **Red-Blue Adversarial Self-Play**: A Minimax game where a Red Agent actively synthesizes hostile edge cases while a Blue Agent hardens the AST until Nash equilibrium is achieved.

---

## 2. Mathematical Formalism & Search Graph

```
                                      Root Node S_0
                                (Initial Failing State)
                                          │
                  ┌───────────────────────┼───────────────────────┐
                  ▼                       ▼                       ▼
            Hypothesis A            Hypothesis B            Hypothesis C
         (e.g. asyncio.Lock)    (e.g. Queue Buffering)   (e.g. Semaphores)
                  │                       │                       │
           [MCTS Rollout]          [MCTS Rollout]          [MCTS Rollout]
                  │                       │                       │
                  ▼                       ▼                       ▼
           Reward R = 0.94         Reward R = 0.21         Reward R = 0.65
             (Selected)               (Pruned)                (Backup)
                  │
                  ▼
      ┌────────────────────────────────────────────────────────┐
      │          Red-Blue Adversarial Self-Play Arena          │
      │                                                        │
      │  [Red Team: Malicious Fuzzer]                          │
      │   - Synthesizes concurrency storms, None inputs        │
      │                 │                                      │
      │                 ▼ (Challenges Code)                    │
      │  [Blue Team: Surgical Hardener]                        │
      │   - Enforces RAII invariant & boundary guards          │
      │                 │                                      │
      │                 ▼                                      │
      │       Pareto-Optimal Robust Patch Evolved              │
      └────────────────────────────────────────────────────────┘
```

### 2.1 State Representation
Each MCTS node represents an immutable workspace snapshot:
$$S_t = \langle \mathcal{G}_{\text{AST}}, \; \mathcal{P}_t, \; \mathcal{E}_t, \; V(S_t), \; N(S_t) \rangle$$
- $\mathcal{G}_{\text{AST}}$: In-memory AST symbol call-graph.
- $\mathcal{P}_t$: Accumulated unified diff patch up to turn $t$.
- $\mathcal{E}_t$: Execution feedback (exit code, parsed stack traces, stdout).
- $V(S_t)$: Running reward evaluation value $V \in [-1.0, 1.0]$.
- $N(S_t)$: Visit count of node $S_t$.

### 2.2 UCB1 Selection Policy
To balance exploration (trying novel repair hypotheses) and exploitation (refining promising diffs), candidate child nodes are selected via Upper Confidence Bound:
$$\text{UCB1}(S_i) = \frac{V(S_i)}{N(S_i)} + C \cdot \sqrt{\frac{\ln N(S_{\text{parent}})}{N(S_i) + \epsilon}}$$
Where $C = \sqrt{2} \approx 1.414$ is the theoretical exploration constant.

### 2.3 Verifiable Reward Function (RLVR)
Unlike ungrounded LLMs that score answers with subjective prose, our reward function is **100% physically grounded in the sandbox**:
$$R(S) = w_{\text{test}} \cdot \mathbb{I}(\text{BasePass}) + w_{\text{adv}} \cdot \mathbb{I}(\text{AdvPass}) - w_{\text{blast}} \cdot \text{BlastRadius}(S) - w_{\text{size}} \cdot |\Delta_{\text{patch}}|$$
Where:
- $\mathbb{I}(\text{BasePass}) \in \{0, 1\}$: Did the original failing test turn green? (Weight: 0.50)
- $\mathbb{I}(\text{AdvPass}) \in \{0, 1\}$: Did the patch survive adversarial fuzzing? (Weight: 0.35)
- $\text{BlastRadius}(S) \in [0.0, 1.0]$: Penalty for cascading changes across unrelated files. (Weight: 0.10)
- $|\Delta_{\text{patch}}|$: Occam's razor penalty favoring minimal surgical diffs over massive refactors. (Weight: 0.05)

---

## 3. Red-Blue Adversarial Minimax Formulation

We formulate code hardening as a two-player zero-sum game between two asymmetric agents:
$$\max_{\theta_{\text{Blue}}} \min_{\phi_{\text{Red}}} \mathbb{E}_{\tau \sim \mathcal{D}} \left[ \mathcal{U}(\text{Code}_{\text{Blue}}, \; \text{Attack}_{\text{Red}}) \right]$$

1. **Red Agent (Hostile Invariant Attacker)**:
   - Evaluates the candidate patch.
   - Searches for edge-case vulnerabilities: unhandled `NoneType`, integer overflows, concurrent race windows, SQL metacharacters, or deadlock states.
   - Generates executable test attack vectors.
2. **Blue Agent (Defensive Code Synthesizer)**:
   - Takes the attack vector as ground-truth counter-examples.
   - Synthesizes localized defensive guards (RAII locks, null safety, boundary clamps) without breaking functional contracts.
3. **Termination Condition**:
   - The game terminates when the Red Agent exhausts all attack attempts within its budget ($K=3$) and the utility $\mathcal{U} \ge 0.95$.

---

## 4. Implementation Plan & Milestones

- [ ] **Milestone 1: MCTS Search Core (`internal/mcts/`)**
  - Implement tree node data structure, parent-child links, and UCB1 selection.
  - Implement rollout policy using Nemotron 3 Ultra with temperature scaling ($T=0.7$ for diverse branching).
- [ ] **Milestone 2: Verifiable Reward Evaluator (`internal/mcts/reward.go`)**
  - Connect sandbox test execution directly to numerical reward computation.
  - Integrate AST Blast Radius as a mathematical penalty.
- [ ] **Milestone 3: Red-Blue Adversarial Arena (`internal/arena/`)**
  - Implement turn-based Minimax self-play loop between Red fuzzer and Blue patcher.
- [ ] **Milestone 4: Empirical Benchmark Validation (AHB-10 Suite)**
  - Run MCTS vs Single-turn baseline on `fastapi_async_deadlock`, `pydantic_v2_migration`, and `sql_injection_remediation`.
  - Record tokens consumed, search depth, and convergence speed.
