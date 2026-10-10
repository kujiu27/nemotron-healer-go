package cli

import (
	"os"
	"encoding/json"
	"bytes"
	"path/filepath"
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
