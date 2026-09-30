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
