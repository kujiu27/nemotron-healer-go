package ast

import (
	"bufio"
	"bytes"
	"encoding/json"
	goast "go/ast"
	"go/parser"
	"go/token"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var reservedKeywords = map[string]bool{
	"if": true, "else": true, "elif": true, "for": true, "while": true, "switch": true,
	"case": true, "default": true, "break": true, "continue": true, "return": true,
	"with": true, "as": true, "try": true, "except": true, "finally": true, "catch": true,
	"throw": true, "raise": true, "assert": true, "import": true, "from": true, "class": true,
	"def": true, "func": true, "function": true, "var": true, "let": true, "const": true,
	"package": true, "type": true, "struct": true, "interface": true, "select": true,
	"go": true, "defer": true, "make": true, "new": true, "len": true, "cap": true,
	"append": true, "print": true, "println": true, "range": true, "yield": true,
	"async": true, "await": true, "lambda": true, "pass": true, "in": true, "is": true,
	"not": true, "and": true, "or": true, "true": true, "false": true, "none": true,
	"nil": true, "null": true, "self": true, "this": true, "super": true,
}

func isBuiltinKeyword(name string) bool {
	return reservedKeywords[strings.ToLower(name)]
}

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

		// Use Go standard library AST parser for Go files
		if ext == ".go" {
			return g.parseGoAST(path, rel)
		}

		// Use Python native AST parser for Python files
		if ext == ".py" {
			if err := g.parsePythonAST(path, rel); err == nil {
				return nil
			}
		}

		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		lineNum := 0

		var currentSym *SymbolNode
		var currentClass string

		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			trimmed := strings.TrimSpace(line)

			// Function or class match
			var name, kind string
			if ext == ".py" {
				if m := pyClassRegex.FindStringSubmatch(trimmed); len(m) > 1 {
					name = m[1]
					kind = "class"
					currentClass = name
				} else if m := pyFuncRegex.FindStringSubmatch(trimmed); len(m) > 1 {
					if currentClass != "" && strings.HasPrefix(line, "    ") {
						name = currentClass + "." + m[1]
					} else {
						name = m[1]
						currentClass = ""
					}
					kind = "function"
				}
			} else if ext == ".ts" || ext == ".js" {
				if m := tsClassRegex.FindStringSubmatch(trimmed); len(m) > 1 {
					name = m[1]
					kind = "class"
					currentClass = name
				} else if m := tsFuncRegex.FindStringSubmatch(trimmed); len(m) > 1 {
					name = m[1]
					kind = "function"
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

			// Capture calls with keyword filtering
			if currentSym != nil {
				matches := callRegex.FindAllStringSubmatch(trimmed, -1)
				for _, match := range matches {
					if len(match) > 1 && match[1] != currentSym.Name && !isBuiltinKeyword(match[1]) {
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
	for _, callers := range g.CallGraph {
		sort.Strings(callers)
	}


	return err
}

func (g *CodeGraph) parseGoAST(path, rel string) error {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return err
	}

	goast.Inspect(node, func(n goast.Node) bool {
		funcDecl, ok := n.(*goast.FuncDecl)
		if !ok {
			return true
		}

		symName := funcDecl.Name.Name
		if funcDecl.Recv != nil && len(funcDecl.Recv.List) > 0 {
			recvType := ""
			switch t := funcDecl.Recv.List[0].Type.(type) {
			case *goast.StarExpr:
				if ident, ok := t.X.(*goast.Ident); ok {
					recvType = ident.Name
				}
			case *goast.Ident:
				recvType = t.Name
			}
			if recvType != "" {
				symName = recvType + "." + symName
			}
		}

		startPos := fset.Position(funcDecl.Pos())
		endPos := fset.Position(funcDecl.End())

		key := rel + "::" + symName
		symNode := &SymbolNode{
			Name:      symName,
			Kind:      "function",
			FilePath:  rel,
			LineStart: startPos.Line,
			LineEnd:   endPos.Line,
			Calls:     make([]string, 0),
		}

		if funcDecl.Body != nil {
			goast.Inspect(funcDecl.Body, func(bodyNode goast.Node) bool {
				call, ok := bodyNode.(*goast.CallExpr)
				if !ok {
					return true
				}
				switch fun := call.Fun.(type) {
				case *goast.Ident:
					if !isBuiltinKeyword(fun.Name) {
						symNode.Calls = append(symNode.Calls, fun.Name)
					}
				case *goast.SelectorExpr:
					if !isBuiltinKeyword(fun.Sel.Name) {
						symNode.Calls = append(symNode.Calls, fun.Sel.Name)
					}
				}
				return true
			})
		}

		g.Symbols[key] = symNode
		g.FileSymbols[rel] = append(g.FileSymbols[rel], key)
		return true
	})

	return nil
}

func (g *CodeGraph) parsePythonAST(path, rel string) error {
	script := `
import ast, json, sys

try:
    with open(sys.argv[1], 'r', encoding='utf-8') as f:
        tree = ast.parse(f.read(), filename=sys.argv[1])
except Exception:
    sys.exit(1)

symbols = []
class_stack = []

class Visitor(ast.NodeVisitor):
    def visit_ClassDef(self, node):
        class_stack.append(node.name)
        symbols.append({
            "name": node.name,
            "kind": "class",
            "line_start": node.lineno,
            "line_end": getattr(node, 'end_lineno', node.lineno),
            "calls": []
        })
        self.generic_visit(node)
        class_stack.pop()

    def visit_FunctionDef(self, node):
        self._handle_func(node)

    def visit_AsyncFunctionDef(self, node):
        self._handle_func(node)

    def _handle_func(self, node):
        fullname = f"{class_stack[-1]}.{node.name}" if class_stack else node.name
        calls = []
        for child in ast.walk(node):
            if isinstance(child, ast.Call):
                if isinstance(child.func, ast.Name):
                    calls.append(child.func.id)
                elif isinstance(child.func, ast.Attribute):
                    calls.append(child.func.attr)
        symbols.append({
            "name": fullname,
            "kind": "function",
            "line_start": node.lineno,
            "line_end": getattr(node, 'end_lineno', node.lineno),
            "calls": list(set(calls))
        })
        self.generic_visit(node)

Visitor().visit(tree)
print(json.dumps(symbols))
`
	cmd := exec.Command("python3", "-c", script, path)
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	if err := cmd.Run(); err != nil {
		return err
	}

	type pySymbol struct {
		Name      string   `json:"name"`
		Kind      string   `json:"kind"`
		LineStart int      `json:"line_start"`
		LineEnd   int      `json:"line_end"`
		Calls     []string `json:"calls"`
	}

	var pySymbols []pySymbol
	if err := json.Unmarshal(outBuf.Bytes(), &pySymbols); err != nil {
		return err
	}

	for _, ps := range pySymbols {
		key := rel + "::" + ps.Name
		symNode := &SymbolNode{
			Name:      ps.Name,
			Kind:      ps.Kind,
			FilePath:  rel,
			LineStart: ps.LineStart,
			LineEnd:   ps.LineEnd,
			Calls:     ps.Calls,
		}
		g.Symbols[key] = symNode
		g.FileSymbols[rel] = append(g.FileSymbols[rel], key)
	}

	return nil
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
	sort.Strings(direct)
	sort.Strings(transitiveList)
	sort.Strings(affectedFiles)


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
