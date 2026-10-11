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
	// Verify precise ruleId matching archetype and compliance fields
	res := report.Runs[0].Results[0]
	if res.RuleID != "NH-CONCURRENCYRACE" {
		t.Errorf("expected RuleID NH-CONCURRENCYRACE, got %s", res.RuleID)
	}
	if res.RuleIndex != 0 {
		t.Errorf("expected RuleIndex 0, got %d", res.RuleIndex)
	}
	if len(report.Runs[0].Tool.Driver.Rules) == 0 || report.Runs[0].Tool.Driver.Rules[0].HelpURI == "" {
		t.Errorf("expected rule helpUri to be populated")
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

func TestExportSarif_NestedDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	sarifPath := filepath.Join(tmpDir, "nested", "sub", "report.sarif")

	session := &engine.HealingSession{
		SessionID:       "nested-test",
		IsResolved:      true,
		DurationSeconds: 0.5,
		CurrentTurn:     1,
		MaxTurns:        1,
		TargetFile:      "main.go",
	}

	err := ExportSarif(session, sarifPath)
	if err != nil {
		t.Fatalf("ExportSarif to nested directory failed: %v", err)
	}

	if _, err := os.Stat(sarifPath); err != nil {
		t.Fatalf("nested sarif file was not created: %v", err)
	}
}

func TestExportSarif_SpecialCharArchetypeSanitization(t *testing.T) {
	tmpDir := t.TempDir()
	sarifPath := filepath.Join(tmpDir, "sanitized.sarif")

	session := &engine.HealingSession{
		SessionID:       "sanitize-test",
		IsResolved:      false,
		LastError:       "deadlock timeout error trace",
		DefectArchetype: "ConcurrencyRace (Hard) / Mutex-Leak",
		TargetFile:      "src/lib.rs",
		TargetLine:      10,
	}

	if err := ExportSarif(session, sarifPath); err != nil {
		t.Fatalf("ExportSarif failed: %v", err)
	}

	data, err := os.ReadFile(sarifPath)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	var report SarifReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	ruleID := report.Runs[0].Results[0].RuleID
	if ruleID != "NH-CONCURRENCYRACE-HARD-MUTEX-LEAK" {
		t.Errorf("expected sanitized ruleID NH-CONCURRENCYRACE-HARD-MUTEX-LEAK, got %s", ruleID)
	}
	if report.Runs[0].Results[0].Level != "error" {
		t.Errorf("expected level error for unresolved session, got %s", report.Runs[0].Results[0].Level)
	}
}

func TestExportSarif_BlastAffectedLocations(t *testing.T) {
	tmpDir := t.TempDir()
	sarifPath := filepath.Join(tmpDir, "report_blast.sarif")

	session := &engine.HealingSession{
		SessionID:          "blast-test",
		IsResolved:         true,
		DurationSeconds:    1.0,
		TargetFile:         "pkg/engine.go",
		TargetLine:         42,
		BlastAffectedFiles: []string{"pkg/engine.go", "pkg/caller.go", "cmd/main.go"},
	}

	err := ExportSarif(session, sarifPath)
	if err != nil {
		t.Fatalf("ExportSarif failed: %v", err)
	}

	data, err := os.ReadFile(sarifPath)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	var report SarifReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	res := report.Runs[0].Results[0]
	if len(res.Locations) != 3 {
		t.Fatalf("expected 3 locations (primary target + 2 affected files), got %d", len(res.Locations))
	}
	if res.Locations[0].PhysicalLocation.ArtifactLocation.URI != "pkg/engine.go" {
		t.Errorf("expected primary location pkg/engine.go, got %s", res.Locations[0].PhysicalLocation.ArtifactLocation.URI)
	}
	if res.Locations[1].PhysicalLocation.ArtifactLocation.URI != "pkg/caller.go" {
		t.Errorf("expected secondary location pkg/caller.go, got %s", res.Locations[1].PhysicalLocation.ArtifactLocation.URI)
	}
	if res.Locations[2].PhysicalLocation.ArtifactLocation.URI != "cmd/main.go" {
		t.Errorf("expected secondary location cmd/main.go, got %s", res.Locations[2].PhysicalLocation.ArtifactLocation.URI)
	}
}

func TestExportSarif_CleanScanEmitsEmptyResults(t *testing.T) {
	tmpDir := t.TempDir()
	sarifPath := filepath.Join(tmpDir, "clean.sarif")

	session := &engine.HealingSession{
		SessionID:       "clean-test",
		IsResolved:      true,
		CurrentTurn:     0,
		AppliedPatches:  nil,
		DurationSeconds: 0.1,
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
		t.Fatalf("invalid json: %v", err)
	}

	if len(report.Runs[0].Results) != 0 {
		t.Fatalf("expected 0 results for clean scan, got %d: %+v", len(report.Runs[0].Results), report.Runs[0].Results)
	}
}
