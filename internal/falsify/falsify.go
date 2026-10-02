package falsify

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

type FalsificationResult struct {
	Passed          bool   `json:"passed"`
	Skipped         bool   `json:"skipped,omitempty"`
	Reason          string `json:"reason,omitempty"`
	IsMalformedTest bool   `json:"is_malformed_test,omitempty"`
	GeneratedTest   string `json:"generated_test"`
	FailureOutput   string `json:"failure_output,omitempty"`
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
		return &FalsificationResult{
			Passed: false,
			Reason: fmt.Sprintf("target file unreadable: %v", err),
		}, err
	}

	lang := detectLanguage(targetFile, f.TestCommand)
	prompt := fmt.Sprintf(`You are an adversarial testing engineer (Red-Teamer).
A patch was applied to resolve a bug. Your mission is to write a single, rigorous %s test to stress-test this code against edge cases (boundary values, null/nil pointers, empty inputs, type mismatches, concurrency hazards).

[FILE]
%s

[CONTENT]
%s

[PATCH]
%s

RULES:
%s`, lang.Name, targetFile, string(contentBytes), patchDiff, lang.Rules)

	messages := []client.ChatMessage{
		{Role: "system", Content: "You are an adversarial unit test generator."},
		{Role: "user", Content: prompt},
	}

	resp, _, _, err := f.Nebius.StreamCompletion(ctx, messages, nil)
	if err != nil {
		return &FalsificationResult{
			Passed:  true,
			Skipped: true,
			Reason:  fmt.Sprintf("adversarial generator unavailable: %v", err),
		}, nil
	}

	testCode := extractTestCode(resp, lang)
	if testCode == "" {
		return &FalsificationResult{
			Passed:  true,
			Skipped: true,
			Reason:  "model did not produce extractable test block",
		}, nil
	}

	// Write temp test file in workspace
	testFileName := lang.TestFileName
	testFilePath := filepath.Join(f.WorkDir, testFileName)
	if err := os.WriteFile(testFilePath, []byte(testCode), 0644); err != nil {
		return &FalsificationResult{
			Passed: false,
			Reason: fmt.Sprintf("failed writing test file: %v", err),
		}, err
	}
	defer os.Remove(testFilePath)

	// Execute adversarial test using the workspace's test runner
	testCmd := f.buildTestCmd(lang, testFileName)
	res, err := f.Sandbox.Run(testCmd)
	if err != nil || !res.IsSuccess {
		output := res.Stdout + "\n" + res.Stderr
		isMalformed := isTestSyntaxOrImportError(output, lang.Name)

		if isMalformed {
			return &FalsificationResult{
				Passed:          true,
				Skipped:         true,
				IsMalformedTest: true,
				Reason:          "synthesized test had invalid syntax/imports; skipped to prevent false-negative rollback",
				GeneratedTest:   testCode,
				FailureOutput:   output,
			}, nil
		}

		return &FalsificationResult{
			Passed:        false,
			GeneratedTest: testCode,
			FailureOutput: output,
		}, nil
	}

	return &FalsificationResult{
		Passed:        true,
		GeneratedTest: testCode,
	}, nil
}

func isTestSyntaxOrImportError(output, lang string) bool {
	lower := strings.ToLower(output)
	if lang == "Python" {
		return strings.Contains(lower, "syntaxerror") ||
			strings.Contains(lower, "indentationerror") ||
			strings.Contains(lower, "modulenotfounderror") ||
			strings.Contains(lower, "import error") ||
			strings.Contains(lower, "importerror") ||
			strings.Contains(lower, "fixture") && strings.Contains(lower, "not found")
	}
	if lang == "Go" {
		return strings.Contains(lower, "[build failed]") ||
			strings.Contains(lower, "syntax error") ||
			strings.Contains(lower, "undefined:") ||
			strings.Contains(lower, "cannot find package")
	}
	if lang == "TypeScript/JavaScript" {
		return strings.Contains(lower, "syntaxerror") ||
			strings.Contains(lower, "cannot find module")
	}
	return false
}

type LangConfig struct {
	Name         string
	Fence        string
	TestFileName string
	Rules        string
}

