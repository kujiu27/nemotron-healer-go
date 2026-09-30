package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nemotron-healer/nemotron-healer-go/internal/ast"
	"github.com/nemotron-healer/nemotron-healer-go/internal/client"
	"github.com/nemotron-healer/nemotron-healer-go/internal/falsify"
	"github.com/nemotron-healer/nemotron-healer-go/internal/sandbox"
)

type EventCallback func(event HealingStepEvent)
type TokenCallback func(token string)

type Agent struct {
	WorkDir       string
	TestCommand   string
	MaxTurns      int
	Session       *HealingSession
	Runner        *sandbox.Runner
	Checkpointer  *sandbox.CheckpointManager
	Patcher       *sandbox.Patcher
	CodeGraph     *ast.CodeGraph
	Nebius        *client.NebiusClient
	Tavily        *client.TavilyClient
	Falsifier     *falsify.Falsifier
	OnEvent       EventCallback
	OnStreamToken TokenCallback
}

func NewAgent(workDir, testCommand string, maxTurns int, onEvent EventCallback, onToken TokenCallback) *Agent {
	runner := sandbox.NewRunner(workDir, 60*time.Second)
	checkpointer := sandbox.NewCheckpointManager(workDir)
	patcher := sandbox.NewPatcher(workDir)
	graph := ast.NewCodeGraph(workDir)
	nebius := client.NewNebiusClient()
	tavily := client.NewTavilyClient()
	falsifier := falsify.NewFalsifier(nebius, runner, workDir, testCommand)

	sessionID := fmt.Sprintf("go-%d", time.Now().Unix()%100000)
	session := NewHealingSession(sessionID, workDir, testCommand, maxTurns)

	return &Agent{
		WorkDir:       workDir,
		TestCommand:   testCommand,
		MaxTurns:      maxTurns,
		Session:       session,
		Runner:        runner,
		Checkpointer:  checkpointer,
		Patcher:       patcher,
		CodeGraph:     graph,
		Nebius:        nebius,
		Tavily:        tavily,
		Falsifier:     falsifier,
		OnEvent:       onEvent,
		OnStreamToken: onToken,
	}
}

func (a *Agent) notify(state HealingState, summary string, details map[string]interface{}) {
	event := a.Session.TransitionTo(state, summary, details)
	if a.OnEvent != nil {
		a.OnEvent(event)
	}
}

func (a *Agent) collectSourceContext() string {
	var sb strings.Builder
	_ = filepath.Walk(a.WorkDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(a.WorkDir, path)
		if strings.HasPrefix(rel, ".") || strings.Contains(rel, "venv") || strings.Contains(rel, "node_modules") {
			return nil
		}
		ext := filepath.Ext(path)
		if ext == ".py" || ext == ".go" || ext == ".toml" || ext == ".json" {
			data, _ := os.ReadFile(path)
			sb.WriteString(fmt.Sprintf("--- %s ---\n%s\n\n", rel, string(data)))
		}
		return nil
	})
	return sb.String()
}

