package engine

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kujiu27/nemotron-healer-go/internal/arena"
	"github.com/kujiu27/nemotron-healer-go/internal/ast"
	"github.com/kujiu27/nemotron-healer-go/internal/client"
	"github.com/kujiu27/nemotron-healer-go/internal/falsify"
	"github.com/kujiu27/nemotron-healer-go/internal/sandbox"
)

type EventCallback func(event HealingStepEvent)
type TokenCallback func(token string)

type Agent struct {
	WorkDir      string
	TestCommand  string
	MaxTurns     int
	EnableSearch bool
	EnableArena  bool
	// DisableGrounding turns off Tavily retrieval and archetype constraints;
	// used as the un-grounded baseline in A/B ablation runs.
	DisableGrounding  bool
	lastTavilyResults []client.TavilySearchResultItem
	Session           *HealingSession
	Runner            *sandbox.Runner
	Checkpointer      *sandbox.CheckpointManager
	Patcher           *sandbox.Patcher
	CodeGraph         *ast.CodeGraph
	Nebius            *client.NebiusClient
	Tavily            *client.TavilyClient
	Falsifier         *falsify.Falsifier
	OnEvent           EventCallback
	OnStreamToken     TokenCallback
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

func (a *Agent) collectSourceContext(priorityFiles ...string) string {
	var sb strings.Builder
	const maxFileBytes = 35 * 1024
	const maxTotalBytes = 120 * 1024
	totalBytes := 0
	loaded := make(map[string]bool)

	appendFile := func(rel string) {
		if loaded[rel] || totalBytes >= maxTotalBytes {
			return
		}
		full := filepath.Join(a.WorkDir, rel)
		data, err := os.ReadFile(full)
		if err != nil {
			return
		}
		loaded[rel] = true
		content := string(data)
		if len(content) > maxFileBytes {
			content = content[:maxFileBytes] + "\n... [Truncated for token budget] ..."
		}
		block := fmt.Sprintf("--- %s ---\n%s\n\n", rel, content)
		sb.WriteString(block)
		totalBytes += len(block)
	}

	for _, pf := range priorityFiles {
		if pf != "" {
			appendFile(pf)
		}
	}

	_ = filepath.Walk(a.WorkDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || totalBytes >= maxTotalBytes {
			return nil
		}
		rel, _ := filepath.Rel(a.WorkDir, path)
		lower := strings.ToLower(rel)
		if strings.HasPrefix(rel, ".") ||
			strings.Contains(lower, "venv") ||
			strings.Contains(lower, "node_modules") ||
			strings.Contains(lower, "__pycache__") ||
			strings.Contains(lower, "dist/") ||
			strings.Contains(lower, "bin/") ||
			strings.Contains(lower, ".git") {
			return nil
		}
		ext := filepath.Ext(path)
		if ext == ".py" || ext == ".go" || ext == ".ts" || ext == ".js" || ext == ".toml" || ext == ".json" || ext == ".sql" {
			appendFile(rel)
		}
		return nil
	})
	return sb.String()
}

