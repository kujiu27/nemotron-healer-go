package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kujiu27/nemotron-healer-go/internal/ast"
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

func TestResolveTargetLocation_GoLineToSymbol(t *testing.T) {
	tmpDir := t.TempDir()
	goCode := `package cache

func Initialize() {
}

func (c *MetricCache) Record(key string, val int) {
	// line 8
	_ = key
}
`
	_ = os.WriteFile(filepath.Join(tmpDir, "cache.go"), []byte(goCode), 0644)

	graph := ast.NewCodeGraph(tmpDir)
	_ = graph.BuildGraph()

	trace := "cache.go:8: data race detected\ncache_test.go:12: test failed"
	loc := ResolveTargetLocation(tmpDir, trace, graph)

	if loc.FilePath != "cache.go" {
		t.Errorf("expected cache.go, got %s", loc.FilePath)
	}
	if loc.LineNum != 8 {
		t.Errorf("expected line 8, got %d", loc.LineNum)
	}
	if loc.Symbol != "MetricCache.Record" {
		t.Errorf("expected symbol MetricCache.Record from line 8, got %q", loc.Symbol)
	}
}
func TestResolveTargetLocation_RustTrace(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	testsDir := filepath.Join(tmpDir, "tests")
	_ = os.MkdirAll(srcDir, 0755)
	_ = os.MkdirAll(testsDir, 0755)

	_ = os.WriteFile(filepath.Join(srcDir, "calc.rs"), []byte("pub fn add() {}\n"), 0644)
	_ = os.WriteFile(filepath.Join(testsDir, "integration_test.rs"), []byte("#[test]\nfn test_it() {}\n"), 0644)

	// Compiler error trace
	compTrace := "error[E0308]: mismatched types\n  --> src/calc.rs:14:5\n   |\n14 |     a - b\n"
	locComp := ResolveTargetLocation(tmpDir, compTrace, nil)
	if locComp.FilePath != "src/calc.rs" || locComp.LineNum != 14 {
		t.Errorf("expected src/calc.rs:14 for Rust compiler trace, got %s:%d", locComp.FilePath, locComp.LineNum)
	}

	// Panic trace with test caller: must prioritize non-test src/calc.rs over tests/integration_test.rs
	panicTrace := "thread 'main' panicked at tests/integration_test.rs:10:5:\nassertion failed\n   at src/calc.rs:25:9\n"
	locPanic := ResolveTargetLocation(tmpDir, panicTrace, nil)
	if locPanic.FilePath != "src/calc.rs" || locPanic.LineNum != 25 {
		t.Errorf("expected non-test src/calc.rs:25 prioritized over test, got %s:%d", locPanic.FilePath, locPanic.LineNum)
	}
}
