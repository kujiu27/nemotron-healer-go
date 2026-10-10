package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DetectTestCommand inspects the workspace directory and automatically discovers
// the appropriate test runner command and ecosystem without requiring manual configuration.
func DetectTestCommand(workDir string) (command string, ecosystem string, err error) {
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return "", "", err
	}

	// 1. Go Ecosystem
	if fileExists(filepath.Join(absWorkDir, "go.mod")) {
		return "go test ./...", "Go (go.mod)", nil
	}
	if hasGoTestFiles(absWorkDir) {
		return "go test .", "Go (Package / Tests)", nil
	}


	// 2. Python Ecosystem
	pyMarkers := []string{"pytest.ini", "pyproject.toml", "setup.cfg", "requirements.txt", "Pipfile"}
	hasPyMarker := false
	for _, m := range pyMarkers {
		if fileExists(filepath.Join(absWorkDir, m)) {
			hasPyMarker = true
			break
		}
	}
	if !hasPyMarker {
		// Check for presence of test_*.py or *_test.py in root or tests/ dir
		if hasPythonTestFiles(absWorkDir) {
			hasPyMarker = true
		}
	}

	if hasPyMarker {
		pytestCmd := "pytest"
		if _, lookErr := exec.LookPath("pytest"); lookErr != nil {
			// Check local virtual environments
			venvCandidates := []string{
				filepath.Join(absWorkDir, ".venv", "bin", "pytest"),
				filepath.Join(absWorkDir, "venv", "bin", "pytest"),
			}
			foundVenv := false
			for _, v := range venvCandidates {
				if fileExists(v) {
					pytestCmd = v
					foundVenv = true
					break
				}
			}
			if !foundVenv {
				pytestCmd = "python3 -m pytest"
			}
		}
		return pytestCmd, "Python (Pytest / Virtualenv)", nil
	}

	// 3. Rust Ecosystem
	if fileExists(filepath.Join(absWorkDir, "Cargo.toml")) {
		return "cargo test", "Rust (Cargo.toml)", nil
	}

	// 4. Node.js / TypeScript Ecosystem
	pkgJSONPath := filepath.Join(absWorkDir, "package.json")
	if fileExists(pkgJSONPath) {
		if data, err := os.ReadFile(pkgJSONPath); err == nil {
			var pkg struct {
				Scripts map[string]string `json:"scripts"`
			}
			if json.Unmarshal(data, &pkg) == nil && pkg.Scripts != nil {
				if _, ok := pkg.Scripts["test"]; ok {
					return "npm test", "Node.js (package.json test script)", nil
				}
			}
		}
	}

	// 5. Makefile
	for _, mf := range []string{"Makefile", "makefile", "GNUmakefile"} {
		mfPath := filepath.Join(absWorkDir, mf)
		if fileExists(mfPath) {
			if data, err := os.ReadFile(mfPath); err == nil {
				content := string(data)
				lines := strings.Split(content, "\n")
				for _, line := range lines {
					trimmed := strings.TrimSpace(line)
					if strings.HasPrefix(trimmed, "test:") {
						return "make test", "Make (Makefile test target)", nil
					}
				}
			}
		}
	}

	// 6. Java / JVM Ecosystem
	if fileExists(filepath.Join(absWorkDir, "pom.xml")) {
		return "mvn test", "Java (Maven pom.xml)", nil
	}
	if fileExists(filepath.Join(absWorkDir, "build.gradle")) || fileExists(filepath.Join(absWorkDir, "build.gradle.kts")) {
		return "gradle test", "JVM (Gradle build.gradle)", nil
	}

	return "", "", fmt.Errorf("no test framework or build file identified in %s", workDir)
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func hasPythonTestFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() && strings.HasSuffix(name, ".py") && (strings.HasPrefix(name, "test_") || strings.HasSuffix(name, "_test.py")) {
			return true
		}
		if e.IsDir() && (name == "tests" || name == "test") {
			return true
		}
	}
	return false
}

func hasGoTestFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() && strings.HasSuffix(name, "_test.go") {
			return true
		}
	}
	return false
}
