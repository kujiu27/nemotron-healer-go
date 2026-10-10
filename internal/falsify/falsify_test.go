package falsify

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectLanguage(t *testing.T) {
	cfgPy := detectLanguage("account.py", "pytest")
	if cfgPy.Name != "Python" {
		t.Fatalf("expected Python, got %s", cfgPy.Name)
	}

	cfgGo := detectLanguage("main.go", "go test ./...")
	if cfgGo.Name != "Go" {
		t.Fatalf("expected Go, got %s", cfgGo.Name)
	}

	cfgTS := detectLanguage("component.tsx", "vitest run")
	if cfgTS.Name != "TypeScript/JavaScript" {
		t.Fatalf("expected TS/JS, got %s", cfgTS.Name)
	}
}

func TestPersistRegressionTest(t *testing.T) {
	tmpDir := t.TempDir()
	f := &Falsifier{WorkDir: tmpDir, TestCommand: "go test ./..."}

	rel, err := f.PersistRegressionTest("pkg/cache.go", "func TestCacheRegression(t *testing.T) {}")
	if err != nil {
		t.Fatalf("PersistRegressionTest failed: %v", err)
	}
	if rel != "pkg/cache_nemotron_regression_test.go" {
		t.Fatalf("unexpected regression test path: %s", rel)
	}

	// Python in tests/ dir
	pyDir := t.TempDir()
	_ = os.Mkdir(filepath.Join(pyDir, "tests"), 0755)
	fPy := &Falsifier{WorkDir: pyDir, TestCommand: "pytest"}
	relPy, errPy := fPy.PersistRegressionTest("app/server.py", "def test_invariant(): pass")
	if errPy != nil {
		t.Fatalf("PersistRegressionTest (py) failed: %v", errPy)
	}
	if relPy != "tests/test_nemotron_regression.py" {
		t.Fatalf("unexpected py regression test path: %s", relPy)
	}
}

func TestBuildTestCmd_SubpackageColocation(t *testing.T) {
	f := &Falsifier{TestCommand: "go test ./..."}
	langGo := detectLanguage("pkg/auth/token.go", "go test ./...")

	// 1. Subpackage Go target
	cmdGo := f.buildTestCmd(langGo, "adversarial_falsify_test.go", "pkg/auth")
	if cmdGo != "go test -v -run TestAdversarialFalsify ./pkg/auth" {
		t.Errorf("expected targeted subpackage test command, got %q", cmdGo)
	}

	// 2. Root Go target
	cmdRoot := f.buildTestCmd(langGo, "adversarial_falsify_test.go", ".")
	if cmdRoot != "go test -v -run TestAdversarialFalsify ." {
		t.Errorf("expected root test command, got %q", cmdRoot)
	}

	// 3. Subpackage Python target
	fPy := &Falsifier{TestCommand: "python3 -m pytest"}
	langPy := detectLanguage("backend/service.py", "python3 -m pytest")
	cmdPy := fPy.buildTestCmd(langPy, "test__adversarial_falsify.py", "backend")
	if cmdPy != "python3 -m pytest backend/test__adversarial_falsify.py" {
		t.Errorf("expected colocated pytest path, got %q", cmdPy)
	}
}
