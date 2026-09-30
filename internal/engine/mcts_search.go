package engine

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/nemotron-healer/nemotron-healer-go/internal/client"
	"github.com/nemotron-healer/nemotron-healer-go/internal/mcts"
)

type MCTSSearchResult struct {
	Resolved        bool
	SelectedNode    *mcts.Node
	NodesExplored   int
	SearchDepth     int
	BestReward      float64
	DurationSeconds float64
}

// RunMCTSSearch executes Test-Time Compute (TTC) Monte Carlo Tree Search across code mutation hypotheses.
func (a *Agent) RunMCTSSearch(ctx context.Context, initialFailingOutput string, budgetNodes int) (*MCTSSearchResult, error) {
	if budgetNodes <= 0 {
		budgetNodes = 6
	}

	start := time.Now()
	a.notify(StateDiagnosing, fmt.Sprintf("⚡ Launching Test-Time Compute (TTC) MCTS Search Engine (Node Budget: %d)...", budgetNodes), nil)

	// Step 0: Root node represents initial broken state
	root := mcts.NewNode("root", nil, "", "", "Initial Failing State")
	evaluator := mcts.DefaultRewardEvaluator()

	codeCtx := a.collectSourceContext()
	targetHint := "engine.py"
	for path := range a.CodeGraph.FileSymbols {
		if !strings.HasPrefix(path, "test_") && !strings.Contains(path, "test") {
			targetHint = path
			break
		}
	}
	blastReport := a.CodeGraph.AnalyzeBlastRadius(targetHint)
	archetype := ClassifyDefect(initialFailingOutput, codeCtx)

	query := fmt.Sprintf("how to fix %s %s", targetHint, archetype.Archetype)
	tavilyResp, _ := a.Tavily.Search(ctx, query, 3)
	docsCtx := a.Tavily.FormatContext(tavilyResp)

	nodesCreated := 0

	// Step 1: Expansion of Root into K=3 distinct architectural hypotheses
	hypotheses := []struct {
		Name     string
		Guidance string
	}{
		{
			Name:     "Hypothesis A: Re-entrant Task-Aware Synchronization",
			Guidance: "Implement an asyncio task-aware re-entrant lock subclassing the existing lock to prevent deadlocks in nested calls.",
		},
		{
			Name:     "Hypothesis B: Decoupled Critical Scope Isolation",
			Guidance: "Decouple the audit logging and release the lock before executing external I/O operations.",
		},
		{
			Name:     "Hypothesis C: Surgical Invariant Guard with Mutex",
			Guidance: "Guard the mutable balance attribute strictly with atomic compare-and-swap or standard context lock.",
		},
	}

	a.notify(StateSynthesizingPatch, fmt.Sprintf("MCTS Tree Expansion: Generating %d divergent patch branches...", len(hypotheses)), nil)

	for i, hyp := range hypotheses {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		childID := fmt.Sprintf("node_%d", i+1)
		a.notify(StateSynthesizingPatch, fmt.Sprintf("Expanding MCTS [%s]: %s", childID, hyp.Name), nil)

		// Create isolated checkpoint for this simulation rollout
		cpID, _ := a.Checkpointer.CreateCheckpoint()

		// Prompt Nemotron 3 Ultra with specific hypothesis guidance
		promptMessages := []client.ChatMessage{
			{Role: "system", Content: "You are an autonomous MCTS code synthesis engine exploring a specific hypothesis branch. Output exact, surgical unified diff patches."},
			{
				Role: "user",
				Content: fmt.Sprintf(`[HYPOTHESIS BRANCH]
%s: %s

[TARGET FILE]
%s

[DEFECT ARCHETYPE]
%s

[FAILING TRACE]
%s

[SOURCE CODE CONTEXT]
%s

[OFFICIAL DOCS]
%s

INSTRUCTIONS:
1. Generate an exact unified diff patch implementing THIS hypothesis inside a `+"```diff"+` block starting with --- a/%s and +++ b/%s.
2. Specify [TARGET_FILE]%s[/TARGET_FILE].`,
					hyp.Name, hyp.Guidance, targetHint, archetype.Archetype, initialFailingOutput, codeCtx, docsCtx, targetHint, targetHint, targetHint),
			},
		}

		rawResp, pTok, cTok, err := a.Nebius.StreamCompletion(ctx, promptMessages, a.OnStreamToken)
		a.Session.TokenLedger.PromptTokens += pTok
		a.Session.TokenLedger.CompletionTokens += cTok
		a.Session.TokenLedger.CalculateCost()

		diffPatch := ""
		targetFile := targetHint
		if err == nil {
			diffPatch = extractDiffBlock(rawResp)
			reTarget := regexp.MustCompile(`\[TARGET_FILE\](.*?)\[/TARGET_FILE\]`)
			if m := reTarget.FindStringSubmatch(rawResp); len(m) > 1 {
				targetFile = strings.TrimSpace(m[1])
			}
		}

		child := mcts.NewNode(childID, root, diffPatch, targetFile, hyp.Name)
		root.Children = append(root.Children, child)
		nodesCreated++

		if diffPatch == "" {
			a.notify(StateRollingBack, fmt.Sprintf("MCTS [%s] failed to synthesize valid diff. Pruning branch...", childID), nil)
			child.Backpropagate(-0.9)
			_ = a.Checkpointer.Rollback(cpID)
			continue
		}

		// Rollout: Apply patch in isolated sandbox
		applied, applyMsg := a.Patcher.ApplyPatch(diffPatch, targetFile)
		if !applied {
			a.notify(StateRollingBack, fmt.Sprintf("MCTS [%s] diff application failed (%s). Pruning branch...", childID, applyMsg), nil)
			child.Backpropagate(-0.8)
			_ = a.Checkpointer.Rollback(cpID)
			continue
		}

		// Rollout: Run verification test in sandbox
		verifyRes, _ := a.Runner.Run(a.TestCommand)
		advPass := false

		if verifyRes.IsSuccess {
			// Run Adversarial Falsification stress test
			falsifyRes, _ := a.Falsifier.StressTest(ctx, targetHint, diffPatch)
			advPass = falsifyRes.Passed
		}

		// Calculate Grounded Verifiable Reward
		reward := evaluator.ComputeReward(verifyRes.IsSuccess, advPass, blastReport.RiskScore, diffPatch)
		child.Backpropagate(reward)

		a.notify(StateVerifyingSandbox, fmt.Sprintf("MCTS Rollout [%s]: BasePass=%v, AdvPass=%v ➔ Grounded Reward: %.3f",
			childID, verifyRes.IsSuccess, advPass, reward), map[string]interface{}{
			"node_id":   childID,
			"reward":    reward,
			"base_pass": verifyRes.IsSuccess,
			"adv_pass":  advPass,
		})

		// If this branch achieves Pareto-optimal victory (>= 0.70)
		if reward >= 0.70 {
			a.notify(StateSucceeded, fmt.Sprintf("🎉 MCTS Search converged on optimal branch [%s] (%s) with Reward: %.3f!",
				childID, hyp.Name, reward), nil)

			a.Session.IsResolved = true
			a.Session.DurationSeconds = time.Since(start).Seconds()
			a.Session.AppliedPatches = append(a.Session.AppliedPatches, diffPatch)

			branchName := fmt.Sprintf("fix/nemotron-mcts-%s", a.Session.SessionID)
			_ = a.Checkpointer.CreateGitPRBranch(branchName, fmt.Sprintf("fix(mcts): verified repair via MCTS search branch [%s] (Reward: %.3f)", childID, reward))

			return &MCTSSearchResult{
				Resolved:        true,
				SelectedNode:    child,
				NodesExplored:   nodesCreated,
				SearchDepth:     1,
				BestReward:      reward,
				DurationSeconds: a.Session.DurationSeconds,
			}, nil
		}

		// Rollback sandbox to explore next branch
		_ = a.Checkpointer.Rollback(cpID)
	}

	// Select best child based on UCB1
	bestChild := root.BestChild(1.414)
	bestReward := 0.0
	if bestChild != nil && bestChild.Visits > 0 {
		bestReward = bestChild.TotalReward / float64(bestChild.Visits)
	}

	return &MCTSSearchResult{
		Resolved:        false,
		SelectedNode:    bestChild,
		NodesExplored:   nodesCreated,
		SearchDepth:     1,
		BestReward:      bestReward,
		DurationSeconds: time.Since(start).Seconds(),
	}, nil
}

func extractDiffBlock(text string) string {
	re := regexp.MustCompile("(?s)```(?:diff|patch)?\r?\n(.*?)```")
	if m := re.FindStringSubmatch(text); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	if strings.Contains(text, "--- a/") && strings.Contains(text, "+++ b/") {
		idx := strings.Index(text, "--- a/")
		return strings.TrimSpace(text[idx:])
	}
	return ""
}
