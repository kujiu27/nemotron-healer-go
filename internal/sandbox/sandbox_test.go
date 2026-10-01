package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunnerExecution(t *testing.T) {
	r := NewRunner(".", 5*time.Second)
	res, err := r.Run("echo 'hello from sandbox'")
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsSuccess {
		t.Fatalf("expected success, got exit code %d", res.ExitCode)
	}
	if res.Stdout != "hello from sandbox\n" {
		t.Fatalf("unexpected stdout: %q", res.Stdout)
	}
}

func TestPatcherFuzzyHunk(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "patch_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	targetFile := filepath.Join(tmpDir, "calculator.py")
	originalCode := "def add(a, b):\n    return a - b\n"
	if err := os.WriteFile(targetFile, []byte(originalCode), 0644); err != nil {
		t.Fatal(err)
	}

	diff := `--- a/calculator.py
+++ b/calculator.py
@@ -1,2 +1,2 @@
 def add(a, b):
-    return a - b
+    return a + b
`
	patcher := NewPatcher(tmpDir)
	applied, msg := patcher.ApplyPatch(diff, "calculator.py")
	if !applied {
		t.Fatalf("expected patch to apply, failed with msg: %s", msg)
	}

	newBytes, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(newBytes) != "def add(a, b):\n    return a + b\n" {
		t.Fatalf("patch content mismatch: %s", string(newBytes))
	}
}

func TestEnsureGitContextBareDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "bare_dir_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create an arbitrary source file in a bare directory
	testFile := filepath.Join(tmpDir, "sample.py")
	if err := os.WriteFile(testFile, []byte("x = 1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cm := NewCheckpointManager(tmpDir)
	if err := cm.EnsureGitContext(); err != nil {
		t.Fatalf("EnsureGitContext failed: %v", err)
	}

	// Verify .git now exists
	if _, err := os.Stat(filepath.Join(tmpDir, ".git")); os.IsNotExist(err) {
		t.Fatalf(".git was not created by EnsureGitContext")
	}

	// Verify PR branch creation works in this auto-scaffolded repository
	if err := cm.CreateGitPRBranch("fix/test-branch", "test commit"); err != nil {
		t.Fatalf("CreateGitPRBranch failed in auto-scaffolded repo: %v", err)
	}
}

func TestPatcherSecurityPathTraversalAndSecrets(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sec_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	patcher := NewPatcher(tmpDir)

	// 1. Path traversal attack attempt
	applied, msg := patcher.ApplyPatch("diff content", "../../.ssh/authorized_keys")
	if applied {
		t.Fatalf("expected path traversal patch to be BLOCKED, but it was applied!")
	}
	if !strings.Contains(msg, "Security Sandbox Blocked") && !strings.Contains(msg, "security violation") {
		t.Fatalf("expected security error message, got: %s", msg)
	}

	// 2. Secret file tampering attempt (.env)
	appliedEnv, msgEnv := patcher.ApplyPatch("diff content", ".env")
	if appliedEnv {
		t.Fatalf("expected .env modification to be BLOCKED, but it was applied!")
	}
	if !strings.Contains(msgEnv, "Security Sandbox Blocked") {
		t.Fatalf("expected security blocked message for .env, got: %s", msgEnv)
	}
}

func TestWorktreeSandbox(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "wt_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cm := NewCheckpointManager(tmpDir)
	if err := cm.EnsureGitContext(); err != nil {
		t.Fatalf("failed to init git: %v", err)
	}

	wt, err := NewWorktreeSandbox(tmpDir, "test_mcts")
	if err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}
	defer wt.Cleanup()

	if _, err := os.Stat(wt.WorkDir); os.IsNotExist(err) {
		t.Fatalf("worktree directory was not created: %s", wt.WorkDir)
	}

	if err := wt.Cleanup(); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	if _, err := os.Stat(wt.WorkDir); !os.IsNotExist(err) {
		t.Fatalf("worktree directory still exists after cleanup: %s", wt.WorkDir)
	}
}

