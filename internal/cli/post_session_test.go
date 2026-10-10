package cli

import (
	"os"
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
