package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitHookInstallAndUninstall(t *testing.T) {
	tmpDir := t.TempDir()

	// Initial check: not a git repo should fail
	_, err := InstallGitHook(tmpDir)
	if err == nil {
		t.Fatal("expected error installing hook in non-git directory, got nil")
	}

	// Init fake .git directory
	gitDir := filepath.Join(tmpDir, ".git")
	_ = os.MkdirAll(gitDir, 0755)

	hookPath, err := InstallGitHook(tmpDir)
	if err != nil {
		t.Fatalf("InstallGitHook failed: %v", err)
	}

	info, err := os.Stat(hookPath)
	if err != nil {
		t.Fatalf("hook file was not created: %v", err)
	}
	if info.Mode()&0111 == 0 {
		t.Fatalf("hook file is not executable: %o", info.Mode())
	}

	// Test uninstall
	if err := UninstallGitHook(tmpDir); err != nil {
		t.Fatalf("UninstallGitHook failed: %v", err)
	}

	if _, err := os.Stat(hookPath); !os.IsNotExist(err) {
		t.Fatal("hook file still exists after uninstall")
	}
}

func TestGitHookAutoHealFlag(t *testing.T) {
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	_ = os.MkdirAll(gitDir, 0755)

	hookPath, err := InstallGitHook(tmpDir, true)
	if err != nil {
		t.Fatalf("InstallGitHook with autoHeal failed: %v", err)
	}

	content, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("failed reading hook file: %v", err)
	}
	if !strings.Contains(string(content), "nemotron-healer hook run . --auto-heal") {
		t.Errorf("expected --auto-heal in hook script, got:\n%s", string(content))
	}
}

func TestGitHookInWorktree(t *testing.T) {
	tmpDir := t.TempDir()

	// Init real git repo
	cInit := exec.Command("git", "init")
	cInit.Dir = tmpDir
	if err := cInit.Run(); err != nil {
		t.Skip("git init unavailable")
	}

	// Configure user to permit initial commit
	_ = exec.Command("git", "-C", tmpDir, "config", "user.name", "Test").Run()
	_ = exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com").Run()
	_ = os.WriteFile(filepath.Join(tmpDir, "README"), []byte("hello"), 0644)
	_ = exec.Command("git", "-C", tmpDir, "add", "README").Run()
	_ = exec.Command("git", "-C", tmpDir, "commit", "-m", "init").Run()

	// Create worktree
	wtDir := filepath.Join(t.TempDir(), "wt")
	cWt := exec.Command("git", "-C", tmpDir, "worktree", "add", "--detach", wtDir, "HEAD")
	if err := cWt.Run(); err != nil {
		t.Skipf("git worktree add failed: %v", err)
	}
	defer func() {
		_ = exec.Command("git", "-C", tmpDir, "worktree", "remove", "--force", wtDir).Run()
	}()

	// Install hook from inside worktree
	hookPath, err := InstallGitHook(wtDir)
	if err != nil {
		t.Fatalf("InstallGitHook inside worktree failed: %v", err)
	}
	if _, err := os.Stat(hookPath); err != nil {
		t.Fatalf("hook not created at %s: %v", hookPath, err)
	}

	// Uninstall hook from inside worktree
	if err := UninstallGitHook(wtDir); err != nil {
		t.Fatalf("UninstallGitHook inside worktree failed: %v", err)
	}
	if _, err := os.Stat(hookPath); !os.IsNotExist(err) {
		t.Fatalf("hook still exists after uninstall in worktree")
	}
}
