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

// EnsureGitContext guarantees that the working directory is inside a valid git repository.
// If the target directory is a bare folder without .git, it auto-initializes a clean local tracking baseline.
func (c *CheckpointManager) EnsureGitContext() error {
	checkCmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	checkCmd.Dir = c.WorkDir
	if err := checkCmd.Run(); err == nil {
		return nil // Already a valid git repository
	}

	// Auto-scaffold local git repository
	initCmd := exec.Command("git", "init")
	initCmd.Dir = c.WorkDir
	if err := initCmd.Run(); err != nil {
		return err
	}

	// Set local author config so commits never fail on missing user.email
	cmdName := exec.Command("git", "config", "user.name", "Nemotron-Healer Bot")
	cmdName.Dir = c.WorkDir
	_ = cmdName.Run()

	cmdEmail := exec.Command("git", "config", "user.email", "bot@nemotron-healer.ai")
	cmdEmail.Dir = c.WorkDir
	_ = cmdEmail.Run()

	// Initial baseline snapshot commit
	cmdAdd := exec.Command("git", "add", "-A")
	cmdAdd.Dir = c.WorkDir
	_ = cmdAdd.Run()

	cmdCommit := exec.Command("git", "commit", "--allow-empty", "-m", "chore(init): snapshot baseline state prior to autonomous repair")
	cmdCommit.Dir = c.WorkDir
	_ = cmdCommit.Run()

	return nil
}

// CreateGitPRBranch commits the verified fix and creates a dedicated PR branch.
func (c *CheckpointManager) CreateGitPRBranch(branchName, commitMsg string) error {
	return c.CreateGitPRBranchWithAudit(branchName, commitMsg, "")
}

// CreateGitPRBranchWithAudit writes an Alibaba OCR-style Audit Card into the PR branch before committing.
func (c *CheckpointManager) CreateGitPRBranchWithAudit(branchName, commitMsg, auditReport string) error {
	_ = c.EnsureGitContext()

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

	cmd3 := exec.Command("git", "commit", "--allow-empty", "-m", commitMsg)
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
