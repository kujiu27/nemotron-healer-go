package sandbox

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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
	if strings.HasPrefix(cleanRel, "..") || filepath.IsAbs(relPath) {
		return "", fmt.Errorf("security violation: path traversal detected (%s escapes workspace)", relPath)
	}

	lower := strings.ToLower(cleanRel)
	if strings.Contains(lower, ".env") ||
		strings.HasPrefix(lower, ".git") ||
		strings.Contains(lower, "id_rsa") ||
		strings.Contains(lower, "id_ed25519") ||
		strings.Contains(lower, ".ssh") ||
		strings.Contains(lower, "credentials") {
		return "", fmt.Errorf("security violation: modification of protected file forbidden (%s)", relPath)
	}

	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return "", err
	}
	fullPath := filepath.Join(absWorkDir, cleanRel)
	if !strings.HasPrefix(fullPath, absWorkDir) {
		return "", fmt.Errorf("security violation: path traversal out of bounds")
	}

	return fullPath, nil
}

// ApplyPatch tries git apply, patch command, and fallback hunk replacer.
func (p *Patcher) ApplyPatch(diffPatch string, targetFileHint string) (bool, string) {
	if strings.TrimSpace(diffPatch) == "" {
		return false, "empty patch provided"
	}

	// Security Sanitization Gate
	if targetFileHint != "" {
		if _, err := ValidateSafePath(p.WorkDir, targetFileHint); err != nil {
			return false, fmt.Sprintf("Security Sandbox Blocked: %v", err)
		}
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

	// Tier 1: git apply
	for _, pFlag := range []string{"-p1", "-p0"} {
		cmd := exec.Command("git", "apply", "--whitespace=fix", pFlag, tmpPatchFile.Name())
		cmd.Dir = p.WorkDir
		var errBuf bytes.Buffer
		cmd.Stderr = &errBuf
		if err := cmd.Run(); err == nil {
			return true, fmt.Sprintf("Applied patch via git apply %s", pFlag)
		}
	}

	// Tier 2: standard patch utility
	for _, pFlag := range []string{"-p1", "-p0"} {
		cmd := exec.Command("patch", pFlag, "-i", tmpPatchFile.Name())
		cmd.Dir = p.WorkDir
		var errBuf bytes.Buffer
		cmd.Stderr = &errBuf
		if err := cmd.Run(); err == nil {
			return true, fmt.Sprintf("Applied patch via patch %s", pFlag)
		}
	}

	// Tier 3: In-memory hunk fallback if targetFileHint provided
	if targetFileHint != "" {
		ok, msg := p.applyFuzzyHunk(targetFileHint, cleanPatch)
		if ok {
			return true, fmt.Sprintf("Applied patch via fallback hunk replacer: %s", msg)
		}
	}

	return false, "All patch strategies (git apply, patch, fuzzy hunk) failed"
}

func (p *Patcher) applyFuzzyHunk(targetRelFile string, diffPatch string) (bool, string) {
	fullPath, err := ValidateSafePath(p.WorkDir, targetRelFile)
	if err != nil {
		return false, fmt.Sprintf("security violation: %v", err)
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return false, fmt.Sprintf("could not read file %s: %v", targetRelFile, err)
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	var oldLines []string
	var newLines []string
	inHunk := false

	for _, dLine := range strings.Split(diffPatch, "\n") {
		if strings.HasPrefix(dLine, "@@") {
			inHunk = true
			continue
		}
		if inHunk {
			if strings.HasPrefix(dLine, "-") && !strings.HasPrefix(dLine, "---") {
				oldLines = append(oldLines, strings.TrimPrefix(dLine, "-"))
			} else if strings.HasPrefix(dLine, "+") && !strings.HasPrefix(dLine, "+++") {
				newLines = append(newLines, strings.TrimPrefix(dLine, "+"))
			} else if strings.HasPrefix(dLine, " ") {
				oldLines = append(oldLines, strings.TrimPrefix(dLine, " "))
				newLines = append(newLines, strings.TrimPrefix(dLine, " "))
			}
		}
	}

	oldBlock := strings.Join(oldLines, "\n")
	newBlock := strings.Join(newLines, "\n")

	if oldBlock != "" && strings.Contains(content, oldBlock) {
		newContent := strings.Replace(content, oldBlock, newBlock, 1)
		if err := os.WriteFile(fullPath, []byte(newContent), 0644); err == nil {
			return true, fmt.Sprintf("Exact block replaced in %s", targetRelFile)
		}
	}

	_ = lines
	return false, "Fuzzy hunk could not locate exact context match"
}
