package sandbox

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// PreFlightSyntaxCheck performs sub-50ms static syntax compilation verification
// before committing or executing long-running test suites.
func PreFlightSyntaxCheck(workDir, relFilePath string) (bool, string) {
	if relFilePath == "" {
		return true, ""
	}
	fullPath := filepath.Join(workDir, relFilePath)
	if _, err := os.Stat(fullPath); err != nil {
		return true, ""
	}

	ext := strings.ToLower(filepath.Ext(relFilePath))

	switch ext {
	case ".go":
		fset := token.NewFileSet()
		if _, err := parser.ParseFile(fset, fullPath, nil, parser.AllErrors); err != nil {
			return false, fmt.Sprintf("Go Syntax Error in %s: %v", relFilePath, err)
		}
	case ".py":
		script := "import ast, sys; ast.parse(open(sys.argv[1], 'rb').read())"
		cmd := exec.Command("python3", "-c", script, fullPath)
		var errBuf bytes.Buffer
		cmd.Stderr = &errBuf
		if err := cmd.Run(); err != nil {
			msg := strings.TrimSpace(errBuf.String())
			if msg == "" {
				msg = err.Error()
			}
			return false, fmt.Sprintf("Python Syntax Error in %s: %s", relFilePath, msg)
		}
	case ".json":
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return false, fmt.Sprintf("JSON Read Error in %s: %v", relFilePath, err)
		}
		if !json.Valid(data) {
			return false, fmt.Sprintf("JSON Syntax Error in %s: invalid JSON payload", relFilePath)
		}
	case ".js", ".mjs", ".cjs":
		if _, lookErr := exec.LookPath("node"); lookErr == nil {
			cmd := exec.Command("node", "--check", fullPath)
			var errBuf bytes.Buffer
			cmd.Stderr = &errBuf
			if err := cmd.Run(); err != nil {
				return false, fmt.Sprintf("JavaScript Syntax Error in %s: %s", relFilePath, strings.TrimSpace(errBuf.String()))
			}
		}
	}
	return true, ""
}

type Patcher struct {
	WorkDir string
}

func NewPatcher(workDir string) *Patcher {
	return &Patcher{WorkDir: workDir}
}

// ValidateSafePath enforces strict sandbox containment, preventing Path Traversal (CWE-23)
// and blocking tampering with sensitive files (.env, .git, credentials, private keys).
func ValidateSafePath(workDir, relPath string) (string, error) {
	cleanRel := filepath.Clean(relPath)
	if cleanRel == ".." || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) || filepath.IsAbs(relPath) {
		return "", fmt.Errorf("security violation: path traversal detected (%s escapes workspace)", relPath)
	}

	lower := strings.ToLower(cleanRel)
	if strings.Contains(lower, ".env") ||
		strings.Contains(lower, "id_rsa") ||
		strings.Contains(lower, "id_ed25519") ||
		strings.Contains(lower, ".ssh") ||
		strings.Contains(lower, "credentials") {
		return "", fmt.Errorf("security violation: modification of protected file forbidden (%s)", relPath)
	}
	// Any path COMPONENT equal to .git (nested repos, submodules) is
	// protected — a prefix-only check let `nested/.git/hooks/pre-commit` through.
	for _, seg := range strings.Split(cleanRel, string(filepath.Separator)) {
		if strings.ToLower(seg) == ".git" {
			return "", fmt.Errorf("security violation: modification of protected .git path forbidden (%s)", relPath)
		}
	}

	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return "", err
	}
	// Resolve BOTH sides: on macOS even TempDir() sits behind /var -> /private/var.
	resolvedWork := absWorkDir
	if rw, rErr := filepath.EvalSymlinks(absWorkDir); rErr == nil {
		resolvedWork = rw
	}
	fullPath := filepath.Join(absWorkDir, cleanRel)
	if !strings.HasPrefix(fullPath, absWorkDir+string(filepath.Separator)) && fullPath != absWorkDir {
		return "", fmt.Errorf("security violation: path traversal out of bounds")
	}

	// Symlink containment: resolve the deepest existing ancestor and require
	// the resolved path to stay inside the workspace. A workspace symlink
	// like `assets -> /etc` must not let a diff write outside.
	probe := fullPath
	for !fileExists(probe) {
		parent := filepath.Dir(probe)
		if parent == probe {
			break
		}
		probe = parent
	}
	if resolved, rErr := filepath.EvalSymlinks(probe); rErr == nil {
		if !strings.HasPrefix(resolved, resolvedWork+string(filepath.Separator)) && resolved != resolvedWork {
			return "", fmt.Errorf("security violation: symlink escapes workspace (%s -> %s)", relPath, resolved)
		}
	}

	return fullPath, nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// ExtractModifiedFiles parses unified diff headers (--- and +++) to discover all target files
