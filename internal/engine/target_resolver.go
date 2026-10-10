package engine

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/kujiu27/nemotron-healer-go/internal/ast"
)

type TargetLocation struct {
	FilePath string
	LineNum  int
	Symbol   string
}

var (
	pyTraceRegex = regexp.MustCompile(`File\s+"([^"]+)",\s+line\s+(\d+)(?:,\s+in\s+([a-zA-Z0-9_]+))?`)
	pyShortRegex = regexp.MustCompile(`([a-zA-Z0-9_\-./\\]+\.py):(\d+)(?::\s*(?:in\s+([a-zA-Z0-9_]+))?)?`)
	goTraceRegex = regexp.MustCompile(`([a-zA-Z0-9_\-./\\]+\.go):(\d+):`)
	tsTraceRegex = regexp.MustCompile(`(?:at\s+(?:async\s+)?([a-zA-Z0-9_.]+)\s+\()?([a-zA-Z0-9_\-./\\]+\.(?:ts|js|tsx|jsx)):(\d+):(\d+)\)?`)
)

func isTestFile(path string) bool {
	lower := strings.ToLower(filepath.ToSlash(path))
	base := filepath.Base(lower)
	return strings.HasPrefix(base, "test_") ||
		strings.HasSuffix(base, "_test.py") ||
		strings.HasSuffix(base, "_test.go") ||
		strings.HasSuffix(base, ".test.ts") ||
		strings.HasSuffix(base, ".test.js") ||
		strings.HasSuffix(base, ".spec.ts") ||
		strings.HasSuffix(base, ".spec.js") ||
		strings.Contains(lower, "/tests/") ||
		strings.Contains(lower, "/test/")
}

// ResolveTargetLocation scans an error trace for source files within the workspace.
// It prioritizes non-test source files appearing in the crash traceback.
func ResolveTargetLocation(workDir string, errorTrace string, graph *ast.CodeGraph) TargetLocation {
	absWorkDir, _ := filepath.Abs(workDir)

	normalizeRel := func(rawPath string) string {
		clean := filepath.Clean(rawPath)
		if filepath.IsAbs(clean) {
			if rel, err := filepath.Rel(absWorkDir, clean); err == nil && !strings.HasPrefix(rel, "..") {
				return rel
			}
		}
		// Try direct relative path inside workDir
		candidate := filepath.Join(absWorkDir, clean)
		if _, err := os.Stat(candidate); err == nil {
			return clean
		}
		// Also check base name inside workDir
		baseCand := filepath.Join(absWorkDir, filepath.Base(clean))
		if _, err := os.Stat(baseCand); err == nil {
			return filepath.Base(clean)
		}
		return clean
	}

	existsInWorkDir := func(rel string) bool {
		full := filepath.Join(absWorkDir, rel)
		info, err := os.Stat(full)
		return err == nil && !info.IsDir()
	}

	var candidates []TargetLocation

	// 1. Python trace regex (e.g. File "models.py", line 12, in ...)
	for _, m := range pyTraceRegex.FindAllStringSubmatch(errorTrace, -1) {
		rel := normalizeRel(m[1])
		if existsInWorkDir(rel) {
			line, _ := strconv.Atoi(m[2])
			sym := ""
			if len(m) > 3 {
				sym = m[3]
			}
			candidates = append(candidates, TargetLocation{FilePath: rel, LineNum: line, Symbol: sym})
		}
	}

	// 2. Python short regex (e.g. account.py:45: in transfer)
	for _, m := range pyShortRegex.FindAllStringSubmatch(errorTrace, -1) {
		rel := normalizeRel(m[1])
		if existsInWorkDir(rel) {
			line, _ := strconv.Atoi(m[2])
			sym := ""
			if len(m) > 3 {
				sym = m[3]
			}
			candidates = append(candidates, TargetLocation{FilePath: rel, LineNum: line, Symbol: sym})
		}
	}

	// 3. Go trace regex (e.g. server.go:34: ...)
	for _, m := range goTraceRegex.FindAllStringSubmatch(errorTrace, -1) {
		rel := normalizeRel(m[1])
		if existsInWorkDir(rel) {
			line, _ := strconv.Atoi(m[2])
			candidates = append(candidates, TargetLocation{FilePath: rel, LineNum: line})
		}
	}

	// 4. TypeScript / JavaScript trace regex
	for _, m := range tsTraceRegex.FindAllStringSubmatch(errorTrace, -1) {
		rel := normalizeRel(m[2])
		if existsInWorkDir(rel) {
			line, _ := strconv.Atoi(m[3])
			candidates = append(candidates, TargetLocation{FilePath: rel, LineNum: line, Symbol: m[1]})
		}
	}

	// Scan candidates in reverse (innermost stack frame first)
	// Prioritize non-test files
	for i := len(candidates) - 1; i >= 0; i-- {
		if !isTestFile(candidates[i].FilePath) {
			return candidates[i]
		}
	}

	// If only test files were in the trace, try callers or symbols referenced in graph
	if len(candidates) > 0 {
		return candidates[len(candidates)-1]
	}

	// Fallback to CodeGraph with deterministic alphabetical sort
	if graph != nil && len(graph.FileSymbols) > 0 {
		var sortedFiles []string
		for f := range graph.FileSymbols {
			sortedFiles = append(sortedFiles, f)
		}
		sort.Strings(sortedFiles)

		// First non-test file
		for _, f := range sortedFiles {
			if !isTestFile(f) {
				return TargetLocation{FilePath: f}
			}
		}
		return TargetLocation{FilePath: sortedFiles[0]}
	}

	// Ecosystem-aware default fallback
	if _, err := os.Stat(filepath.Join(absWorkDir, "go.mod")); err == nil {
		if _, err := os.Stat(filepath.Join(absWorkDir, "main.go")); err == nil {
			return TargetLocation{FilePath: "main.go"}
		}
		if matches, _ := filepath.Glob(filepath.Join(absWorkDir, "*.go")); len(matches) > 0 {
			for _, m := range matches {
				base := filepath.Base(m)
				if !isTestFile(base) {
					return TargetLocation{FilePath: base}
				}
			}
		}
		return TargetLocation{FilePath: "main.go"}
	}
	if _, err := os.Stat(filepath.Join(absWorkDir, "package.json")); err == nil {
		for _, cand := range []string{"index.ts", "index.js", "src/index.ts", "src/index.js"} {
			if _, err := os.Stat(filepath.Join(absWorkDir, cand)); err == nil {
				return TargetLocation{FilePath: cand}
			}
		}
		return TargetLocation{FilePath: "index.ts"}
	}
	if _, err := os.Stat(filepath.Join(absWorkDir, "Cargo.toml")); err == nil {
		return TargetLocation{FilePath: "src/main.rs"}
	}

	return TargetLocation{FilePath: "main.py"}
}
