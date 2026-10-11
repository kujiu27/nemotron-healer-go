package ast

import (
	"os"
	"path/filepath"
	"strings"
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
	if len(report.DirectDependents) == 0 {
		t.Fatalf("expected transfer_money in DirectDependents of UserAccount.get_balance, got none")
	}
	if report.DirectDependents[0] != "account.py::transfer_money" {
		t.Errorf("expected account.py::transfer_money as dependent, got %s", report.DirectDependents[0])
	}
	promptCtx := report.FormatPromptContext()
	if !strings.Contains(promptCtx, "UserAccount.get_balance") {
		t.Errorf("expected prompt context to mention symbol, got: %s", promptCtx)
	}
	if !strings.Contains(promptCtx, "account.py::transfer_money") {
		t.Errorf("expected prompt context to mention caller transfer_money, got: %s", promptCtx)
	}
	if !strings.Contains(promptCtx, "Interface Invariant") {
		t.Errorf("expected prompt context to include Interface Invariant, got: %s", promptCtx)
	}
}

func TestFindSymbolAtLine(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ast_line_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	goCode := `package service

func ProcessOrder(id string) error {
	return nil
}

type Engine struct{}

func (e *Engine) RunTask(name string) {
	_ = name
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "order.go"), []byte(goCode), 0644); err != nil {
		t.Fatal(err)
	}

	graph := NewCodeGraph(tmpDir)
	if err := graph.BuildGraph(); err != nil {
		t.Fatalf("build graph failed: %v", err)
	}

	// Line 3 is inside ProcessOrder
	sym1 := graph.FindSymbolAtLine("order.go", 3)
	if sym1 != "ProcessOrder" {
		t.Errorf("expected ProcessOrder at line 3, got %q", sym1)
	}

	// Line 10 is inside Engine.RunTask
	sym2 := graph.FindSymbolAtLine("order.go", 10)
	if sym2 != "Engine.RunTask" {
		t.Errorf("expected Engine.RunTask at line 10, got %q", sym2)
	}

	// Line 1 is package declaration (outside any function)
	sym0 := graph.FindSymbolAtLine("order.go", 1)
	if sym0 != "" {
		t.Errorf("expected empty symbol at line 1, got %q", sym0)
	}
}
func TestCodeGraph_GoMethodResolution(t *testing.T) {
	tmpDir := t.TempDir()

	goCode := `package service

type Calculator struct{}

func (c *Calculator) Add(a, b int) int {
	return a + b
}

func Compute() int {
	calc := &Calculator{}
	return calc.Add(1, 2)
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "calc.go"), []byte(goCode), 0644); err != nil {
		t.Fatal(err)
	}

	graph := NewCodeGraph(tmpDir)
	if err := graph.BuildGraph(); err != nil {
		t.Fatalf("build graph failed: %v", err)
	}

	if _, ok := graph.Symbols["calc.go::Calculator.Add"]; !ok {
		t.Fatalf("expected calc.go::Calculator.Add symbol, got: %+v", graph.Symbols)
	}

	// Hint as bare method name "Add"
	report := graph.AnalyzeBlastRadius("calc.go", "Add")
	if len(report.DirectDependents) == 0 {
		t.Fatalf("expected Compute in DirectDependents of Calculator.Add, got none")
	}
	if report.DirectDependents[0] != "calc.go::Compute" {
		t.Errorf("expected calc.go::Compute as dependent, got %s", report.DirectDependents[0])
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

func TestBuildGraph_LongLineFileSurvivesScannerBuffer(t *testing.T) {
	tmpDir := t.TempDir()
	// Generate a JavaScript file with a line exceeding 64KB (e.g. 100KB minified code or long comment)
	longComment := "// " + strings.Repeat("x", 100*1024)
	jsCode := longComment + "\nfunction processLargePayload() { return true; }\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "bundle.js"), []byte(jsCode), 0644); err != nil {
		t.Fatal(err)
	}

	graph := NewCodeGraph(tmpDir)
	if err := graph.BuildGraph(); err != nil {
		t.Fatalf("BuildGraph failed on >64KB line in JS bundle: %v", err)
	}

	if _, ok := graph.Symbols["bundle.js::processLargePayload"]; !ok {
		t.Fatalf("expected bundle.js::processLargePayload extracted after long line, symbols: %+v", graph.Symbols)
	}
}

func TestRustASTExtraction(t *testing.T) {
	tmpDir := t.TempDir()
	rustCode := `pub struct Worker {
    id: u32,
}

impl Worker {
    pub fn process(&self) -> bool {
        execute_task()
    }
}

pub fn execute_task() -> bool {
    true
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "lib.rs"), []byte(rustCode), 0644); err != nil {
		t.Fatal(err)
	}

	graph := NewCodeGraph(tmpDir)
	if err := graph.BuildGraph(); err != nil {
		t.Fatalf("build graph failed: %v", err)
	}

	if _, ok := graph.Symbols["lib.rs::Worker"]; !ok {
		t.Fatalf("expected lib.rs::Worker extracted, got: %+v", graph.Symbols)
	}
	if _, ok := graph.Symbols["lib.rs::process"]; !ok {
		t.Fatalf("expected lib.rs::process extracted, got: %+v", graph.Symbols)
	}
	if _, ok := graph.Symbols["lib.rs::execute_task"]; !ok {
		t.Fatalf("expected lib.rs::execute_task extracted, got: %+v", graph.Symbols)
	}

	report := graph.AnalyzeBlastRadius("lib.rs", "execute_task")
	if len(report.AffectedFiles) == 0 {
		t.Fatalf("expected at least 1 affected file, got 0")
	}
}

func TestTSXASTExtraction(t *testing.T) {
	tmpDir := t.TempDir()
	tsxCode := `export default function App() {
    return renderContent()
}

export const Header = () => {
    return true
}

function renderContent() {
    return true
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "App.tsx"), []byte(tsxCode), 0644); err != nil {
		t.Fatal(err)
	}

	graph := NewCodeGraph(tmpDir)
	if err := graph.BuildGraph(); err != nil {
		t.Fatalf("build graph failed: %v", err)
	}

	if _, ok := graph.Symbols["App.tsx::App"]; !ok {
		t.Fatalf("expected App.tsx::App extracted, got: %+v", graph.Symbols)
	}
	if _, ok := graph.Symbols["App.tsx::Header"]; !ok {
		t.Fatalf("expected App.tsx::Header extracted, got: %+v", graph.Symbols)
	}
	if _, ok := graph.Symbols["App.tsx::renderContent"]; !ok {
		t.Fatalf("expected App.tsx::renderContent extracted, got: %+v", graph.Symbols)
	}

	report := graph.AnalyzeBlastRadius("App.tsx", "renderContent")
	if len(report.AffectedFiles) == 0 {
		t.Fatalf("expected at least 1 affected file, got 0")
	}
}