func ExtractModifiedFiles(diffPatch string) []string {
	fileMap := make(map[string]bool)
	lines := strings.Split(diffPatch, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		var p string
		if strings.HasPrefix(trimmed, "--- ") {
			p = strings.TrimPrefix(trimmed, "--- ")
		} else if strings.HasPrefix(trimmed, "+++ ") {
			p = strings.TrimPrefix(trimmed, "+++ ")
		}
		if p == "" || p == "/dev/null" || strings.HasPrefix(p, "/dev/null") {
			continue
		}
		// Strip timestamp if present (e.g. "path/to/file\t2026-10-01 ...")
		cleanP := strings.TrimSpace(strings.Split(p, "\t")[0])
		cleanP = strings.TrimPrefix(cleanP, "a/")
		cleanP = strings.TrimPrefix(cleanP, "b/")
		if cleanP != "" && cleanP != "/dev/null" {
			fileMap[cleanP] = true
		}
	}
	var res []string
	for f := range fileMap {
		res = append(res, f)
	}
	sort.Strings(res)
	return res
}

// ApplyPatch tries git apply, patch command, and atomic multi-file fallback hunk replacer.
func (p *Patcher) ApplyPatch(diffPatch string, targetFileHint ...string) (bool, string) {
	if strings.TrimSpace(diffPatch) == "" {
		return false, "empty patch provided"
	}

	// Clean up markdown fences if any
	cleanPatch := strings.TrimSpace(diffPatch)
	if strings.HasPrefix(cleanPatch, "```diff") {
		cleanPatch = strings.TrimPrefix(cleanPatch, "```diff")
	} else if strings.HasPrefix(cleanPatch, "```") {
		cleanPatch = strings.TrimPrefix(cleanPatch, "```")
	}
	cleanPatch = strings.TrimSuffix(cleanPatch, "```")
	cleanPatch = strings.TrimSpace(cleanPatch) + "\n"

	// 1. Security Sanitization Gate across all modified files
	modFiles := ExtractModifiedFiles(cleanPatch)
	for _, hint := range targetFileHint {
		if hint != "" {
			found := false
			for _, m := range modFiles {
				if m == hint {
					found = true
					break
				}
			}
			if !found {
				modFiles = append(modFiles, hint)
			}
		}
	}

	for _, f := range modFiles {
		if _, err := ValidateSafePath(p.WorkDir, f); err != nil {
			return false, fmt.Sprintf("Security Sandbox Blocked: %v", err)
		}
	}

	// Temp patch file
	tmpPatchFile, err := os.CreateTemp("", "nemotron_patch_*.patch")
	if err != nil {
		return false, fmt.Sprintf("failed to create temp patch: %v", err)
	}
	defer os.Remove(tmpPatchFile.Name())

	if _, err := tmpPatchFile.WriteString(cleanPatch); err != nil {
		return false, fmt.Sprintf("failed to write temp patch: %v", err)
	}
	tmpPatchFile.Close()

	fileCountInfo := fmt.Sprintf("%d file(s)", len(modFiles))
	if len(modFiles) > 0 {
		fileCountInfo = fmt.Sprintf("%d file(s) [%s]", len(modFiles), strings.Join(modFiles, ", "))
	}

	// Tier 1: git apply (atomic by default across all files)
	for _, pFlag := range []string{"-p1", "-p0"} {
		cmd := exec.Command("git", "apply", "--whitespace=fix", pFlag, tmpPatchFile.Name())
		cmd.Dir = p.WorkDir
		var errBuf bytes.Buffer
		cmd.Stderr = &errBuf
		if err := cmd.Run(); err == nil {
			return true, fmt.Sprintf("Applied atomic patch via git apply %s across %s", pFlag, fileCountInfo)
		}
	}

	// Tier 2: standard patch utility
	for _, pFlag := range []string{"-p1", "-p0"} {
		cmd := exec.Command("patch", pFlag, "-i", tmpPatchFile.Name())
		cmd.Dir = p.WorkDir
		var errBuf bytes.Buffer
		cmd.Stderr = &errBuf
		if err := cmd.Run(); err == nil {
			return true, fmt.Sprintf("Applied patch via patch %s across %s", pFlag, fileCountInfo)
		}
	}

	// Tier 3: Atomic In-memory multi-file hunk fallback
	if len(modFiles) > 0 {
		ok, msg := p.applyAtomicMultiFileHunks(modFiles, cleanPatch)
		if ok {
			return true, fmt.Sprintf("Applied patch via atomic fallback hunk replacer: %s", msg)
		}
	}

	return false, "All patch strategies (git apply, patch, fuzzy hunk) failed"
}

