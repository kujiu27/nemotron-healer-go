package ast

import (
	"fmt"
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

	report := graph.AnalyzeBlastRadius("account.py")
	if len(report.AffectedFiles) == 0 {
		t.Fatalf("expected at least 1 affected file, got 0")
	}
}

func TestFastAPISample(t *testing.T) {
	sampleDir := "/Users/fas/develop/pythonprojects/nemotron-healer/samples/fastapi_async_deadlock"
	graph := NewCodeGraph(sampleDir)
	if err := graph.BuildGraph(); err != nil {
		t.Fatal(err)
	}
	fmt.Printf("Symbols found in sample: %d\n", len(graph.Symbols))
	for k := range graph.Symbols {
		fmt.Println("Symbol:", k)
	}
}
