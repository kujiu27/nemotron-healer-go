# Devpost Submission Draft (Nebius x NVIDIA Global AI Hackathon) — FINAL

Track: Coding and Agentic Engineering
Repo: https://github.com/kujiu27/nemotron-healer-go (MIT)
Stack: Nebius Token Factory (`nvidia/Nemotron-3-Ultra-550b-a55b` +
`nvidia/Nemotron-3-Nano-30B-A3B`), Tavily Search + Extract APIs, Go.

---

## Title

Nemotron-Healer — an in-situ autonomous code self-healing agent that verifies its own fixes

## Elevator pitch (≤100 words)

When CI goes red, Nemotron-Healer reproduces the failure, maps the symbol-graph blast radius,
retrieves live official docs via Tavily, synthesizes a surgical unified diff with NVIDIA
Nemotron 3 Ultra on Nebius Token Factory, then attacks its own patch with adversarial
counter-example tests before committing a fix branch with a full audit card. Single 8.0MB
static Go binary, ~12MB RSS, hot start under 10ms — drop-in as a CLI or a one-line GitHub
Action that is smoke-tested on a real runner on every push.

## The problem

LLM coding assistants operate open-loop: they patch the symptom, hallucinate APIs past their
training cutoff, and overfit tests (hardcoded returns, deleted assertions). Nothing verifies
the fix survived contact with edge cases. Engineers still triage broken builds by hand.

**Why not just use an existing coding agent?** Swe-agent/OpenHands/Aider-class tools generate
a patch and stop — the commit decision is blind trust. Nemotron-Healer is closed-loop: the
patch is not committed until an adversarially generated counter-example FAILS to break it,
and every number in its audit card is measured, not claimed. That verification gate, plus
in-situ operation on your own test command, is the product.

## What it does

1. **Reproduce** — runs your failing test command, captures exit code and parsed tracebacks.
2. **Scope** — builds a workspace symbol graph (Go via `go/parser` AST; Python/TS/JS via line
   grammars) and computes a Blast Radius risk score; high-risk targets get a hard
   non-signature-breaking constraint injected into the prompt.
3. **Ground** — the Nemotron Nano triage tier distills the traceback into a search
   query (rule-table fallback), queries Tavily, and extracts the full text of the
   top official doc; every claim in the final audit card carries a verifiable source URL.
4. **Synthesize** — Nemotron 3 Ultra (Token Factory, streaming) emits a minimal unified diff.
   Optional test-time-compute search: divergent archetype-guided hypotheses, sandbox rollouts,
   reward-scored branch selection, and feedback-guided depth-2 hardening of candidates that
   pass base tests but fail adversarial ones.
5. **Falsify** — an adversarial model generates hostile edge-case tests executed against the
   patched code. Failing the counter-example rolls the patch back. Skips (e.g. generator
   unavailable) are reported honestly as SKIPPED in the audit card, never as PASSED.
6. **Commit** — verified fix lands on a dedicated branch with a full audit report: token ledger,
   TTFT/TPS, catalog-priced cost, Tavily citation table, falsification status.

## Evidence you can verify in 60 seconds

We shipped an adversarial self-audit first (`GRAND_PRIZE_GAP_AUDIT.md`, with a remediation
ledger mapping every finding to its fix commit), then fixed everything it found:

- `make bench` regenerates `docs/PERF.md` — committed receipts for the 8.0MB binary,
  ~12MB RSS, sub-10ms start. No API keys needed.
- Two benchmark cases are REAL upstream bugs with red/green proofs in-repo:
  sergi/go-diff `6dbe13c` (AHB-06) and pelletier/go-toml `6fa69af` (AHB-07) —
  buggy tree fails, the upstream 17-line fix passes.
- The GitHub Action chain runs end-to-end on a real runner in CI (`action-smoke` job)
  on every push — no release-binary dependency.
- Release binaries auto-publish on tag pushes; `v0.7.32` assets are live.
- RED-state integrity gate: CI verifies every benchmark case still FAILS its own
  test command as shipped — no leaked answers, enforced on every push.

## How we built it

Pure Go, zero runtime dependencies. Cobra CLI + bubbletea TUI, streaming SSE client for Token
Factory with exponential-backoff retry (429/5xx), transactional checkpoint/rollback with an
exact manifest (files created by failed patches are deleted; pre-existing untracked files are
untouched). Deterministic defect-archetype classifier (concurrency, interface-breaking,
security, resource-leak, null-type) binds mandatory negative constraints to every synthesis
prompt. A three-tier patch applier (git apply → patch(1) → per-hunk in-memory fallback)
survives imperfect LLM diffs. Every user-visible number comes from the Token Factory catalog
or the streamed usage counters — never estimates.

## Challenges we ran into

- Adversarial test synthesis can produce invalid tests; we classify syntax/import failures and
  report SKIPPED instead of punishing the patch.
- Rollback correctness: naive `git checkout .` leaks files created by failed patches; we diff
  against a checkpoint manifest.
- LLM diffs are messy: our first fuzzy fallback concatenated all hunks and never matched;
  we rewrote it to apply hunks individually with a regression test locking the behavior.
- Best-of-N patch search burns tokens; reward-scored selection plus early-exit on reward ≥ 0.70
  keeps the budget bounded.

## Accomplishments we're proud of

- Verification-first loop: no patch is committed until a hostile counter-example fails to break it.
- Audit card with live Tavily citations — every grounding claim is checkable by the reviewer.
- Honesty under failure: infrastructure errors fail closed; skips are labeled, never faked.
- Single-binary distribution: same artifact runs locally, in CI, and as a GitHub Action.

## What we learned

Anti-overfitting gates must fail honestly: silently passing verification is worse than
skipping it. Self-auditing the repo before the judges do — and committing the receipts —
is cheaper than being caught.

## What's next

- Published real-API benchmark runs (`scripts/run_real_eval.sh` → `docs/EVAL_RESULTS.md`).
- SWE-bench-style external evaluation with repeated measurements.

## Run it

```bash
go install github.com/kujiu27/nemotron-healer-go/cmd/nemotron-healer@latest
export NEBIUS_API_KEY=...   # tokenfactory.nebius.com
export TAVILY_API_KEY=...   # tavily.com
nemotron-healer . --command "pytest" --no-tui
```

Or as a one-line GitHub Action (`uses: kujiu27/nemotron-healer-go@v0.7.32`) — see README.

## Video

3-minute demo following `DEMO_VIDEO_SCRIPT.md`: red CI, live heal of a 4-file cascading
concurrency deadlock, adversarial falsification, audit card, branch commit.
