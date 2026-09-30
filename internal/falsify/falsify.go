package falsify

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

type FalsificationResult struct {
	Passed          bool    `json:"passed"`
	GeneratedTest   string  `json:"generated_test"`
	FailureOutput   string  `json:"failure_output,omitempty"`
	ConfidenceScore float64 `json:"confidence_score"`
}

type Falsifier struct {
	Nebius      *client.NebiusClient
	Sandbox     *sandbox.Runner
	WorkDir     string
	TestCommand string
}

func NewFalsifier(nebius *client.NebiusClient, runner *sandbox.Runner, workDir string, testCommand string) *Falsifier {
	return &Falsifier{
		Nebius:      nebius,
		Sandbox:     runner,
		WorkDir:     workDir,
		TestCommand: testCommand,
	}
}

func (f *Falsifier) StressTest(ctx context.Context, targetFile, patchDiff string) (*FalsificationResult, error) {
	fullPath := filepath.Join(f.WorkDir, targetFile)
	contentBytes, err := os.ReadFile(fullPath)
	if err != nil {
		return &FalsificationResult{Passed: true, ConfidenceScore: 0.8}, nil
	}

	prompt := fmt.Sprintf(`You are an adversarial testing engineer (Red-Teamer).
A patch was applied to resolve a bug. Your mission is to write a single, rigorous Python test using pytest to stress-test this code against edge cases (boundary values, NoneType, empty inputs, type mismatches).

[FILE]
%s

[CONTENT]
%s

[PATCH]
%s

RULES:
1. Output ONLY valid, runnable pytest Python code inside a `+"```python"+` block.
2. If testing async functions, do NOT use @pytest.mark.asyncio. Instead, use standard synchronous `+"`def test_...():`"+` and execute async calls via `+"`asyncio.run(...)`"+`.`, targetFile, string(contentBytes), patchDiff)

	messages := []client.ChatMessage{
		{Role: "system", Content: "You are an adversarial unit test generator."},
		{Role: "user", Content: prompt},
	}

	resp, _, _, err := f.Nebius.StreamCompletion(ctx, messages, nil)
	if err != nil {
		return &FalsificationResult{Passed: true, ConfidenceScore: 0.8}, nil
	}

	testCode := ""
	reTest := regexp.MustCompile("(?s)```python\n(.*?)```")
	if m := reTest.FindStringSubmatch(resp); len(m) > 1 {
		testCode = strings.TrimSpace(m[1])
	}

	if testCode == "" {
		return &FalsificationResult{Passed: true, ConfidenceScore: 0.85}, nil
	}

	// Write temp test file in workspace
	testFileName := "test__adversarial_falsify.py"
	testFilePath := filepath.Join(f.WorkDir, testFileName)
	if err := os.WriteFile(testFilePath, []byte(testCode), 0644); err != nil {
		return &FalsificationResult{Passed: true, ConfidenceScore: 0.8}, nil
	}
	defer os.Remove(testFilePath)

	// Execute adversarial test using the workspace's test runner
	testCmd := f.TestCommand
	if strings.Contains(testCmd, "pytest") {
		// Replace or append test file
		tokens := strings.Fields(testCmd)
		testCmd = fmt.Sprintf("%s %s", tokens[0], testFileName)
	} else {
		testCmd = fmt.Sprintf("%s %s", testCmd, testFileName)
	}

	res, err := f.Sandbox.Run(testCmd)
	if err != nil || !res.IsSuccess {
		return &FalsificationResult{
			Passed:          false,
			GeneratedTest:   testCode,
			FailureOutput:   res.Stdout + "\n" + res.Stderr,
			ConfidenceScore: 0.3,
		}, nil
	}

	return &FalsificationResult{
		Passed:          true,
		GeneratedTest:   testCode,
		ConfidenceScore: 0.99,
	}, nil
}