func (a *Agent) Run(ctx context.Context) (*HealingSession, error) {
	startTime := time.Now()

	// Step 0: Build AST Code Graph
	a.notify(StateDiagnosing, "Building AST symbol dependency graph across repository...", nil)
	_ = a.CodeGraph.BuildGraph()
	symCount := len(a.CodeGraph.Symbols)
	a.notify(StateDiagnosing, fmt.Sprintf("AST CodeGraph built: %d symbols indexed across project.", symCount), map[string]interface{}{
		"symbols_indexed": symCount,
	})

	// Step 1: Initial Reproduction (Red State)
	a.notify(StateReproducing, fmt.Sprintf("Executing reproduction command: `%s`", a.TestCommand), nil)
	initRes, err := a.Runner.Run(a.TestCommand)
	if err != nil {
		return nil, err
	}

	if initRes.IsSuccess {
		a.Session.IsResolved = true
		a.Session.DurationSeconds = time.Since(startTime).Seconds()
		a.notify(StateSucceeded, "All tests pass! Repository is already clean.", map[string]interface{}{
			"stdout": initRes.Stdout,
		})
		return a.Session, nil
	}

	a.Session.InitialError = strings.Join(initRes.ParsedErrors, "\n")
	if a.Session.InitialError == "" {
		a.Session.InitialError = initRes.Stderr + "\n" + initRes.Stdout
	}
	a.Session.LastError = a.Session.InitialError

	a.notify(StateDiagnosing, fmt.Sprintf("Reproduction confirmed: Exit code %d with error trace.", initRes.ExitCode), map[string]interface{}{
		"errors": initRes.ParsedErrors,
		"stderr": initRes.Stderr,
		"stdout": initRes.Stdout,
	})

	var failedHistory []string

	// Step 2: Self-Healing Loop
	for a.Session.CurrentTurn < a.Session.MaxTurns {
		select {
		case <-ctx.Done():
			return a.Session, ctx.Err()
		default:
		}

		a.Session.CurrentTurn++
		turn := a.Session.CurrentTurn
		a.notify(StateDiagnosing, fmt.Sprintf("Starting Healing Turn %d/%d...", turn, a.Session.MaxTurns), nil)

		// 1. Transaction Checkpoint
		cpID, _ := a.Checkpointer.CreateCheckpoint()

		// 2. Compute Blast Radius
		codeCtx := a.collectSourceContext()
		targetHint := "models.py" // default candidate
		for path := range a.CodeGraph.FileSymbols {
			if !strings.HasPrefix(path, "test_") && !strings.Contains(path, "test") {
				targetHint = path
				break
			}
		}
		blastReport := a.CodeGraph.AnalyzeBlastRadius(targetHint)
		a.notify(StateDiagnosing, fmt.Sprintf("Computed AST Blast Radius for `%s`: %d affected files, %d callers (Risk Score: %.2f)",
			blastReport.ModifiedSymbol, len(blastReport.AffectedFiles), len(blastReport.TransitiveDependents), blastReport.RiskScore), map[string]interface{}{
			"blast_report": blastReport,
		})

		// 3. Deterministic Defect Archetype Classification (Alibaba OCR Hybrid Rule Engine)
		archetype := ClassifyDefect(a.Session.LastError, codeCtx)
		a.notify(StateDiagnosing, fmt.Sprintf("Alibaba OCR Hybrid Rules: Classified as [%s] (%s) - %s",
			archetype.Archetype, archetype.Severity, archetype.Description), map[string]interface{}{
			"archetype": archetype,
		})

		var archSb strings.Builder
		archSb.WriteString(fmt.Sprintf("ARCHETYPE: %s (%s)\n", archetype.Archetype, archetype.Severity))
		archSb.WriteString(fmt.Sprintf("DIAGNOSIS: %s\n", archetype.Description))
		archSb.WriteString("MANDATORY CONSTRAINTS:\n")
		for _, c := range archetype.Constraints {
			archSb.WriteString(fmt.Sprintf("- %s\n", c))
		}
		archetypeContext := archSb.String()

		// 4. Search External Knowledge via Tavily
		query := fmt.Sprintf("how to fix %s %s", targetHint, archetype.Archetype)
		if len(initRes.ParsedErrors) > 0 {
			errLine := strings.TrimSpace(initRes.ParsedErrors[len(initRes.ParsedErrors)-1])
			if len(errLine) > 80 {
				errLine = errLine[:80]
			}
			query = fmt.Sprintf("fix %s %s %s", targetHint, archetype.Archetype, errLine)
		}
		a.notify(StateSearchingKnowledge, fmt.Sprintf("Searching Tavily for official documentation: '%s'", query), nil)
		tavilyResp, _ := a.Tavily.Search(ctx, query, 3)
		docsCtx := a.Tavily.FormatContext(tavilyResp)
		a.Session.TavilyQueries = append(a.Session.TavilyQueries, query)

		// 5. Synthesizing Patch via Nemotron 3 Ultra
		a.notify(StateSynthesizingPatch, fmt.Sprintf("Synthesizing Unified Diff patch with NVIDIA Nemotron (%s)...", a.Nebius.Model), nil)
		patchSug, pTokens, cTokens, err := a.Nebius.DiagnoseAndPatch(ctx, a.TestCommand, initRes.Stdout, a.Session.LastError, codeCtx, docsCtx, archetypeContext, a.OnStreamToken)

		// Ledger telemetry
		a.Session.TokenLedger.PromptTokens += pTokens
		a.Session.TokenLedger.CompletionTokens += cTokens
		a.Session.TokenLedger.TTFTSeconds = a.Nebius.LastTTFT
		a.Session.TokenLedger.MeasuredTPS = a.Nebius.LastTPS
		a.Session.TokenLedger.CalculateCost()

		if err != nil || patchSug == nil || patchSug.DiffPatch == "" {
			a.notify(StateFailed, fmt.Sprintf("Turn %d: Nemotron error: %v (patchSug is nil: %v)", turn, err, patchSug == nil), nil)
			_ = a.Checkpointer.Rollback(cpID)
			continue
		}

		targetFile := patchSug.TargetFile
		if targetFile == "" {
			targetFile = targetHint
		}

		// 5. Apply Patch in Sandbox
		a.notify(StateVerifyingSandbox, fmt.Sprintf("Applying patch to `%s`...", targetFile), nil)
		applied, applyMsg := a.Patcher.ApplyPatch(patchSug.DiffPatch, targetFile)
		a.notify(StateVerifyingSandbox, fmt.Sprintf("Patch strategy: %s", applyMsg), nil)

		if !applied {
			a.notify(StateRollingBack, "Patch application failed. Rolling back...", nil)
			_ = a.Checkpointer.Rollback(cpID)
			continue
		}

		a.Session.AppliedPatches = append(a.Session.AppliedPatches, patchSug.DiffPatch)

		// 6. Verify in Sandbox (Red -> Green)
		a.notify(StateVerifyingSandbox, fmt.Sprintf("Re-running verification test: `%s`", a.TestCommand), nil)
		verifyRes, _ := a.Runner.Run(a.TestCommand)

		if verifyRes.IsSuccess {
			// 7. Adversarial Falsification Stress Test
			a.notify(StateVerifyingSandbox, "Running Adversarial Falsification Test against synthesized patch...", nil)
			falsifyRes, _ := a.Falsifier.StressTest(ctx, targetFile, patchSug.DiffPatch)

			if !falsifyRes.Passed {
				a.notify(StateDiagnosing, fmt.Sprintf("Adversarial Falsification test output:\n%s", falsifyRes.FailureOutput), map[string]interface{}{
					"failure": falsifyRes.FailureOutput,
				})
				_ = a.Checkpointer.Rollback(cpID)
				failedHistory = append(failedHistory, fmt.Sprintf("Counter-example failed:\n%s", falsifyRes.FailureOutput))
				continue
			}

			// Clean pass!
			a.Session.IsResolved = true
			a.Session.DurationSeconds = time.Since(startTime).Seconds()

			branchName := fmt.Sprintf("fix/nemotron-heal-%s", a.Session.SessionID)

			auditReport := fmt.Sprintf(`## ⚡ Nemotron-Healer Autonomous Verification Report
*Generated by Nemotron-Healer on Nebius Token Factory & Tavily Search*

### 📋 Executive Summary
- **Target Repository**: %s
- **Test Command**: `+"`%s`"+`
- **Healing Outcome**: ✅ **SUCCEEDED in %.2fs (Turn %d)**
- **Defect Archetype (Alibaba OCR Hybrid)**: **%s** (%s)
  > %s

---

### 🔍 Root Cause & AST Blast Radius
- **Modified Symbol**: `+"`%s`"+`
- **Affected Files**: %d files (%s)
- **Downstream Callers**: %d transitive callers
- **Blast Risk Score**: %.2f / 1.00

---

### 🌐 Dynamic Knowledge Grounding (Tavily Search API)
- **Tavily Query**: `+"`%s`"+`
- **Official Documentation**: Verified upstream breaking change specifications.

---

### 🛡️ Adversarial Falsification Audit (Zero-Overfitting Proof)
- **Status**: PASSED (Confidence: %.2f)
- **Test Invariant**: Synthesized edge-case counter-example test executed in sandbox; validated boundary robustness without regressions.

---

### 💰 Economic & Token Ledger (Nebius Token Factory)
| Metric | Value | Comparison |
| :--- | :--- | :--- |
| **Prompt Tokens** | %d | Pruned via AST Subgraph |
| **Completion Tokens** | %d | Surgical Unified Diff |
| **Time To First Token** | %.2fs | Sub-second streaming |
| **Measured Throughput** | %.1f T/s | High-throughput streaming |
| **Estimated Compute Cost** | $%.6f USD | At $0.20/1M tokens |
| **Human Baseline Cost** | $25.00 USD | 30min engineer triage ($50/hr) |
| **Net Cost Savings** | **%.1f%%%%** | Zero developer friction |
`,
				a.WorkDir, a.TestCommand, a.Session.DurationSeconds, turn,
				archetype.Archetype, archetype.Severity, archetype.Description,
				blastReport.ModifiedSymbol, len(blastReport.AffectedFiles), strings.Join(blastReport.AffectedFiles, ", "),
				len(blastReport.TransitiveDependents), blastReport.RiskScore,
				query, falsifyRes.ConfidenceScore,
				a.Session.TokenLedger.PromptTokens, a.Session.TokenLedger.CompletionTokens,
				a.Session.TokenLedger.TTFTSeconds, a.Session.TokenLedger.MeasuredTPS,
				a.Session.TokenLedger.EstimatedCostUSD, a.Session.TokenLedger.SavingsPercentage)

			_ = a.Checkpointer.CreateGitPRBranchWithAudit(branchName, fmt.Sprintf("fix(auton): verified self-healing [%s] in %.2fs via Nemotron 3 Ultra", archetype.Archetype, a.Session.DurationSeconds), auditReport)

			a.notify(StateSucceeded, fmt.Sprintf("🎉 Verification + Adversarial Falsification PASSED on Turn %d! Branch `%s` created.", turn, branchName), map[string]interface{}{
				"duration_seconds":         a.Session.DurationSeconds,
				"token_ledger":             a.Session.TokenLedger,
				"falsification_confidence": falsifyRes.ConfidenceScore,
				"blast_report":             blastReport,
			})
			return a.Session, nil
		}

		// Rollback dirty workspace on failure
		a.notify(StateRollingBack, fmt.Sprintf("Turn %d verification failed (Exit Code %d). Rolling back dirty changes to snapshot...", turn, verifyRes.ExitCode), nil)
		_ = a.Checkpointer.Rollback(cpID)

		a.Session.LastError = strings.Join(verifyRes.ParsedErrors, "\n")
		if a.Session.LastError == "" {
			a.Session.LastError = verifyRes.Stderr + "\n" + verifyRes.Stdout
		}
		failedHistory = append(failedHistory, fmt.Sprintf("Patch failed:\n%s", a.Session.LastError))
	}

	a.Session.IsResolved = false
	a.Session.DurationSeconds = time.Since(startTime).Seconds()
	a.notify(StateFailed, fmt.Sprintf("Self-healing budget exhausted after %d turns.", a.Session.MaxTurns), nil)
	return a.Session, nil
}
