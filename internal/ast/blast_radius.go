package ast

import (
	"bufio"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type SymbolNode struct {
	Name      string   `json:"name"`
	Kind      string   `json:"kind"` // "class", "function", "type"
	FilePath  string   `json:"file_path"`
	LineStart int      `json:"line_start"`
	LineEnd   int      `json:"line_end"`
	Calls     []string `json:"calls"`
}

type BlastRadiusReport struct {
	ModifiedSymbol       string   `json:"modified_symbol"`
	SourceFile           string   `json:"source_file"`
	AffectedFiles        []string `json:"affected_files"`
	DirectDependents     []string `json:"direct_dependents"`
	TransitiveDependents []string `json:"transitive_dependents"`
	RiskScore            float64  `json:"risk_score"` // 0.0 - 1.0
}

type CodeGraph struct {
	WorkDir     string
	Symbols     map[string]*SymbolNode // "path::symbol"
	CallGraph   map[string][]string    // symbol -> callers
	FileSymbols map[string][]string    // path -> symbols
}

func NewCodeGraph(workDir string) *CodeGraph {
	return &CodeGraph{
		WorkDir:     workDir,
		Symbols:     make(map[string]*SymbolNode),
		CallGraph:   make(map[string][]string),
		FileSymbols: make(map[string][]string),
	}
}

// BuildGraph scans the workspace and extracts function/class definitions and calls.
func (g *CodeGraph) BuildGraph() error {
	g.Symbols = make(map[string]*SymbolNode)
	g.CallGraph = make(map[string][]string)
	g.FileSymbols = make(map[string][]string)

	pyFuncRegex := regexp.MustCompile(`^(?:async\s+)?def\s+([a-zA-Z0-9_]+)\s*\(`)
	pyClassRegex := regexp.MustCompile(`^class\s+([a-zA-Z0-9_]+)\b`)
	goFuncRegex := regexp.MustCompile(`^func\s+(?:\([^\)]+\)\s*)?([a-zA-Z0-9_]+)\s*\(`)
	tsFuncRegex := regexp.MustCompile(`^(?:export\s+)?(?:async\s+)?function\s+([a-zA-Z0-9_]+)\s*\(`)
	tsClassRegex := regexp.MustCompile(`^(?:export\s+)?class\s+([a-zA-Z0-9_]+)\b`)
	tsArrowRegex := regexp.MustCompile(`^(?:export\s+)?const\s+([a-zA-Z0-9_]+)\s*=\s*(?:async\s*)?\(`)

	callRegex := regexp.MustCompile(`\b([a-zA-Z0-9_]+)\s*\(`)

	err := filepath.Walk(g.WorkDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(g.WorkDir, path)
		if rel == "." {
			return nil
		}
		if (strings.HasPrefix(rel, ".") && rel != ".") || strings.Contains(rel, "/.") ||
			strings.Contains(rel, "venv") || strings.Contains(rel, "node_modules") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if info.IsDir() {
			return nil
		}

		ext := filepath.Ext(path)
		if ext != ".py" && ext != ".go" && ext != ".ts" && ext != ".js" {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		lineNum := 0

		var currentSym *SymbolNode

		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			trimmed := strings.TrimSpace(line)

			// Function or class match
			var name, kind string
			if ext == ".py" {
				if m := pyFuncRegex.FindStringSubmatch(trimmed); len(m) > 1 {
					name = m[1]
					kind = "function"
				} else if m := pyClassRegex.FindStringSubmatch(trimmed); len(m) > 1 {
					name = m[1]
					kind = "class"
				}
			} else if ext == ".go" {
				if m := goFuncRegex.FindStringSubmatch(trimmed); len(m) > 1 {
					name = m[1]
					kind = "function"
				}
			} else if ext == ".ts" || ext == ".js" {
				if m := tsFuncRegex.FindStringSubmatch(trimmed); len(m) > 1 {
					name = m[1]
					kind = "function"
				} else if m := tsClassRegex.FindStringSubmatch(trimmed); len(m) > 1 {
					name = m[1]
					kind = "class"
				} else if m := tsArrowRegex.FindStringSubmatch(trimmed); len(m) > 1 {
					name = m[1]
					kind = "function"
				}
			}

			if name != "" {
				if currentSym != nil {
					currentSym.LineEnd = lineNum - 1
				}
				key := rel + "::" + name
				currentSym = &SymbolNode{
					Name:      name,
					Kind:      kind,
					FilePath:  rel,
					LineStart: lineNum,
					LineEnd:   lineNum,
					Calls:     make([]string, 0),
				}
				g.Symbols[key] = currentSym
				g.FileSymbols[rel] = append(g.FileSymbols[rel], key)
			}

			// Capture calls
			if currentSym != nil {
				matches := callRegex.FindAllStringSubmatch(trimmed, -1)
				for _, match := range matches {
					if len(match) > 1 && match[1] != currentSym.Name {
						currentSym.Calls = append(currentSym.Calls, match[1])
					}
				}
			}
		}

		if currentSym != nil {
			currentSym.LineEnd = lineNum
		}

		return nil
	})

	// Invert edges into caller graph
	for symKey, sym := range g.Symbols {
		for _, called := range sym.Calls {
			targetKey := g.resolveSymbol(called, sym.FilePath)
			if targetKey != "" {
				g.CallGraph[targetKey] = append(g.CallGraph[targetKey], symKey)
			}
		}
	}

	return err
}

