package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kujiu27/nemotron-healer-go/internal/client"
	"github.com/kujiu27/nemotron-healer-go/internal/engine"
)

func TestHandlePostSession_SarifExport(t *testing.T) {
	tmpDir := t.TempDir()
	sarifPath := filepath.Join(tmpDir, "test.sarif")

	origSarif := sarifFlag
	sarifFlag = sarifPath
	defer func() {
		sarifFlag = origSarif
	}()

	session := &engine.HealingSession{
		IsResolved:      true,
		DurationSeconds: 1.23,
		CurrentTurn:     1,
		MaxTurns:        2,
		AppliedPatches:  []string{"--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-broken\n+fixed\n"},
		TargetFile:      "main.go",
	}

	agent := &engine.Agent{
		Nebius: &client.NebiusClient{Model: "nvidia/Nemotron-3-Ultra-550b-a55b"},
	}

	var humanBuf bytes.Buffer
	err := handlePostSession(session, agent, tmpDir, "main", &humanBuf)
	if err != nil {
		t.Fatalf("unexpected error from handlePostSession: %v", err)
	}

	if _, statErr := os.Stat(sarifPath); statErr != nil {
		t.Fatalf("expected sarif file exported to %s, got err: %v", sarifPath, statErr)
	}
}

func TestHandlePostSession_GitHubOutput(t *testing.T) {
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "github_output.txt")
	t.Setenv("GITHUB_OUTPUT", outputFile)

	session := &engine.HealingSession{
		SessionID:       "test-session-123",
		IsResolved:      true,
		DurationSeconds: 2.34,
		CurrentTurn:     1,
		MaxTurns:        3,
		AppliedPatches:  []string{"--- a/code.go\n+++ b/code.go\n"},
		PatchDigest:     "sha256:abc12345",
		DefectArchetype: "ResourceLeak",
		TargetFile:      "code.go",
	}

	agent := &engine.Agent{
		Nebius: &client.NebiusClient{Model: "nvidia/Nemotron-3-Ultra-550b-a55b"},
	}

	var humanBuf bytes.Buffer
	err := handlePostSession(session, agent, tmpDir, "main", &humanBuf)
	if err != nil {
		t.Fatalf("unexpected error from handlePostSession: %v", err)
	}

	data, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("failed to read GITHUB_OUTPUT: %v", err)
	}
	content := string(data)
	expectedSubstrings := []string{
		"is_resolved=true",
		"branch_name=fix/nemotron-heal-test-session-123",
		"turns_used=1",
		"patch_digest=sha256:abc12345",
		"defect_archetype=ResourceLeak",
		"duration_seconds=2.34",
		"patches_count=1",
	}
	for _, sub := range expectedSubstrings {
		if !bytes.Contains(data, []byte(sub)) {
			t.Errorf("expected GITHUB_OUTPUT to contain %q, got:\n%s", sub, content)
		}
	}
}

