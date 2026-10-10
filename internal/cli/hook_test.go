package cli

import (
	"os"
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