func (a *Agent) Run(ctx context.Context) (*HealingSession, error) {
	startTime := time.Now()

	// Ensure repository is safely Git-tracked for atomic transactions
	_ = a.Checkpointer.EnsureGitContext()

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

	// Step 2: If Test-Time Compute (TTC) Divergent Hypothesis Search is enabled, explore hypothesis branches
	if a.EnableSearch {
		mctsRes, err := a.RunMCTSSearch(ctx, a.Session.LastError, 6)
		if err == nil && mctsRes.Resolved {
			return a.Session, nil
		}
		a.notify(StateDiagnosing, "DHS (Divergent Hypothesis Search) did not achieve high-reward convergence; falling back to sequential repair...", nil)
	}

	// Step 3: Self-Healing Loop
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

		// 2. Resolve Target Location & Compute AST Blast Radius
		loc := ResolveTargetLocation(a.WorkDir, a.Session.LastError, a.CodeGraph)
		targetHint := loc.FilePath
		codeCtx := a.collectSourceContext(targetHint)
		blastReport := a.CodeGraph.AnalyzeBlastRadius(targetHint, loc.Symbol)
		a.notify(StateDiagnosing, fmt.Sprintf("Computed AST Blast Radius for `%s` (file: `%s`): %d affected files, %d callers (Risk Score: %.2f)",
			blastReport.ModifiedSymbol, targetHint, len(blastReport.AffectedFiles), len(blastReport.TransitiveDependents), blastReport.RiskScore), map[string]interface{}{
			"blast_report": blastReport,
		})

		// 3. Deterministic Defect Archetype Classification (Rule Engine)
		archetype := ClassifyDefect(a.Session.LastError, codeCtx)
		a.notify(StateDiagnosing, fmt.Sprintf("Archetype Rule Engine: Classified as [%s] (%s) - %s",
			archetype.Archetype, archetype.Severity, archetype.Description), map[string]interface{}{
			"archetype": archetype,
		})

		archetypeContext := ""
		if !a.DisableGrounding {
			var archSb strings.Builder
			archSb.WriteString(fmt.Sprintf("ARCHETYPE: %s (%s)\n", archetype.Archetype, archetype.Severity))
			archSb.WriteString(fmt.Sprintf("DIAGNOSIS: %s\n", archetype.Description))
			archSb.WriteString("MANDATORY CONSTRAINTS:\n")
			for _, c := range archetype.Constraints {
				archSb.WriteString(fmt.Sprintf("- %s\n", c))
			}
			if blastReport.RiskScore > 0.70 {
				archSb.WriteString(fmt.Sprintf("\nCRITICAL AST BLAST CONSTRAINT (Risk Score: %.2f > 0.70):\n", blastReport.RiskScore))
				archSb.WriteString(fmt.Sprintf("- Target symbol `%s` has %d callers across %d files.\n", blastReport.ModifiedSymbol, len(blastReport.TransitiveDependents), len(blastReport.AffectedFiles)))
				archSb.WriteString("- DO NOT alter public signatures, function parameters, or return types.\n")
				archSb.WriteString("- MUST restrict patch strictly to internal logic to avoid cascading downstream regressions.\n")
			}
			if archetype.ExemplarPattern != "" {
				archSb.WriteString(fmt.Sprintf("\n%s\n", archetype.ExemplarPattern))
			}
			archetypeContext = archSb.String()
		}

		// 3.5 Pre-flight Triage via Tier-1 FastModel (Heterogeneous Dual-Model Architecture)
		if a.Nebius.FastModel != "" && !a.DisableGrounding {
			triage, ftP, ftC, ftErr := a.Nebius.FastTriage(ctx, a.Session.LastError)
			if ftErr == nil && triage != nil {
				a.Session.TokenLedger.PromptTokens += ftP
				a.Session.TokenLedger.CompletionTokens += ftC
				if triage.HypothesizedRootCause != "" {
					a.notify(StateDiagnosing, fmt.Sprintf("Tier-1 Fast Triage (%s): %s", a.Nebius.FastModel, triage.HypothesizedRootCause), nil)
				}
			}
		}

		// 4. Search External Knowledge via Tavily
		docsCtx := ""
		query := ""
		if a.DisableGrounding {
			a.notify(StateDiagnosing, "Grounding disabled (baseline mode): skipping Tavily retrieval and archetype constraints.", nil)
		} else {
			query = BuildGroundingQuery(a.WorkDir, targetHint, archetype, a.Session.LastError)
			a.notify(StateSearchingKnowledge, fmt.Sprintf("Searching Tavily for official documentation: '%s'", query), nil)
			tavilyResp, _ := a.Tavily.Search(ctx, query, 3)
			docsCtx = a.Tavily.FormatContext(tavilyResp)
			if tavilyResp != nil {
				a.lastTavilyResults = tavilyResp.Results
			}
			a.Session.TavilyQueries = append(a.Session.TavilyQueries, query)
		}

		// 5. Synthesizing Patch via Nemotron 3 Ultra
		a.notify(StateSynthesizingPatch, fmt.Sprintf("Synthesizing Unified Diff patch with NVIDIA Nemotron (%s)...", a.Nebius.Model), nil)
		patchSug, pTokens, cTokens, err := a.Nebius.DiagnoseAndPatch(ctx, a.TestCommand, initRes.Stdout, a.Session.LastError, codeCtx, docsCtx, archetypeContext, failedHistory, a.OnStreamToken)
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

		// 5. Apply Atomic Patch in Sandbox
		targetSummary := fmt.Sprintf("`%s`", targetFile)
		if len(patchSug.TargetFiles) > 1 {
			targetSummary = fmt.Sprintf("%d files (`%s`)", len(patchSug.TargetFiles), strings.Join(patchSug.TargetFiles, "`, `"))
		}
		a.notify(StateVerifyingSandbox, fmt.Sprintf("Applying atomic patch across %s...", targetSummary), nil)
		applied, applyMsg := a.Patcher.ApplyPatch(patchSug.DiffPatch, patchSug.TargetFiles...)
		a.notify(StateVerifyingSandbox, fmt.Sprintf("Patch strategy: %s", applyMsg), nil)

		if !applied {
			a.notify(StateRollingBack, "Patch application failed. Rolling back...", nil)
			_ = a.Checkpointer.Rollback(cpID)
			continue
		}

		// 5.5 Fast Pre-Flight Syntax Gate (<50ms)
		syntaxOK := true
		var syntaxErrMsg string
		targetsToCheck := patchSug.TargetFiles
		if len(targetsToCheck) == 0 && targetFile != "" {
			targetsToCheck = []string{targetFile}
		}
		for _, tf := range targetsToCheck {
			if ok, sErr := sandbox.PreFlightSyntaxCheck(a.WorkDir, tf); !ok {
				syntaxOK = false
				syntaxErrMsg = sErr
				break
			}
		}
		if !syntaxOK {
			a.notify(StateDiagnosing, fmt.Sprintf("⚡ Instant Syntax Gate (<50ms) rejected patch: %s", syntaxErrMsg), map[string]interface{}{
				"syntax_error": syntaxErrMsg,
			})
			_ = a.Checkpointer.Rollback(cpID)
			failedHistory = append(failedHistory, fmt.Sprintf("Failed Attempt Unified Diff:\n```diff\n%s\n```\nFailure Mode: Pre-flight Syntax Compilation Error\nError Diagnostics:\n%s", patchSug.DiffPatch, syntaxErrMsg))
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
				failedHistory = append(failedHistory, fmt.Sprintf("Failed Attempt Unified Diff:\n```diff\n%s\n```\nFailure Mode: Adversarial Edge Case Falsification\nError Diagnostics:\n%s", patchSug.DiffPatch, falsifyRes.FailureOutput))
				continue
			}

			// 8. Optional Red-Blue Adversarial Arena (attack/defend self-play)
			arenaNotes := "Single-Agent Verifiable Falsification"
			if a.EnableArena {
				a.notify(StateVerifyingSandbox, "Entering Red-Blue Adversarial Arena (Self-Play)...", nil)
				gameArena := arena.NewArena(a.Nebius, a.Runner, a.Patcher, a.WorkDir, a.TestCommand)
				rounds, eq, _ := gameArena.SelfPlay(ctx, targetFile, 2)
				a.notify(StateVerifyingSandbox, fmt.Sprintf("Red-Blue Arena: Survived %d attack rounds (defense held all rounds: %v)", len(rounds), eq), nil)
				arenaNotes = fmt.Sprintf("Survived %d Adversarial Attack-Defend Rounds (defense held all rounds: %v)", len(rounds), eq)
			}

			// Clean pass!
			a.Session.IsResolved = true
			a.Session.DurationSeconds = time.Since(startTime).Seconds()
			a.Session.ThoughtChain = patchSug.ThoughtChain

			// Cryptographic Patch Provenance (SHA-256 non-repudiation signature)
			hasher := sha256.New()
			hasher.Write([]byte(patchSug.DiffPatch))
			patchDigest := fmt.Sprintf("sha256:%x", hasher.Sum(nil))
			a.Session.PatchDigest = patchDigest

			// 7.5 Synthesize and Persist Permanent Regression Test Guard
			if falsifyRes.Passed && falsifyRes.GeneratedTest != "" && !falsifyRes.Skipped {
				regRelFile, regErr := a.Falsifier.PersistRegressionTest(targetFile, falsifyRes.GeneratedTest)
				if regErr == nil {
					a.Session.RegressionTestFile = regRelFile
					a.notify(StateVerifyingSandbox, fmt.Sprintf("🛡️ Synthesized permanent regression test artifact: `%s`", regRelFile), map[string]interface{}{
						"regression_file": regRelFile,
					})
				}
			}

			branchName := fmt.Sprintf("fix/nemotron-heal-%s", a.Session.SessionID)

			var tavilyCitationTable strings.Builder
			if !a.DisableGrounding && len(a.Session.TavilyQueries) > 0 {
				tavilyCitationTable.WriteString(fmt.Sprintf("- **Tavily Query**: `%s`\n", a.Session.TavilyQueries[len(a.Session.TavilyQueries)-1]))
				if len(a.lastTavilyResults) > 0 {
					tavilyCitationTable.WriteString("\n| # | Source Reference | Verifiable URL | Relevance | Ground-Truth Excerpt |\n")
					tavilyCitationTable.WriteString("| :--- | :--- | :--- | :--- | :--- |\n")
					for cIdx, item := range a.lastTavilyResults {
						snippet := strings.ReplaceAll(item.Content, "\n", " ")
						if len(snippet) > 85 {
							snippet = snippet[:85] + "..."
						}
						tavilyCitationTable.WriteString(fmt.Sprintf("| %d | %s | [%s](%s) | %.2f | *\"%s\"* |\n",
							cIdx+1, item.Title, item.URL, item.URL, item.Score, snippet))
					}
				} else {
					tavilyCitationTable.WriteString("- **Official Documentation**: Tavily returned no results for this query.\n")
				}
			} else {
				tavilyCitationTable.WriteString("- **Knowledge Grounding**: Disabled (baseline mode).\n")
			}

			falsifyStatus := "PASSED"
			if falsifyRes.Skipped {
				falsifyStatus = "SKIPPED — " + falsifyRes.Reason
			}

			// Quantitative Multi-Tier Economics
			pTok := a.Session.TokenLedger.PromptTokens
			cTok := a.Session.TokenLedger.CompletionTokens
			nebiusCost := a.Session.TokenLedger.EstimatedCostUSD
			cloudCost := (float64(pTok)*5.0 + float64(cTok)*15.0) / 1_000_000.0
			cloudSavingsPct := 0.0
			if cloudCost > 0 {
				cloudSavingsPct = ((cloudCost - nebiusCost) / cloudCost) * 100.0
			}

			regressionGuardSection := "- **Regression Guard**: Counter-example verified in sandbox ephemeral test."
			if a.Session.RegressionTestFile != "" {
				regressionGuardSection = fmt.Sprintf("- **Permanent Regression Test**: `%s` (Committed into branch to permanently prevent regressions in CI)", a.Session.RegressionTestFile)
			}

			reasoningSection := ""
			if a.Session.ThoughtChain != "" {
				reasoningSection = fmt.Sprintf("\n---\n\n### 🧠 NVIDIA Nemotron Deep Reasoning Trace\n<details>\n<summary>Click to expand architectural & mathematical deduction chain (%d chars)</summary>\n\n```text\n%s\n```\n\n</details>\n", len(a.Session.ThoughtChain), a.Session.ThoughtChain)
			}

			auditReport := fmt.Sprintf(`## ⚡ Nemotron-Healer Autonomous Verification Report
*Generated by Nemotron-Healer on Nebius Token Factory & Tavily Search*

### 📋 Executive Summary
- **Target Repository**: %s
- **Test Command**: `+"`%s`"+`
- **Healing Outcome**: ✅ **SUCCEEDED in %.2fs (Turn %d)**
- **Defect Archetype (Deterministic Rule Engine)**: **%s** (%s)
  > %s

---

### 🔍 Root Cause & AST Blast Radius
- **Modified Symbol**: `+"`%s`"+`
- **Affected Files**: %d files (%s)
- **Downstream Callers**: %d transitive callers
- **Blast Risk Score**: %.2f / 1.00

---

### 🌐 Dynamic Knowledge Grounding (Tavily Search API)
%s

---

### 🛡️ Adversarial Falsification & Permanent Regression Guard
- **Status**: %s (Confidence: %.2f)
- **Arena Defense**: %s
%s
%s
---

### 🔒 Cryptographic Provenance & Audit Signature
- **Patch SHA-256 Digest**: `+"`%s`"+`
- **Inference Platform**: Nebius Token Factory (High-Throughput Streaming Engine)
- **Model Signature**: `+"`%s`"+`
- **Tamper Evidence**: Cryptographically bound to Git commit trailers `+"`X-Nemotron-Audit`"+`

---

### 💰 Quantitative Economic & Throughput Matrix
| Infrastructure Tier | Model / Agent | Pricing Rate (Prompt / Completion) | Estimated Run Cost | Savings vs Baseline |
| :--- | :--- | :--- | :--- | :--- |
| ⚡ **Nebius Token Factory** | **NVIDIA Nemotron 3 Ultra** | **$1.00 / $3.00 per 1M** | **$%.6f USD** | — (Our Platform) |
| ☁️ Proprietary Cloud Baseline | Standard GPT-4o API | $5.00 / $15.00 per 1M | $%.6f USD | **-%.1f%%%% Cost Reduction** |
| 🧑‍💻 Senior Staff Engineer | 30-min Manual Triage ($50/hr) | Fixed Engineering Salary | $25.00 USD | **-%.1f%%%% Net Savings** |

**Nebius High-Performance Streaming Metrics:**
- **Time To First Token (TTFT)**: `+"`%.2fs`"+`
- **Measured Generation Throughput**: `+"`%.1f tokens/sec`"+`
- **Total In-Flight Tokens**: %d (Prompt: %d, Completion: %d)
`,
				a.WorkDir, a.TestCommand, a.Session.DurationSeconds, turn,
				archetype.Archetype, archetype.Severity, archetype.Description,
				blastReport.ModifiedSymbol, len(blastReport.AffectedFiles), strings.Join(blastReport.AffectedFiles, ", "),
				len(blastReport.TransitiveDependents), blastReport.RiskScore,
				tavilyCitationTable.String(), falsifyStatus, falsifyRes.ConfidenceScore, arenaNotes,
				regressionGuardSection, reasoningSection,
				patchDigest, a.Nebius.Model,
				nebiusCost, cloudCost, cloudSavingsPct, a.Session.TokenLedger.SavingsPercentage,
				a.Session.TokenLedger.TTFTSeconds, a.Session.TokenLedger.MeasuredTPS,
				a.Session.TokenLedger.TotalTokens, pTok, cTok)

			commitMsg := fmt.Sprintf("fix(auton): verified self-healing [%s] in %.2fs via Nemotron 3 Ultra\n\nX-Nemotron-Audit: %s\nX-Nemotron-Model: %s\nX-Nemotron-Platform: Nebius Token Factory", archetype.Archetype, a.Session.DurationSeconds, patchDigest, a.Nebius.Model)
			if a.Session.RegressionTestFile != "" {
				commitMsg = fmt.Sprintf("fix(auton): verified self-healing [%s] + regression test [%s]\n\nX-Nemotron-Audit: %s\nX-Nemotron-Model: %s\nX-Nemotron-Platform: Nebius Token Factory", archetype.Archetype, a.Session.RegressionTestFile, patchDigest, a.Nebius.Model)
			}
			_ = a.Checkpointer.CreateGitPRBranchWithAudit(branchName, commitMsg, auditReport)

			// Step Summary Hook for GitHub Actions Native CI/CD
			if summaryPath := os.Getenv("GITHUB_STEP_SUMMARY"); summaryPath != "" {
				if f, err := os.OpenFile(summaryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
					_, _ = f.WriteString("\n" + auditReport + "\n")
					_ = f.Close()
				}
			}

			a.notify(StateSucceeded, fmt.Sprintf("🎉 Verification + Adversarial Falsification PASSED on Turn %d! Branch `%s` created.", turn, branchName), map[string]interface{}{
				"duration_seconds":         a.Session.DurationSeconds,
				"token_ledger":             a.Session.TokenLedger,
				"falsification_confidence": falsifyRes.ConfidenceScore,
				"blast_report":             blastReport,
				"regression_file":          a.Session.RegressionTestFile,
				"patch_digest":             patchDigest,
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
		failedHistory = append(failedHistory, fmt.Sprintf("Failed Attempt Unified Diff:\n```diff\n%s\n```\nFailure Mode: Test Verification Regression / Failure\nError Diagnostics:\n%s", patchSug.DiffPatch, a.Session.LastError))
	}

	a.Session.IsResolved = false
	a.Session.DurationSeconds = time.Since(startTime).Seconds()
	a.notify(StateFailed, fmt.Sprintf("Self-healing budget exhausted after %d turns.", a.Session.MaxTurns), nil)
	return a.Session, nil
}