func TestRollbackRemovesFilesCreatedAfterCheckpoint(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "rollback_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	orig := filepath.Join(tmpDir, "engine.py")
	notes := filepath.Join(tmpDir, "user_notes.txt") // pre-existing untracked file
	if err := os.WriteFile(orig, []byte("def run():\n    return 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notes, []byte("user data"), 0644); err != nil {
		t.Fatal(err)
	}

	cm := NewCheckpointManager(tmpDir)
	cpID, err := cm.CreateCheckpoint()
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a patch: modify tracked-ish file and create a new file
	if err := os.WriteFile(orig, []byte("PATCHED"), 0644); err != nil {
		t.Fatal(err)
	}
	created := filepath.Join(tmpDir, "generated_helper.py")
	if err := os.WriteFile(created, []byte("x = 1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := cm.Rollback(cpID); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(orig)
	if err != nil || string(data) != "def run():\n    return 1\n" {
		t.Fatalf("modified file not restored: %q, err=%v", string(data), err)
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Fatal("file created after checkpoint survived rollback")
	}
	if _, err := os.Stat(notes); err != nil {
		t.Fatal("pre-existing untracked file deleted by rollback")
	}
}

func TestExtractModifiedFiles(t *testing.T) {
	diff := `--- a/pkg/client.go	2026-10-01 10:00:00
+++ b/pkg/client.go	2026-10-01 10:05:00
@@ -10,2 +10,2 @@
--- a/internal/engine/agent.go
+++ b/internal/engine/agent.go
@@ -50,3 +50,3 @@
--- /dev/null
+++ b/pkg/newfile.go
`
	files := ExtractModifiedFiles(diff)
	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d: %v", len(files), files)
	}
	expected := map[string]bool{"pkg/client.go": true, "internal/engine/agent.go": true, "pkg/newfile.go": true}
	for _, f := range files {
		if !expected[f] {
			t.Errorf("unexpected file in diff: %s", f)
		}
	}
}

func TestMultiFileAtomicPatching(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "multifile_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "file2.txt")
	if err := os.WriteFile(file1, []byte("Hello Alice\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file2, []byte("Hello Bob\n"), 0644); err != nil {
		t.Fatal(err)
	}

	diff := `--- a/file1.txt
+++ b/file1.txt
@@ -1 +1 @@
-Hello Alice
+Hello Carol
--- a/file2.txt
+++ b/file2.txt
@@ -1 +1 @@
-Hello Bob
+Hello Dave
`

	patcher := NewPatcher(tmpDir)
	applied, msg := patcher.ApplyPatch(diff)
	if !applied {
		t.Fatalf("multi-file patch application failed: %s", msg)
	}

	b1, _ := os.ReadFile(file1)
	b2, _ := os.ReadFile(file2)
	if string(b1) != "Hello Carol\n" {
		t.Fatalf("file1 not updated correctly: %q", string(b1))
	}
	if string(b2) != "Hello Dave\n" {
		t.Fatalf("file2 not updated correctly: %q", string(b2))
	}
}

func TestMultiFileSecurityViolationBlocksAll(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sec_multi_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	validFile := filepath.Join(tmpDir, "valid.txt")
	if err := os.WriteFile(validFile, []byte("original\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Diff attempts to patch a valid file AND tamper with .env or escape workspace
	diff := `--- a/valid.txt
+++ b/valid.txt
@@ -1 +1 @@
-original
+tampered
--- a/.env
+++ b/.env
@@ -1 +1 @@
-KEY=old
+KEY=leaked
`

	patcher := NewPatcher(tmpDir)
	applied, msg := patcher.ApplyPatch(diff)
	if applied {
		t.Fatalf("expected multi-file security violation to be blocked, but patch was applied!")
	}
	if !strings.Contains(msg, "Security Sandbox Blocked") {
		t.Fatalf("expected security sandbox blocked message, got: %s", msg)
	}

	// Ensure the valid file was NOT modified (strict transaction atomicity)
	b1, _ := os.ReadFile(validFile)
	if string(b1) != "original\n" {
		t.Fatalf("valid file was partially modified despite security block: %q", string(b1))
	}
}
