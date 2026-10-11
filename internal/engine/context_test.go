package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectSourceContext_BlastPriorityAndPruning(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Target and caller files (from blast radius)
	targetCode := "package main\nfunc Broken() {}\n"
	callerCode := "package main\nfunc Caller() { Broken() }\n"
	rsCode := "pub fn rust_task() {}\n"
	tsxCode := "export const Component = () => <div />;\n"
	nodeModulesCode := "console.log('huge node_modules library');\n"
	gitFileCode := "package git\nfunc Secret() {}\n"

	_ = os.WriteFile(filepath.Join(tmpDir, "target.go"), []byte(targetCode), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "caller.go"), []byte(callerCode), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "lib.rs"), []byte(rsCode), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "App.tsx"), []byte(tsxCode), 0644)

	// Create directories that MUST be pruned
	_ = os.MkdirAll(filepath.Join(tmpDir, "node_modules", "pkg"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, "node_modules", "pkg", "heavy.js"), []byte(nodeModulesCode), 0644)

	_ = os.MkdirAll(filepath.Join(tmpDir, ".git", "objects"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, ".git", "ignored.go"), []byte(gitFileCode), 0644)

	agent := NewAgent(tmpDir, "go test", 2, nil, nil)

	// Prioritize target.go and caller.go as blast radius files
	ctxStr := agent.collectSourceContext("target.go", "caller.go")

	// Target and caller should be present
	if !strings.Contains(ctxStr, "--- target.go ---") {
		t.Errorf("expected target.go in context")
	}
	if !strings.Contains(ctxStr, "--- caller.go ---") {
		t.Errorf("expected caller.go in context")
	}

	// Rust and TSX files should be supported and collected
	if !strings.Contains(ctxStr, "--- lib.rs ---") {
		t.Errorf("expected lib.rs in context")
	}
	if !strings.Contains(ctxStr, "--- App.tsx ---") {
		t.Errorf("expected App.tsx in context")
	}

	// node_modules and .git must be pruned
	if strings.Contains(ctxStr, "node_modules") {
		t.Errorf("expected node_modules to be pruned, found in context")
	}
	if strings.Contains(ctxStr, ".git") {
		t.Errorf("expected .git to be pruned, found in context")
	}

	// Verify order: priority files appear first
	targetIdx := strings.Index(ctxStr, "--- target.go ---")
	callerIdx := strings.Index(ctxStr, "--- caller.go ---")
	rsIdx := strings.Index(ctxStr, "--- lib.rs ---")
	if targetIdx > callerIdx || callerIdx > rsIdx {
		t.Errorf("expected priority files to appear first, got targetIdx=%d, callerIdx=%d, rsIdx=%d", targetIdx, callerIdx, rsIdx)
	}
}

func TestHealingSession_BlastRadiusJSONSerialization(t *testing.T) {
	session := NewHealingSession("sess-1", ".", "go test", 2)
	session.BlastRiskScore = 0.85
	session.BlastAffectedFiles = []string{"pkg/engine.go", "pkg/caller.go"}

	data, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	score, ok := parsed["blast_risk_score"].(float64)
	if !ok || score != 0.85 {
		t.Errorf("expected blast_risk_score 0.85, got %v", parsed["blast_risk_score"])
	}

	affRaw, ok := parsed["blast_affected_files"].([]interface{})
	if !ok || len(affRaw) != 2 {
		t.Errorf("expected 2 affected files, got %v", parsed["blast_affected_files"])
	}
}