func detectLanguage(targetFile, testCmd string) LangConfig {
	ext := strings.ToLower(filepath.Ext(targetFile))
	lowerCmd := strings.ToLower(testCmd)

	if ext == ".go" || strings.Contains(lowerCmd, "go test") {
		return LangConfig{
			Name:         "Go",
			Fence:        "go",
			TestFileName: "adversarial_falsify_test.go",
			Rules:        "1. Output ONLY valid, runnable Go test code inside a ```go block.\n2. Use standard `func TestAdversarialFalsify(t *testing.T)` in the same package as the target file.",
		}
	}
	if ext == ".ts" || ext == ".tsx" || ext == ".js" || strings.Contains(lowerCmd, "jest") || strings.Contains(lowerCmd, "vitest") || strings.Contains(lowerCmd, "npm test") {
		return LangConfig{
			Name:         "TypeScript/JavaScript",
			Fence:        "typescript",
			TestFileName: "adversarial_falsify.test.ts",
			Rules:        "1. Output ONLY valid, runnable test code inside a ```typescript or ```javascript block.\n2. Use standard `test(...)` or `it(...)`.",
		}
	}
	return LangConfig{
		Name:         "Python",
		Fence:        "python",
		TestFileName: "test__adversarial_falsify.py",
		Rules:        "1. Output ONLY valid, runnable pytest Python code inside a ```python block.\n2. If testing async functions, do NOT use @pytest.mark.asyncio. Instead, use standard synchronous `def test_...():` and execute async calls via `asyncio.run(...)`.",
	}
}

func (f *Falsifier) buildTestCmd(lang LangConfig, testFileName string) string {
	if lang.Name == "Go" {
		return "go test -v -run TestAdversarialFalsify ."
	}
	if strings.Contains(f.TestCommand, "pytest") {
		tokens := strings.Fields(f.TestCommand)
		return fmt.Sprintf("%s %s", tokens[0], testFileName)
	}
	return fmt.Sprintf("%s %s", f.TestCommand, testFileName)
}

func extractTestCode(resp string, lang LangConfig) string {
	reMain := regexp.MustCompile(fmt.Sprintf("(?s)```(?:%s|%s)?\r?\n(.*?)```", lang.Fence, strings.ToLower(lang.Name)))
	if m := reMain.FindStringSubmatch(resp); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	reAny := regexp.MustCompile("(?s)```[a-zA-Z0-9_-]*\r?\n(.*?)```")
	if m := reAny.FindStringSubmatch(resp); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// PersistRegressionTest formats and permanently saves the verified test code into an enduring regression test file in the workspace
func (f *Falsifier) PersistRegressionTest(targetFile, testCode string) (string, error) {
	if strings.TrimSpace(testCode) == "" {
		return "", fmt.Errorf("empty regression test code")
	}

	lang := detectLanguage(targetFile, f.TestCommand)
	var regRelFile string

	switch lang.Name {
	case "Go":
		dir := filepath.Dir(targetFile)
		base := strings.TrimSuffix(filepath.Base(targetFile), ".go")
		if dir == "." || dir == "" {
			regRelFile = fmt.Sprintf("%s_nemotron_regression_test.go", base)
		} else {
			regRelFile = filepath.Join(dir, fmt.Sprintf("%s_nemotron_regression_test.go", base))
		}
	case "Rust":
		regRelFile = "tests/nemotron_regression_test.rs"
	case "TypeScript/JavaScript":
		regRelFile = "tests/nemotron_regression.test.ts"
	default: // Python
		if _, err := os.Stat(filepath.Join(f.WorkDir, "tests")); err == nil {
			regRelFile = "tests/test_nemotron_regression.py"
		} else {
			regRelFile = "test_nemotron_regression.py"
		}
	}

	// targetFile is LLM-controlled; the derived regression path must pass the
	// same sandbox gate as patches (traversal, .git, symlink containment).
	fullPath, err := sandbox.ValidateSafePath(f.WorkDir, regRelFile)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return "", err
	}

	header := "// [Nemotron-Healer] Autonomous Regression Invariant Guard\n// Synthesized by NVIDIA Nemotron 3 Ultra to permanently prevent regression.\n\n"
	if lang.Name == "Python" {
		header = "# [Nemotron-Healer] Autonomous Regression Invariant Guard\n# Synthesized by NVIDIA Nemotron 3 Ultra to permanently prevent regression.\n\n"
	}

	if err := os.WriteFile(fullPath, []byte(header+testCode+"\n"), 0644); err != nil {
		return "", err
	}

	return regRelFile, nil
}
