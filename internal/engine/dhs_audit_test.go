package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kujiu27/nemotron-healer-go/internal/ast"
	"github.com/kujiu27/nemotron-healer-go/internal/falsify"
)

func TestFinalizeSuccessfulHealing_AuditParity(t *testing.T) {
	tmpDir := t.TempDir()

	// Create sample file
	mainGo := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(mainGo, []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatalf("failed to write main.go: %v", err)
	}

	agent := NewAgent(tmpDir, "go test", 5, nil, nil)
	agent.startTime = time.Now().Add(-2 * time.Second)

	// Ensure git context is ready
	if err := agent.Checkpointer.EnsureGitContext(); err != nil {
		t.Fatalf("EnsureGitContext failed: %v", err)
	}

	samplePatch := "--- a/main.go\n+++ b/main.go\n@@ -1,3 +1,3 @@\n package main\n-func main() {}\n+func main() { /* healed */ }\n"
	falsifyRes := &falsify.FalsificationResult{
		Passed:        true,
		GeneratedTest: "package main\n\nimport \"testing\"\n\nfunc TestMain(t *testing.T) {}\n",
	}
	archetype := ArchetypeAnalysis{
		Archetype:   "GeneralAssertion",
		Severity:    "MEDIUM",
		Description: "Logical assertion failed in test",
	}
	blastReport := &ast.BlastRadiusReport{
		ModifiedSymbol:       "main",
		AffectedFiles:        []string{"main.go"},
		TransitiveDependents: []string{"main"},
		RiskScore:            0.25,
	}
	agent.Session.TavilyQueries = []string{"gjson empty string query"}
	agent.Session.TavilyAnswer = "GJSON path syntax supports empty string queries via the array operator."
	agent.Session.TavilyExtractURL = "https://github.com/tidwall/gjson"
	agent.Session.TavilyExtractBytes = 1845


	branchName := agent.finalizeSuccessfulHealing(
		"main.go",
		samplePatch,
		falsifyRes,
		archetype,
		blastReport,
		1,
		"deep reasoning trace",
		"Single-Agent Verifiable Falsification",
		"DHS Parallel Search (Branch: node_2, Depth: 1, Reward: 0.850)",
	)

	// 1. Session state must be marked resolved
	if !agent.Session.IsResolved {
		t.Errorf("expected Session.IsResolved = true")
	}

	// 2. Cryptographic Patch Digest must be SHA-256
	if !strings.HasPrefix(agent.Session.PatchDigest, "sha256:") || len(agent.Session.PatchDigest) != 71 {
		t.Errorf("expected 71-char sha256: digest, got %q", agent.Session.PatchDigest)
	}

	// 3. Permanent regression test file must be recorded
	if agent.Session.RegressionTestFile == "" {
		t.Errorf("expected non-empty Session.RegressionTestFile")
	}

	// 4. Git PR branch must exist and commit message must contain audit trailers
	cmd := exec.Command("git", "log", "-1", "--pretty=format:%B", branchName)
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log on branch %s failed: %v, output: %s", branchName, err, string(out))
	}

	commitMsg := string(out)
	if !strings.Contains(commitMsg, "X-Nemotron-Audit: "+agent.Session.PatchDigest) {
		t.Errorf("commit message missing X-Nemotron-Audit trailer with patch digest: %s", commitMsg)
	}
	if !strings.Contains(commitMsg, "X-Nemotron-Platform: Nebius Token Factory") {
		t.Errorf("commit message missing X-Nemotron-Platform trailer: %s", commitMsg)
	}
	if !strings.Contains(commitMsg, "DHS Parallel Search") {
		t.Errorf("commit message missing resolution mechanism description: %s", commitMsg)
	}

	// 5. Audit Report file must be committed into the PR branch
	cmdShow := exec.Command("git", "show", branchName+":HEAL_AUDIT_REPORT.md")
	cmdShow.Dir = tmpDir
	showOut, showErr := cmdShow.CombinedOutput()
	if showErr != nil {
		t.Fatalf("failed to show HEAL_AUDIT_REPORT.md on branch %s: %v, output: %s", branchName, showErr, string(showOut))
	}
	auditContent := string(showOut)
	if !strings.Contains(auditContent, "## ⚡ Nemotron-Healer Autonomous Verification Report") {
		t.Errorf("HEAL_AUDIT_REPORT.md missing Markdown header: %s", auditContent)
	}
	if !strings.Contains(auditContent, "DHS Parallel Search") {
		t.Errorf("HEAL_AUDIT_REPORT.md missing DHS Parallel Search mechanism: %s", auditContent)
	}
	if !strings.Contains(auditContent, agent.Session.PatchDigest) {
		t.Errorf("HEAL_AUDIT_REPORT.md missing patch digest: %s", auditContent)
	}
	if !strings.Contains(auditContent, "Tavily AI Synthesized Summary") {
		t.Errorf("HEAL_AUDIT_REPORT.md missing Tavily AI Synthesized Summary: %s", auditContent)
	}
	if !strings.Contains(auditContent, "Tavily Extract API (Deep Full-Text Grounding)") {
		t.Errorf("HEAL_AUDIT_REPORT.md missing Tavily Extract API section: %s", auditContent)
	}
	if !strings.Contains(auditContent, "1845 bytes") {
		t.Errorf("HEAL_AUDIT_REPORT.md missing 1845 bytes metric: %s", auditContent)
	}
}

func TestFinalizeSuccessfulHealing_NilFalsify(t *testing.T) {
	tmpDir := t.TempDir()

	mainGo := filepath.Join(tmpDir, "main.go")
	_ = os.WriteFile(mainGo, []byte("package main\n\nfunc main() {}\n"), 0644)

	agent := NewAgent(tmpDir, "go test", 3, nil, nil)
	_ = agent.Checkpointer.EnsureGitContext()

	samplePatch := "--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-package main\n+package main // patched\n"
	archetype := ArchetypeAnalysis{Archetype: "NullTypeError", Severity: "LOW"}

	branchName := agent.finalizeSuccessfulHealing(
		"main.go",
		samplePatch,
		nil, // nil falsify result
		archetype,
		nil, // nil blast report
		1,
		"",
		"",
		"",
	)

	if !agent.Session.IsResolved {
		t.Errorf("expected session resolved even with nil falsify/blast")
	}
	if agent.Session.RegressionTestFile != "" {
		t.Errorf("expected empty regression file when falsify is nil")
	}
	if branchName == "" {
		t.Errorf("expected non-empty branch name")
	}
}