func (g *CodeGraph) resolveSymbol(calledName, currentFile string) string {
	sameFileKey := currentFile + "::" + calledName
	if _, ok := g.Symbols[sameFileKey]; ok {
		return sameFileKey
	}
	for key, sym := range g.Symbols {
		if sym.Name == calledName {
			return key
		}
	}
	return ""
}

// AnalyzeBlastRadius calculates the impact of changing a symbol or file.
func (g *CodeGraph) AnalyzeBlastRadius(targetFile string, targetSymbolHint ...string) *BlastRadiusReport {
	symbolsInFile := g.FileSymbols[targetFile]
	targetSym := "<file_scope>"
	if len(symbolsInFile) > 0 {
		targetSym = symbolsInFile[0]
	}

	// If explicit symbol hint provided, locate exact matching symbol
	if len(targetSymbolHint) > 0 && targetSymbolHint[0] != "" {
		hint := targetSymbolHint[0]
		expectedKey := targetFile + "::" + hint
		found := false
		for _, sym := range symbolsInFile {
			if sym == expectedKey || strings.HasSuffix(sym, "::"+hint) {
				targetSym = sym
				found = true
				break
			}
		}
		if !found {
			if _, ok := g.Symbols[hint]; ok {
				targetSym = hint
			} else {
				targetSym = expectedKey
			}
		}
	} else if len(symbolsInFile) > 1 {
		// Prefer symbol with existing callers if none explicitly specified
		for _, s := range symbolsInFile {
			if len(g.CallGraph[s]) > 0 {
				targetSym = s
				break
			}
		}
	}
	direct := g.CallGraph[targetSym]
	transitiveMap := make(map[string]bool)
	queue := append([]string{}, direct...)

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		if !transitiveMap[curr] {
			transitiveMap[curr] = true
			queue = append(queue, g.CallGraph[curr]...)
		}
	}

	affectedFilesMap := map[string]bool{targetFile: true}
	var transitiveList []string
	for k := range transitiveMap {
		transitiveList = append(transitiveList, k)
		if idx := strings.Index(k, "::"); idx != -1 {
			affectedFilesMap[k[:idx]] = true
		}
	}

	var affectedFiles []string
	for f := range affectedFilesMap {
		affectedFiles = append(affectedFiles, f)
	}

	total := float64(len(g.Symbols))
	if total == 0 {
		total = 1
	}
	score := math.Min(1.0, float64(len(transitiveList)+len(affectedFiles))/(total*0.5))

	return &BlastRadiusReport{
		ModifiedSymbol:       targetSym,
		SourceFile:           targetFile,
		AffectedFiles:        affectedFiles,
		DirectDependents:     direct,
		TransitiveDependents: transitiveList,
		RiskScore:            math.Round(score*100) / 100,
	}
}