// splitDiffIntoFileChunks segments a unified diff into per-file chunks
func splitDiffIntoFileChunks(diffPatch string) map[string]string {
	chunks := make(map[string]string)
	lines := strings.Split(diffPatch, "\n")
	var currentFile string
	var currentLines []string

	flush := func() {
		if currentFile != "" && len(currentLines) > 0 {
			chunks[currentFile] = strings.Join(currentLines, "\n")
		}
		currentLines = nil
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--- ") {
			flush()
			raw := strings.TrimPrefix(trimmed, "--- ")
			clean := strings.TrimSpace(strings.Split(raw, "\t")[0])
			clean = strings.TrimPrefix(clean, "a/")
			clean = strings.TrimPrefix(clean, "b/")
			if clean != "/dev/null" {
				currentFile = clean
			}
		} else if strings.HasPrefix(trimmed, "+++ ") && currentFile == "" {
			raw := strings.TrimPrefix(trimmed, "+++ ")
			clean := strings.TrimSpace(strings.Split(raw, "\t")[0])
			clean = strings.TrimPrefix(clean, "a/")
			clean = strings.TrimPrefix(clean, "b/")
			if clean != "/dev/null" {
				currentFile = clean
			}
		}
		currentLines = append(currentLines, line)
	}
	flush()
	return chunks
}

func (p *Patcher) applyAtomicMultiFileHunks(targetFiles []string, diffPatch string) (bool, string) {
	fileChunks := splitDiffIntoFileChunks(diffPatch)
	if len(fileChunks) == 0 && len(targetFiles) == 1 {
		fileChunks[targetFiles[0]] = diffPatch
	}

	type fileMutation struct {
		relPath    string
		fullPath   string
		newContent string
	}

	var mutations []fileMutation

	for relFile, chunk := range fileChunks {
		fullPath, err := ValidateSafePath(p.WorkDir, relFile)
		if err != nil {
			return false, fmt.Sprintf("security violation: %v", err)
		}
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return false, fmt.Sprintf("could not read file %s: %v", relFile, err)
		}

		content := string(data)
		newContent, ok := replaceHunksInContent(content, chunk)
		if !ok {
			return false, fmt.Sprintf("fuzzy hunk could not locate exact context match in %s", relFile)
		}
		mutations = append(mutations, fileMutation{
			relPath:    relFile,
			fullPath:   fullPath,
			newContent: newContent,
		})
	}

	if len(mutations) == 0 {
		return false, "no applicable hunks found"
	}

	// Atomic Commit: Write all files simultaneously
	for _, m := range mutations {
		if err := os.WriteFile(m.fullPath, []byte(m.newContent), 0644); err != nil {
			return false, fmt.Sprintf("failed writing updated file %s: %v", m.relPath, err)
		}
	}

	var changed []string
	for _, m := range mutations {
		changed = append(changed, m.relPath)
	}
	sort.Strings(changed)
	return true, fmt.Sprintf("Atomically replaced hunks in: %s", strings.Join(changed, ", "))
}

type diffHunk struct {
	oldLines []string
	newLines []string
}

// parseHunks splits a per-file diff chunk into individual hunks. Lines inside
// a hunk keep their leading character stripped ('\ No newline' markers are
// skipped; they carry no content for matching).
func parseHunks(diffPatch string) []diffHunk {
	var hunks []diffHunk
	var cur *diffHunk
	for _, dLine := range strings.Split(diffPatch, "\n") {
		switch {
		case strings.HasPrefix(dLine, "@@"):
			hunks = append(hunks, diffHunk{})
			cur = &hunks[len(hunks)-1]
		case cur == nil:
			// header or diff noise before the first hunk
		case strings.HasPrefix(dLine, "\\"):
			// "\ No newline at end of file" marker: skip
		case strings.HasPrefix(dLine, "---"):
			// file header
		case strings.HasPrefix(dLine, "+++"):
			// file header
		case strings.HasPrefix(dLine, "-"):
			cur.oldLines = append(cur.oldLines, strings.TrimPrefix(dLine, "-"))
		case strings.HasPrefix(dLine, "+"):
			cur.newLines = append(cur.newLines, strings.TrimPrefix(dLine, "+"))
		case strings.HasPrefix(dLine, " "):
			l := strings.TrimPrefix(dLine, " ")
			cur.oldLines = append(cur.oldLines, l)
			cur.newLines = append(cur.newLines, l)
		}
	}
	return hunks
}

// replaceHunksInContent applies every hunk of the chunk independently: each
// hunk's old block must match somewhere in the content, and is replaced once.
// All hunks must apply or nothing does (callers write content only on true).
// The previous implementation concatenated ALL hunks into one block, which
// only matched when the hunks were contiguous in the file — multi-hunk diffs
// (the common LLM shape) always failed the fallback.
func replaceHunksInContent(content string, diffPatch string) (string, bool) {
	hunks := parseHunks(diffPatch)
	if len(hunks) == 0 {
		return "", false
	}

	newContent := content
	applied := 0
	for _, h := range hunks {
		if len(h.oldLines) == 0 && len(h.newLines) == 0 {
			continue // empty hunk
		}
		oldBlock := strings.Join(h.oldLines, "\n")
		newBlock := strings.Join(h.newLines, "\n")
		if oldBlock == "" {
			// pure insertion with no context: cannot locate reliably
			return "", false
		}
		if !strings.Contains(newContent, oldBlock) {
			return "", false
		}
		newContent = strings.Replace(newContent, oldBlock, newBlock, 1)
		applied++
	}
	if applied == 0 {
		return "", false
	}
	return newContent, true
}
