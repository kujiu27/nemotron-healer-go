package cli

import (
	"encoding/json"
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

func TestHookCmd_JSONPure(t *testing.T) {
	tmpDir := t.TempDir()
	cInit := exec.Command("git", "init")
	cInit.Dir = tmpDir
	if err := cInit.Run(); err != nil {
		t.Skip("git init unavailable")
	}

	origJSON := jsonFlag
	jsonFlag = true
	defer func() { jsonFlag = origJSON }()

	outInstall := captureStdout(func() {
		_ = hookInstallCmd.RunE(hookInstallCmd, []string{tmpDir})
	})

	var resInstall struct {
		Status   string `json:"status"`
		HookPath string `json:"hook_path"`
		AutoHeal bool   `json:"auto_heal"`
	}
	if err := json.Unmarshal([]byte(outInstall), &resInstall); err != nil {
		t.Fatalf("hook install --json stdout not pure JSON: %v, raw:\n%s", err, outInstall)
	}
	if resInstall.Status != "installed" || resInstall.HookPath == "" {
		t.Errorf("unexpected hook install payload: %+v", resInstall)
	}

	outUninstall := captureStdout(func() {
		_ = hookUninstallCmd.RunE(hookUninstallCmd, []string{tmpDir})
	})

	var resUninstall struct {
		Status  string `json:"status"`
		Target  string `json:"target"`
		Success bool   `json:"success"`
	}
	if err := json.Unmarshal([]byte(outUninstall), &resUninstall); err != nil {
		t.Fatalf("hook uninstall --json stdout not pure JSON: %v, raw:\n%s", err, outUninstall)
	}
	if resUninstall.Status != "uninstalled" || !resUninstall.Success {
		t.Errorf("unexpected hook uninstall payload: %+v", resUninstall)
	}

	// Test hook run --json with passing dummy test command
	dummyGo := filepath.Join(tmpDir, "dummy_test.go")
	_ = os.WriteFile(dummyGo, []byte("package dummy\n"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module dummy\n\ngo 1.22\n"), 0644)
	outRun := captureStdout(func() {
		_ = hookRunCmd.RunE(hookRunCmd, []string{tmpDir})
	})
	var resRun struct {
		Status  string `json:"status"`
		Success bool   `json:"success"`
	}
	if err := json.Unmarshal([]byte(outRun), &resRun); err != nil {
		t.Fatalf("hook run --json stdout not pure JSON: %v, raw:\n%s", err, outRun)
	}
	if resRun.Status != "passed" || !resRun.Success {
		t.Errorf("unexpected hook run payload: %+v", resRun)
	}
}
