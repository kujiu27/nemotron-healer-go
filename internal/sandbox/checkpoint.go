package sandbox

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type CheckpointManager struct {
	WorkDir string
}

func NewCheckpointManager(workDir string) *CheckpointManager {
	return &CheckpointManager{WorkDir: workDir}
}

// CreateCheckpoint creates a lightweight snapshot of modified files in the workspace.
func (c *CheckpointManager) CreateCheckpoint() (string, error) {
	checkpointID := fmt.Sprintf("cp_%d", time.Now().UnixNano())
	backupDir := filepath.Join(os.TempDir(), "nemotron_checkpoints", checkpointID)
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", err
	}

	// Copy all trackable source files to backupDir
	err := filepath.Walk(c.WorkDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(c.WorkDir, path)
		if err != nil || rel == "." {
			return nil
		}
		// Skip hidden, git, cache
		parts := strings.Split(rel, string(os.PathSeparator))
		for _, p := range parts {
			if strings.HasPrefix(p, ".") || p == "venv" || p == "node_modules" || p == "__pycache__" || p == "bin" {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		if !info.IsDir() {
			destPath := filepath.Join(backupDir, rel)
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return err
			}
			copyFile(path, destPath)
		}
		return nil
	})

	return checkpointID, err
}

// Rollback restores files from the checkpoint.
func (c *CheckpointManager) Rollback(checkpointID string) error {
	backupDir := filepath.Join(os.TempDir(), "nemotron_checkpoints", checkpointID)
	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		return fmt.Errorf("checkpoint %s does not exist", checkpointID)
	}

	// First, if it's a git repo, attempt git checkout .
	cmd := exec.Command("git", "checkout", ".")
	cmd.Dir = c.WorkDir
	_ = cmd.Run()

	// Restore files from backupDir
	err := filepath.Walk(backupDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(backupDir, path)
		destPath := filepath.Join(c.WorkDir, rel)
		_ = os.MkdirAll(filepath.Dir(destPath), 0755)
		copyFile(path, destPath)
		return nil
	})

	// Clean up checkpoint
	_ = os.RemoveAll(backupDir)
	return err
}

// CreateGitPRBranch commits the verified fix and creates a dedicated PR branch.
func (c *CheckpointManager) CreateGitPRBranch(branchName, commitMsg string) error {
	return c.CreateGitPRBranchWithAudit(branchName, commitMsg, "")
}

// CreateGitPRBranchWithAudit writes an Alibaba OCR-style Audit Card into the PR branch before committing.
func (c *CheckpointManager) CreateGitPRBranchWithAudit(branchName, commitMsg, auditReport string) error {
	cmd1 := exec.Command("git", "checkout", "-b", branchName)
	cmd1.Dir = c.WorkDir
	_ = cmd1.Run()

	if auditReport != "" {
		auditPath := filepath.Join(c.WorkDir, "HEAL_AUDIT_REPORT.md")
		_ = os.WriteFile(auditPath, []byte(auditReport), 0644)
	}

	cmd2 := exec.Command("git", "add", "-A")
	cmd2.Dir = c.WorkDir
	_ = cmd2.Run()

	cmd3 := exec.Command("git", "commit", "-m", commitMsg)
	cmd3.Dir = c.WorkDir
	return cmd3.Run()
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