func TestHandlePostSession_JSONPurityIncludesBranchName(t *testing.T) {
	tmpDir := t.TempDir()
	origJSON := jsonFlag
	jsonFlag = true
	defer func() { jsonFlag = origJSON }()

	session := &engine.HealingSession{
		SessionID:       "sess-777",
		BranchName:      "fix/nemotron-heal-sess-777",
		IsResolved:      true,
		DurationSeconds: 1.0,
		CurrentTurn:     1,
		MaxTurns:        1,
	}

	agent := &engine.Agent{
		Nebius: &client.NebiusClient{Model: "nvidia/Nemotron-3-Ultra-550b-a55b"},
	}

	var humanBuf bytes.Buffer
	out := captureStdout(func() {
		_ = handlePostSession(session, agent, tmpDir, "main", &humanBuf)
	})

	var res struct {
		BranchName string `json:"branch_name"`
		IsResolved bool   `json:"is_resolved"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("stdout not valid JSON: %v, raw:\n%s", err, out)
	}
	if res.BranchName != "fix/nemotron-heal-sess-777" {
		t.Errorf("expected branch_name 'fix/nemotron-heal-sess-777', got %q", res.BranchName)
	}
}

func TestHandlePostSession_CleanRepo(t *testing.T) {
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "github_output.txt")
	t.Setenv("GITHUB_OUTPUT", outputFile)

	cleanText := "## ⚡ Nemotron-Healer Status: Repository Clean\n*All tests passed on initial reproduction check — no autonomous code repair required.*\n"
	session := &engine.HealingSession{
		SessionID:       "clean-session-999",
		IsResolved:      true,
		DurationSeconds: 0.85,
		CurrentTurn:     0,
		MaxTurns:        5,
		AppliedPatches:  nil,
		AuditReport:     cleanText,
	}

	agent := &engine.Agent{
		Nebius: &client.NebiusClient{Model: "nvidia/Nemotron-3-Ultra-550b-a55b"},
	}

	var humanBuf bytes.Buffer
	err := handlePostSession(session, agent, tmpDir, "main", &humanBuf)
	if err != nil {
		t.Fatalf("unexpected error from handlePostSession on clean repo: %v", err)
	}

	data, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("failed to read GITHUB_OUTPUT: %v", err)
	}
	content := string(data)
	expectedSubstrings := []string{
		"is_resolved=true\n",
		"branch_name=\n", // must NOT fabricate phantom branch on clean repo
		"turns_used=0\n",
		"patches_count=0\n",
	}
	for _, sub := range expectedSubstrings {
		if !strings.Contains(content, sub) {
			t.Errorf("expected GITHUB_OUTPUT to contain %q, got:\n%s", sub, content)
		}
	}

	if !strings.Contains(humanBuf.String(), "::notice title=Nemotron Repository Clean::") {
		t.Errorf("expected clean notice, got:\n%s", humanBuf.String())
	}

	reportData, rErr := os.ReadFile(filepath.Join(tmpDir, "HEAL_AUDIT_REPORT.md"))
	if rErr != nil || !strings.Contains(string(reportData), "Repository Clean") {
		t.Errorf("expected HEAL_AUDIT_REPORT.md to be written with clean summary, got err: %v, content: %s", rErr, string(reportData))
	}
}

func TestHandlePostSession_FailureDiagnostics(t *testing.T) {
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "github_output.txt")
	t.Setenv("GITHUB_OUTPUT", outputFile)

	failText := "## ⚡ Nemotron-Healer Failure Diagnostic Report\n*Autonomous self-healing could not verify a complete fix within 5 turns.*\n"
	session := &engine.HealingSession{
		SessionID:       "fail-session-404",
		IsResolved:      false,
		DurationSeconds: 12.34,
		CurrentTurn:     5,
		MaxTurns:        5,
		InitialError:    "panic: runtime error: invalid memory address",
		LastError:       "panic: runtime error: invalid memory address",
		DefectArchetype: "NullTypeError",
		AuditReport:     failText,
	}

	agent := &engine.Agent{
		Nebius: &client.NebiusClient{Model: "nvidia/Nemotron-3-Ultra-550b-a55b"},
	}

	var humanBuf bytes.Buffer
	err := handlePostSession(session, agent, tmpDir, "main", &humanBuf)
	if err == nil {
		t.Fatalf("expected error from handlePostSession on failed session, got nil")
	}

	data, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("failed to read GITHUB_OUTPUT: %v", err)
	}
	content := string(data)
	expectedSubstrings := []string{
		"is_resolved=false\n",
		"branch_name=\n",
		"turns_used=5\n",
		"defect_archetype=NullTypeError\n",
		"patches_count=0\n",
	}
	for _, sub := range expectedSubstrings {
		if !strings.Contains(content, sub) {
			t.Errorf("expected GITHUB_OUTPUT to contain %q, got:\n%s", sub, content)
		}
	}

	if !strings.Contains(humanBuf.String(), "::error title=Nemotron Self-Healing Failed::Could not verify fix within 5 turns") {
		t.Errorf("expected error notification, got:\n%s", humanBuf.String())
	}

	reportData, rErr := os.ReadFile(filepath.Join(tmpDir, "HEAL_AUDIT_REPORT.md"))
	if rErr != nil || !strings.Contains(string(reportData), "Failure Diagnostic Report") {
		t.Errorf("expected HEAL_AUDIT_REPORT.md to be written with failure report, got err: %v, content: %s", rErr, string(reportData))
	}
}
