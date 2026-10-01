package ast

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCodeGraphExtraction(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ast_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	pyCode := `class UserAccount:
    def get_balance(self):
        return 100

def transfer_money():
    acc = UserAccount()
    return acc.get_balance()
`
	if err := os.WriteFile(filepath.Join(tmpDir, "account.py"), []byte(pyCode), 0644); err != nil {
		t.Fatal(err)
	}

	graph := NewCodeGraph(tmpDir)
	if err := graph.BuildGraph(); err != nil {
		t.Fatalf("build graph failed: %v", err)
	}

	if len(graph.Symbols) < 2 {
		t.Fatalf("expected at least 2 symbols extracted, got %d", len(graph.Symbols))
	}

	if _, ok := graph.Symbols["account.py::UserAccount.get_balance"]; !ok {
		t.Fatalf("expected account.py::UserAccount.get_balance, got: %+v", graph.Symbols)
	}

	report := graph.AnalyzeBlastRadius("account.py", "UserAccount.get_balance")
	if len(report.AffectedFiles) == 0 {
		t.Fatalf("expected at least 1 affected file, got 0")
	}
}

func TestFastAPISample(t *testing.T) {
	sampleDir := filepath.Join("..", "..", "samples", "fastapi_async_deadlock")
	if _, err := os.Stat(sampleDir); err != nil {
		t.Skip("sample directory not found, skipping sample test")
	}
	graph := NewCodeGraph(sampleDir)
	if err := graph.BuildGraph(); err != nil {
		t.Fatal(err)
	}
	if len(graph.Symbols) == 0 {
		t.Fatalf("expected symbols found in sample, got 0")
	}
}

func TestGoASTExtraction(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ast_go_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	goCode := `package service

type Engine struct {
	id string
}

func (e *Engine) Start() bool {
	return e.boot()
}

func (e *Engine) boot() bool {
	return true
}

func Run() {
	eng := &Engine{}
	eng.Start()
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "engine.go"), []byte(goCode), 0644); err != nil {
		t.Fatal(err)
	}

	graph := NewCodeGraph(tmpDir)
	if err := graph.BuildGraph(); err != nil {
		t.Fatalf("build graph failed: %v", err)
	}

	if _, ok := graph.Symbols["engine.go::Engine.Start"]; !ok {
		t.Fatalf("expected engine.go::Engine.Start extracted, got: %+v", graph.Symbols)
	}

	report := graph.AnalyzeBlastRadius("engine.go", "Engine.boot")
	if report.RiskScore == 0 {
		t.Logf("Engine.boot risk score: %f", report.RiskScore)
	}
}
