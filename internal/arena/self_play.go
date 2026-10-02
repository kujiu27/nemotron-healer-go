package arena

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kujiu27/nemotron-healer-go/internal/client"
	"github.com/kujiu27/nemotron-healer-go/internal/sandbox"
)

type AdversarialRoundResult struct {
	Round              int    `json:"round"`
	RedAttackVector    string `json:"red_attack_vector"`
	RedAttackCode      string `json:"red_attack_code"`
	AttackSucceeded    bool   `json:"attack_succeeded"`
	BlueDefensePatch   string `json:"blue_defense_patch,omitempty"`
	DefenseVerified    bool   `json:"defense_verified"`
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

// SelfPlay executes a multi-round attack/defend adversarial loop between Red (Fuzzer) and Blue (Hardener).
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

		lang := detectArenaLang(targetFile, a.TestCommand, round)

		// 1. Red Agent: Synthesize a targeted adversarial attack test
		redPrompt := fmt.Sprintf(`[ROLE: RED TEAM HOSTILE INVARIANT ATTACKER]
You are a world-class security & concurrency fuzzer.
Your objective: Write a targeted %s test that STRESS-TESTS and BREAKS the following code.
Focus on: race conditions under high concurrency, extreme boundary values, null/nil pointers, unhandled exceptions.

[TARGET CODE]
%s

RULES:
%s`, lang.Name, currentCode, lang.Rules)

		redMessages := []client.ChatMessage{
			{Role: "system", Content: "You are an elite Red-Team security researcher writing adversarial unit tests."},
			{Role: "user", Content: redPrompt},
		}

		redResp, _, _, err := a.Nebius.StreamCompletion(ctx, redMessages, nil)
		if err != nil {
			return results, false, err
		}

		attackTestCode := extractAttackCode(redResp, lang)
		if attackTestCode == "" {
			// Red failed to find an attack vector -> Equilibrium reached
			results = append(results, AdversarialRoundResult{
				Round:              round,
				RedAttackVector:    "No further vulnerable invariants identified.",
				AttackSucceeded:    false,
				EquilibriumReached: true,
			})
			return results, true, nil
		}

		// Execute Red's attack against the current code
		testFileName := lang.AttackFileName
		testFilePath := filepath.Join(a.WorkDir, testFileName)
		_ = os.WriteFile(testFilePath, []byte(attackTestCode), 0644)

		testCmd := a.buildTestCmd(lang, testFileName)
		attackExec, _ := a.Runner.Run(testCmd)
		_ = os.Remove(testFilePath)

		if attackExec.IsSuccess {
			// The current code already survived Red's attack!
			results = append(results, AdversarialRoundResult{
				Round:              round,
				RedAttackVector:    "Hostile test executed but was successfully repelled by existing defenses.",
				RedAttackCode:      attackTestCode,
				AttackSucceeded:    false,
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

type ArenaLangConfig struct {
	Name           string
	Fence          string
	AttackFileName string
	Rules          string
}

func detectArenaLang(targetFile, testCmd string, round int) ArenaLangConfig {
	ext := strings.ToLower(filepath.Ext(targetFile))
	lowerCmd := strings.ToLower(testCmd)

	if ext == ".go" || strings.Contains(lowerCmd, "go test") {
		return ArenaLangConfig{
			Name:           "Go",
			Fence:          "go",
			AttackFileName: fmt.Sprintf("arena_red_attack_r%d_test.go", round),
			Rules:          fmt.Sprintf("1. Output ONLY valid, runnable Go test code inside a ```go block.\n2. Use standard `func TestRedAttackR%d(t *testing.T)` in the same package as the target code.", round),
		}
	}
	if ext == ".ts" || ext == ".tsx" || ext == ".js" || strings.Contains(lowerCmd, "jest") || strings.Contains(lowerCmd, "vitest") || strings.Contains(lowerCmd, "npm test") {
		return ArenaLangConfig{
			Name:           "TypeScript/JavaScript",
			Fence:          "typescript",
			AttackFileName: fmt.Sprintf("red_attack_r%d.test.ts", round),
			Rules:          "1. Output ONLY valid, runnable test code inside a ```typescript or ```javascript block.\n2. Use standard `test(...)` or `it(...)`.",
		}
	}
	return ArenaLangConfig{
		Name:           "Python",
		Fence:          "python",
		AttackFileName: fmt.Sprintf("test__red_attack_r%d.py", round),
		Rules:          "1. Output ONLY valid, runnable pytest test code inside a ```python block.\n2. If testing async code, wrap coroutines in sync test functions using asyncio.run().\n3. Do NOT test trivial functionality—test the weakest invariants.",
	}
}

func (a *Arena) buildTestCmd(lang ArenaLangConfig, testFileName string) string {
	if lang.Name == "Go" {
		return "go test -v -run TestRedAttack ."
	}
	if strings.Contains(a.TestCommand, "pytest") {
		tokens := strings.Fields(a.TestCommand)
		return fmt.Sprintf("%s %s", tokens[0], testFileName)
	}
	return fmt.Sprintf("%s %s", a.TestCommand, testFileName)
}

func extractAttackCode(text string, lang ArenaLangConfig) string {
	reMain := regexp.MustCompile(fmt.Sprintf("(?s)```(?:%s|%s)?\r?\n(.*?)```", lang.Fence, strings.ToLower(lang.Name)))
	if m := reMain.FindStringSubmatch(text); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	reAny := regexp.MustCompile("(?s)```[a-zA-Z0-9_-]*\r?\n(.*?)```")
	if m := reAny.FindStringSubmatch(text); len(m) > 1 {
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
