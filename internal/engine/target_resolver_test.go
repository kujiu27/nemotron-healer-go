package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveTargetLocation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "resolver_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create dummy files
	servicePy := filepath.Join(tmpDir, "service.py")
	testCascadePy := filepath.Join(tmpDir, "test_cascade.py")
	_ = os.WriteFile(servicePy, []byte("def run(): pass\n"), 0644)
	_ = os.WriteFile(testCascadePy, []byte("def test_it(): pass\n"), 0644)

	trace := `
Traceback (most recent call last):
  File "test_cascade.py", line 15, in test_deadlock
    res = await service.execute()
  File "service.py", line 42, in execute
    raise RuntimeError("Deadlock detected")
RuntimeError: Deadlock detected
`

	loc := ResolveTargetLocation(tmpDir, trace, nil)
	if loc.FilePath != "service.py" {
		t.Fatalf("expected service.py, got %s", loc.FilePath)
	}
	if loc.LineNum != 42 {
		t.Fatalf("expected line 42, got %d", loc.LineNum)
	}
	if loc.Symbol != "execute" {
		t.Fatalf("expected symbol execute, got %s", loc.Symbol)
	}
}

func TestResolveTargetLocation_EcosystemFallback(t *testing.T) {
	// 1. Go ecosystem fallback
	goDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(goDir, "go.mod"), []byte("module test\n"), 0644)
	_ = os.WriteFile(filepath.Join(goDir, "server.go"), []byte("package main\n"), 0644)
	goLoc := ResolveTargetLocation(goDir, "unrecognized error without file line references", nil)
	if goLoc.FilePath != "server.go" {
		t.Errorf("expected server.go for Go project, got %s", goLoc.FilePath)
	}

	// 2. Node/TS ecosystem fallback
	nodeDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(nodeDir, "package.json"), []byte("{}\n"), 0644)
	_ = os.WriteFile(filepath.Join(nodeDir, "index.ts"), []byte("export const x = 1;\n"), 0644)
	nodeLoc := ResolveTargetLocation(nodeDir, "syntax error", nil)
	if nodeLoc.FilePath != "index.ts" {
		t.Errorf("expected index.ts for Node project, got %s", nodeLoc.FilePath)
	}
}
