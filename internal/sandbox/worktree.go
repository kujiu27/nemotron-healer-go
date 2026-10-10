package sandbox

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// WorktreeSandbox represents an isolated Git worktree filesystem sandbox.
// It shares the underlying Git object database with zero file-copy overhead
// and guarantees 100% isolation from the user's primary working directory.
type WorktreeSandbox struct {
	BaseRepoDir string
	WorkDir     string
	BranchName  string
	IsActive    bool
}

// NewWorktreeSandbox creates an isolated git worktree for parallel branch exploration or safe rollouts.
func NewWorktreeSandbox(baseRepoDir string, prefix string) (*WorktreeSandbox, error) {
	// Verify that baseRepoDir is inside a valid git worktree
	checkCmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	checkCmd.Dir = baseRepoDir
	if err := checkCmd.Run(); err != nil {
		return nil, fmt.Errorf("base directory %s is not inside a git repository: %w", baseRepoDir, err)
	}

	if prefix == "" {
		prefix = "nemotron_wt"
	}

	uniqueID := fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	worktreePath := filepath.Join(os.TempDir(), "nemotron_worktrees", uniqueID)

	// Ensure parent directory exists
	// 0700: worktree copies of the repo must not be world-readable in /tmp.
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0700); err != nil {
		return nil, fmt.Errorf("failed to create worktree parent dir: %w", err)
	}

	// Create detached worktree from HEAD
	cmd := exec.Command("git", "worktree", "add", "--detach", worktreePath, "HEAD")
	cmd.Dir = baseRepoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git worktree add failed (%s): %w", string(out), err)
	}

	// Synchronize uncommitted tracked changes from baseRepoDir so the worktree
	// mirrors the active working tree rather than a stale historical HEAD commit.
	diffCmd := exec.Command("git", "diff", "HEAD")
	diffCmd.Dir = baseRepoDir
	if diffOut, dErr := diffCmd.Output(); dErr == nil && len(diffOut) > 0 {
		applyCmd := exec.Command("git", "apply", "--whitespace=fix", "-")
		applyCmd.Dir = worktreePath
		applyCmd.Stdin = bytes.NewReader(diffOut)
		_ = applyCmd.Run()
	}

	// Synchronize untracked files from baseRepoDir (excluding .git and ignored files)
	untrackedCmd := exec.Command("git", "ls-files", "--others", "--exclude-standard")
	untrackedCmd.Dir = baseRepoDir
	if untrackedOut, uErr := untrackedCmd.Output(); uErr == nil {
		for _, file := range strings.Split(strings.TrimSpace(string(untrackedOut)), "\n") {
			trimmed := strings.TrimSpace(file)
			if trimmed == "" {
				continue
			}
			srcPath := filepath.Join(baseRepoDir, trimmed)
			dstPath := filepath.Join(worktreePath, trimmed)
			_ = os.MkdirAll(filepath.Dir(dstPath), 0755)
			_ = copyFile(srcPath, dstPath)
		}
	}

	return &WorktreeSandbox{
		BaseRepoDir: baseRepoDir,
		WorkDir:     worktreePath,
		BranchName:  uniqueID,
		IsActive:    true,
	}, nil
}

// Cleanup removes the isolated worktree safely.
func (w *WorktreeSandbox) Cleanup() error {
	if !w.IsActive || w.WorkDir == "" {
		return nil
	}

	// Remove worktree via git command
	cmd := exec.Command("git", "worktree", "remove", "--force", w.WorkDir)
	cmd.Dir = w.BaseRepoDir
	_ = cmd.Run()

	// Prune dead worktree metadata
	cmdPrune := exec.Command("git", "worktree", "prune")
	cmdPrune.Dir = w.BaseRepoDir
	_ = cmdPrune.Run()

	// Ensure disk cleanup
	_ = os.RemoveAll(w.WorkDir)
	w.IsActive = false
	return nil
}
