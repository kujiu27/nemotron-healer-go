package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
		TargetFile:      "pkg/engine.go",
		TargetLine:      42,
		TargetSymbol:    "mutateBalance",
		DefectArchetype: "ConcurrencyRace",
		PatchDigest:     "sha256:1234abcd",
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
	expectedVersion := strings.TrimPrefix(Version, "v")
	if report.Runs[0].Tool.Driver.Version != expectedVersion {
		t.Fatalf("expected tool driver version %s, got %s", expectedVersion, report.Runs[0].Tool.Driver.Version)
	}


	if len(report.Runs[0].Results) == 0 || report.Runs[0].Results[0].Level != "warning" {
		t.Fatalf("unexpected result level: %+v", report.Runs[0].Results)
	}

	// Verify precise ruleId matching archetype
	res := report.Runs[0].Results[0]
	if res.RuleID != "NH-CONCURRENCYRACE" {
		t.Errorf("expected RuleID NH-CONCURRENCYRACE, got %s", res.RuleID)
	}

	// Verify precise location mapping
	if len(res.Locations) != 1 {
		t.Fatalf("expected 1 location, got %d", len(res.Locations))
	}
	loc := res.Locations[0]
	if loc.PhysicalLocation.ArtifactLocation.URI != "pkg/engine.go" {
		t.Errorf("expected URI pkg/engine.go, got %s", loc.PhysicalLocation.ArtifactLocation.URI)
	}
	if loc.PhysicalLocation.Region.StartLine != 42 {
		t.Errorf("expected StartLine 42, got %d", loc.PhysicalLocation.Region.StartLine)
	}
}
