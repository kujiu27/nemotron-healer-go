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

// CreateCheckpoint creates a lightweight snapshot of trackable source files in the workspace.
// A sidecar manifest (backupDir + ".manifest") records the exact file set so Rollback can
// delete files created after the checkpoint without touching pre-existing untracked files.
func (c *CheckpointManager) CreateCheckpoint() (string, error) {
	checkpointID := fmt.Sprintf("cp_%d", time.Now().UnixNano())
	backupDir := filepath.Join(os.TempDir(), "nemotron_checkpoints", checkpointID)
	// 0700: snapshots of the workspace must not be world-readable in /tmp.
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return "", err
	}

	var manifest []string
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
			manifest = append(manifest, rel)
		}
		return nil
	})
	if err == nil {
		err = os.WriteFile(backupDir+".manifest", []byte(strings.Join(manifest, "\n")), 0644)
	}

	return checkpointID, err
}

// Rollback restores files from the checkpoint.
func (c *CheckpointManager) Rollback(checkpointID string) error {
	backupDir := filepath.Join(os.TempDir(), "nemotron_checkpoints", checkpointID)
	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		return fmt.Errorf("checkpoint %s does not exist", checkpointID)
	}

	// First, if it's a git repo, restore tracked modifications
	cmdCheckout := exec.Command("git", "checkout", ".")
	cmdCheckout.Dir = c.WorkDir
	_ = cmdCheckout.Run()

	// Delete files created after the checkpoint (manifest diff) so patches that
	// add new files cannot leak into the next rollout. ponytail: git clean -fd
	// would also delete pre-existing untracked user files; manifest is exact.
	seen := map[string]bool{}
	if data, err := os.ReadFile(backupDir + ".manifest"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if line != "" {
				seen[line] = true
			}
		}
	}
	_ = filepath.Walk(c.WorkDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(c.WorkDir, path)
		if err != nil {
			return nil
		}
		parts := strings.Split(rel, string(os.PathSeparator))
		for _, p := range parts {
			if strings.HasPrefix(p, ".") || p == "venv" || p == "node_modules" || p == "__pycache__" || p == "bin" {
				return nil
			}
		}
		if !seen[rel] {
			_ = os.Remove(path)
		}
		return nil
	})

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

// CreateGitPRBranchWithAudit writes the Audit Card into the PR branch before committing.
func (c *CheckpointManager) CreateGitPRBranchWithAudit(branchName, commitMsg, auditReport string) error {
	_ = c.EnsureGitContext()

	// Branch creation must SUCCEED before any add/commit: on collision the
	// old code silently committed the LLM patch onto the user's current branch.
	cmd1 := exec.Command("git", "checkout", "-b", branchName)
	cmd1.Dir = c.WorkDir
	if err := cmd1.Run(); err != nil {
		return fmt.Errorf("refusing to commit: branch creation failed for %s: %w", branchName, err)
	}

	if auditReport != "" {
		auditPath := filepath.Join(c.WorkDir, "HEAL_AUDIT_REPORT.md")
		if err := os.WriteFile(auditPath, []byte(auditReport), 0644); err != nil {
			return err
		}
	}

	cmd2 := exec.Command("git", "add", "-A")
	cmd2.Dir = c.WorkDir
	if err := cmd2.Run(); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}

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
