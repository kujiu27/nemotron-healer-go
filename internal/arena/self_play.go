package arena

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nemotron-healer/nemotron-healer-go/internal/client"
	"github.com/nemotron-healer/nemotron-healer-go/internal/sandbox"
)

type AdversarialRoundResult struct {
	Round             int    `json:"round"`
	RedAttackVector   string `json:"red_attack_vector"`
	RedAttackCode     string `json:"red_attack_code"`
	AttackSucceeded   bool   `json:"attack_succeeded"`
	BlueDefensePatch  string `json:"blue_defense_patch,omitempty"`
	DefenseVerified   bool   `json:"defense_verified"`
	EquilibriumReached bool   `json:"equilibrium_reached"`
}

type Arena struct {
	Nebius      *client.NebiusClient
	Runner      *sandbox.Runner
	Patcher     *sandbox.Patcher
	WorkDir     string
	TestCommand string
}

func NewArena(nebius *client.NebiusClient, runner *sandbox.Runner, patcher *sandbox.Patcher, workDir, testCmd string) *Arena {
	return &Arena{
		Nebius:      nebius,
		Runner:      runner,
		Patcher:     patcher,
		WorkDir:     workDir,
		TestCommand: testCmd,
	}
}

// SelfPlay executes a multi-turn Minimax game between Red (Fuzzer) and Blue (Hardener).
func (a *Arena) SelfPlay(ctx context.Context, targetFile string, maxRounds int) ([]AdversarialRoundResult, bool, error) {
	if maxRounds <= 0 {
		maxRounds = 3
	}

	results := make([]AdversarialRoundResult, 0)
	fullPath := filepath.Join(a.WorkDir, targetFile)

	for round := 1; round <= maxRounds; round++ {
		codeBytes, err := os.ReadFile(fullPath)
		if err != nil {
			return results, false, err
		}
		currentCode := string(codeBytes)

		// 1. Red Agent: Synthesize a targeted adversarial attack test
		redPrompt := fmt.Sprintf(`[ROLE: RED TEAM HOSTILE INVARIANT ATTACKER]
You are a world-class security & concurrency fuzzer.
Your objective: Write a targeted pytest test that STRESS-TESTS and BREAKS the following code.
Focus on: race conditions under high concurrency, extreme boundary values, NoneType, unhandled exceptions.

[TARGET CODE]
%s

RULES:
1. Output ONLY valid, runnable pytest test code inside a `+"```python"+` block.
2. If testing async code, wrap coroutines in sync test functions using asyncio.run().
3. Do NOT test trivial functionality—test the weakest invariants.`, currentCode)

		redMessages := []client.ChatMessage{
			{Role: "system", Content: "You are an elite Red-Team security researcher writing adversarial unit tests."},
			{Role: "user", Content: redPrompt},
		}

		redResp, _, _, err := a.Nebius.StreamCompletion(ctx, redMessages, nil)
		if err != nil {
			return results, false, err
		}

		attackTestCode := extractPythonBlock(redResp)
		if attackTestCode == "" {
			// Red failed to find an attack vector -> Equilibrium reached
			results = append(results, AdversarialRoundResult{
				Round:             round,
				RedAttackVector:   "No further vulnerable invariants identified.",
				AttackSucceeded:   false,
				EquilibriumReached: true,
			})
			return results, true, nil
		}

		// Execute Red's attack against the current code
		testFileName := fmt.Sprintf("test__red_attack_r%d.py", round)
		testFilePath := filepath.Join(a.WorkDir, testFileName)
		_ = os.WriteFile(testFilePath, []byte(attackTestCode), 0644)

		testCmd := a.buildTestCmd(testFileName)
		attackExec, _ := a.Runner.Run(testCmd)
		_ = os.Remove(testFilePath)

		if attackExec.IsSuccess {
			// The current code already survived Red's attack!
			results = append(results, AdversarialRoundResult{
				Round:             round,
				RedAttackVector:   "Hostile test executed but was successfully repelled by existing defenses.",
				RedAttackCode:     attackTestCode,
				AttackSucceeded:   false,
				EquilibriumReached: true,
			})
			return results, true, nil
		}

		// Red successfully broke the code! Now Blue must harden it.
		bluePrompt := fmt.Sprintf(`[ROLE: BLUE TEAM DEFENSIVE HARDENER]
Red Team successfully broke your code with an adversarial invariant attack!

[TARGET CODE]
%s

[RED ATTACK TEST]
%s

[EXECUTION TRACE OF ATTACK FAILURE]
%s

Your objective: Generate a surgical Unified Diff patch on %s to fortify the code against this attack without breaking functional correctness.
Output the exact unified diff inside a `+"```diff"+` block.`, currentCode, attackTestCode, attackExec.Stdout+"\n"+attackExec.Stderr, targetFile)

		blueMessages := []client.ChatMessage{
			{Role: "system", Content: "You are a Principal Defensive Systems Engineer writing unbreakable, robust code."},
			{Role: "user", Content: bluePrompt},
		}

		blueResp, _, _, err := a.Nebius.StreamCompletion(ctx, blueMessages, nil)
		if err != nil {
			return results, false, err
		}

		patchDiff := extractDiffBlock(blueResp)
		if patchDiff == "" {
			results = append(results, AdversarialRoundResult{
				Round:           round,
				RedAttackCode:   attackTestCode,
				AttackSucceeded: true,
				DefenseVerified: false,
			})
			return results, false, fmt.Errorf("blue team failed to generate defense diff in round %d", round)
		}

		// Apply Blue's patch
		applied, _ := a.Patcher.ApplyPatch(patchDiff, targetFile)
		if !applied {
			return results, false, fmt.Errorf("failed to apply blue defense patch in round %d", round)
		}

		// Re-verify against Red attack test AND base tests
		_ = os.WriteFile(testFilePath, []byte(attackTestCode), 0644)
		reAttackExec, _ := a.Runner.Run(testCmd)
		baseExec, _ := a.Runner.Run(a.TestCommand)
		_ = os.Remove(testFilePath)

		roundResult := AdversarialRoundResult{
			Round:            round,
			RedAttackVector:  "Red identified concurrency/boundary invariant failure.",
			RedAttackCode:    attackTestCode,
			AttackSucceeded:  true,
			BlueDefensePatch: patchDiff,
			DefenseVerified:  reAttackExec.IsSuccess && baseExec.IsSuccess,
		}
		results = append(results, roundResult)

		if !roundResult.DefenseVerified {
			return results, false, fmt.Errorf("blue defense failed verification in round %d", round)
		}
	}

	return results, true, nil
}

func (a *Arena) buildTestCmd(testFileName string) string {
	if strings.Contains(a.TestCommand, "pytest") {
		tokens := strings.Fields(a.TestCommand)
		return fmt.Sprintf("%s %s", tokens[0], testFileName)
	}
	return fmt.Sprintf("%s %s", a.TestCommand, testFileName)
}

func extractPythonBlock(text string) string {
	re := regexp.MustCompile("(?s)```python\n(.*?)```")
	if m := re.FindStringSubmatch(text); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func extractDiffBlock(text string) string {
	re := regexp.MustCompile("(?s)```(?:diff|patch)?\r?\n(.*?)```")
	if m := re.FindStringSubmatch(text); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}
