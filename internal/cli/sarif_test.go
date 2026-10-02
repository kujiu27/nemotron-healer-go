package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kujiu27/nemotron-healer-go/internal/engine"
)

func TestExportSarif(t *testing.T) {
	tmpDir := t.TempDir()
	sarifPath := filepath.Join(tmpDir, "report.sarif")

	session := &engine.HealingSession{
		SessionID:       "test-session",
		IsResolved:      true,
		DurationSeconds: 1.25,
		CurrentTurn:     1,
		MaxTurns:        3,
	}

	err := ExportSarif(session, sarifPath)
	if err != nil {
		t.Fatalf("ExportSarif failed: %v", err)
	}

	data, err := os.ReadFile(sarifPath)
	if err != nil {
		t.Fatalf("failed to read SARIF file: %v", err)
	}

	var report SarifReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("invalid SARIF JSON format: %v", err)
	}

	if report.Version != "2.1.0" {
		t.Fatalf("expected SARIF version 2.1.0, got %s", report.Version)
	}

	if len(report.Runs) == 0 || report.Runs[0].Tool.Driver.Name != "Nemotron-Healer" {
		t.Fatalf("tool driver name mismatch: %+v", report.Runs)
	}

	if len(report.Runs[0].Results) == 0 || report.Runs[0].Results[0].Level != "warning" {
		t.Fatalf("unexpected result level: %+v", report.Runs[0].Results)
	}
}
