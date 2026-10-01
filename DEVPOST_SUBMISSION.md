# Devpost Submission Draft (Nebius x NVIDIA Global AI Hackathon)

Track: Coding and Agentic Engineering
Repo: https://github.com/kujiu27/nemotron-healer-go (MIT)
Stack: Nebius Token Factory (`nvidia/Nemotron-3-Ultra-550b-a55b`), Tavily Search API, Go.

---

## Title

Nemotron-Healer — an in-situ autonomous code self-healing agent that verifies its own fixes

## Elevator pitch (≤100 words)

When CI goes red, Nemotron-Healer reproduces the failure, maps the symbol-graph blast radius,
retrieves current official docs via Tavily, synthesizes a surgical unified diff with NVIDIA
Nemotron 3 Ultra on Nebius Token Factory, then attacks its own patch with adversarial
counter-example tests before committing a fix branch with a full audit card. Single 7MB static
Go binary, ~12MB RSS, hot start under 10ms — drop-in as a CLI or a one-line GitHub Action.

## The problem

LLM coding assistants operate open-loop: they patch the symptom, hallucinate APIs past their
training cutoff, and overfit tests (hardcoded returns, deleted assertions). Nothing verifies the
fix survived contact with edge cases. Engineers still triage broken builds by hand.

## What it does

1. **Reproduce** — runs your failing test command, captures exit code and parsed tracebacks.
2. **Scope** — builds a workspace symbol graph (Go via `go/parser` AST; Python/TS/JS via line
   grammars) and computes a Blast Radius risk score; high-risk targets get a hard
   non-signature-breaking constraint injected into the prompt.
3. **Ground** — queries Tavily for live official migration docs; every claim in the final audit
   card carries a verifiable source URL.
4. **Synthesize** — Nemotron 3 Ultra (Token Factory, streaming) emits a minimal unified diff.
   Optional test-time-compute search: divergent archetype-guided hypotheses, sandbox rollouts,
   UCB1 selection, and feedback-guided depth-2 hardening of candidates that pass base tests but
   fail adversarial ones.
5. **Falsify** — an adversarial model generates hostile edge-case tests executed against the
   patched code. Failing the counter-example rolls the patch back. Skips (e.g. generator
   unavailable) are reported honestly as SKIPPED in the audit card, never as PASSED.
6. **Commit** — verified fix lands on a dedicated branch with a full audit report: token ledger,
   TTFT/TPS, catalog-priced cost, Tavily citation table, falsification status.

## How we built it

Pure Go, zero runtime dependencies. Cobra CLI + bubbletea TUI, streaming SSE client for Token
Factory, transactional checkpoint/rollback with an exact manifest (files created by failed
patches are deleted; pre-existing untracked files are untouched). Deterministic defect-archetype
classifier (concurrency, interface-breaking, security, resource-leak, null-type) binds mandatory
negative constraints to every synthesis prompt. Every user-visible number (pricing, throughput)
comes from the Token Factory catalog or the streamed usage counters — not estimates.

## Challenges we ran into

- Adversarial test synthesis can produce invalid tests; we classify syntax/import failures and
  report SKIPPED instead of punishing the patch.
- Rollback correctness: naive `git checkout .` leaks files created by failed patches; we diff
  against a checkpoint manifest.
- Best-of-N patch search burns tokens; UCB1 selection plus early-exit on reward ≥ 0.70 keeps the
  budget bounded.

## Accomplishments we're proud of

- Verification-first loop: no patch is committed until a hostile counter-example fails to break it.
- Audit card with live Tavily citations — every grounding claim is checkable by the reviewer.
- Single-binary distribution: same artifact runs locally, in CI, and as a GitHub Action.

## What we learned

Anti-overfitting gates must fail honestly: silently passing verification is worse than skipping it.

## What's next

- Container-isolated execution mode for untrusted repos.
- External benchmark runs (SWE-bench-style) with published, repeated measurements (`eval --repeat`).

## Run it

```bash
export NEBIUS_API_KEY=...   # tokenfactory.nebius.com
export TAVILY_API_KEY=...   # tavily.com
nemotron-healer . --command "pytest" --no-tui
```

## Video

3-minute demo following `DEMO_VIDEO_SCRIPT.md`: red CI, live heal of a 4-file cascading
concurrency deadlock, adversarial falsification, audit card, branch commit.
