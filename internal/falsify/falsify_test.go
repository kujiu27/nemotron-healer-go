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
