package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectTestCommand(t *testing.T) {
	t.Run("Detect Go project", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "detect_go_*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		_ = os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module example.com/test\n\ngo 1.22\n"), 0644)
		cmd, eco, err := DetectTestCommand(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cmd != "go test ./..." {
			t.Fatalf("expected 'go test ./...', got %q", cmd)
		}
		if !strings.Contains(eco, "Go") {
			t.Fatalf("expected Go ecosystem, got %q", eco)
		}
	})

	t.Run("Detect Go package tests without go.mod", func(t *testing.T) {
		tmpDir := t.TempDir()
		_ = os.WriteFile(filepath.Join(tmpDir, "lib.go"), []byte("package lib\n"), 0644)
		_ = os.WriteFile(filepath.Join(tmpDir, "lib_test.go"), []byte("package lib\nimport \"testing\"\nfunc TestLib(t *testing.T){}\n"), 0644)
		cmd, eco, err := DetectTestCommand(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cmd != "go test ." {
			t.Fatalf("expected 'go test .', got %q", cmd)
		}
		if !strings.Contains(eco, "Go") {
			t.Fatalf("expected Go ecosystem, got %q", eco)
		}
	})

	t.Run("Detect Python project", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "detect_py_*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		_ = os.WriteFile(filepath.Join(tmpDir, "pyproject.toml"), []byte("[tool.pytest.ini_options]\n"), 0644)
		cmd, eco, err := DetectTestCommand(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(cmd, "pytest") {
			t.Fatalf("expected pytest command, got %q", cmd)
		}
		if !strings.Contains(eco, "Python") {
			t.Fatalf("expected Python ecosystem, got %q", eco)
		}
	})

	t.Run("Detect Python project with Windows Scripts/pytest.exe", func(t *testing.T) {
		tmpDir := t.TempDir()
		_ = os.WriteFile(filepath.Join(tmpDir, "pyproject.toml"), []byte("[tool.pytest]\n"), 0644)
		scriptsDir := filepath.Join(tmpDir, ".venv", "Scripts")
		_ = os.MkdirAll(scriptsDir, 0755)
		pytestExe := filepath.Join(scriptsDir, "pytest.exe")
		_ = os.WriteFile(pytestExe, []byte("fake"), 0755)

		cmd, eco, err := DetectTestCommand(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(cmd, "pytest") {
			t.Fatalf("expected pytest in command, got %q", cmd)
		}
		if !strings.Contains(eco, "Python") {
			t.Fatalf("expected Python ecosystem, got %q", eco)
		}
	})

	t.Run("Detect Rust project", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "detect_rs_*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		_ = os.WriteFile(filepath.Join(tmpDir, "Cargo.toml"), []byte("[package]\nname = \"demo\"\n"), 0644)
		cmd, eco, err := DetectTestCommand(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cmd != "cargo test" {
			t.Fatalf("expected 'cargo test', got %q", cmd)
		}
		if !strings.Contains(eco, "Rust") {
			t.Fatalf("expected Rust ecosystem, got %q", eco)
		}
	})

	t.Run("Detect Node.js project", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "detect_node_*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		_ = os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(`{"name":"app","scripts":{"test":"jest"}}`), 0644)
		cmd, eco, err := DetectTestCommand(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cmd != "npm test" {
			t.Fatalf("expected 'npm test', got %q", cmd)
		}
		if !strings.Contains(eco, "Node.js") {
			t.Fatalf("expected Node ecosystem, got %q", eco)
		}
	})

	t.Run("Detect Makefile test target", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "detect_make_*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		_ = os.WriteFile(filepath.Join(tmpDir, "Makefile"), []byte("all:\n\t@echo build\n\ntest:\n\t@echo test\n"), 0644)
		cmd, eco, err := DetectTestCommand(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cmd != "make test" {
			t.Fatalf("expected 'make test', got %q", cmd)
		}
		if !strings.Contains(eco, "Make") {
			t.Fatalf("expected Make ecosystem, got %q", eco)
		}
	})

	t.Run("Unknown project returns error", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "detect_empty_*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		_, _, err = DetectTestCommand(tmpDir)
		if err == nil {
			t.Fatal("expected error on empty directory, got nil")
		}
	})
}
