package sandbox

import (
	"os"
	"os/exec"
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

func TestWorktreeSandbox_DirtyWorkspaceSync(t *testing.T) {
	tmpDir := t.TempDir()
	cm := NewCheckpointManager(tmpDir)
	if err := cm.EnsureGitContext(); err != nil {
		t.Fatalf("failed to init git: %v", err)
	}

	// 1. Create a committed baseline file
	baseFile := filepath.Join(tmpDir, "tracked.txt")
	_ = os.WriteFile(baseFile, []byte("v1"), 0644)
	cAdd := exec.Command("git", "add", "tracked.txt")
	cAdd.Dir = tmpDir
	_ = cAdd.Run()
	cCommit := exec.Command("git", "commit", "-m", "init tracked")
	cCommit.Dir = tmpDir
	_ = cCommit.Run()

	// 2. Introduce UNCOMMITTED modification
	_ = os.WriteFile(baseFile, []byte("v2-uncommitted"), 0644)

	// 3. Introduce UNTRACKED new file
	untrackedFile := filepath.Join(tmpDir, "new_untracked.txt")
	_ = os.WriteFile(untrackedFile, []byte("fresh-code"), 0644)

	// 4. Launch WorktreeSandbox
	wt, err := NewWorktreeSandbox(tmpDir, "test_sync")
	if err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}
	defer wt.Cleanup()

	// 5. Verify that uncommitted modification is present in the worktree
	wtTracked := filepath.Join(wt.WorkDir, "tracked.txt")
	contentTracked, err := os.ReadFile(wtTracked)
	if err != nil {
		t.Fatalf("could not read tracked file in worktree: %v", err)
	}
	if string(contentTracked) != "v2-uncommitted" {
		t.Errorf("expected worktree to reflect uncommitted change 'v2-uncommitted', got: %s", string(contentTracked))
	}

	// 6. Verify that untracked new file is present in the worktree
	wtUntracked := filepath.Join(wt.WorkDir, "new_untracked.txt")
	contentUntracked, err := os.ReadFile(wtUntracked)
	if err != nil {
		t.Fatalf("could not read untracked file in worktree: %v", err)
	}
	if string(contentUntracked) != "fresh-code" {
		t.Errorf("expected worktree to reflect untracked file 'fresh-code', got: %s", string(contentUntracked))
	}
}

func TestCopyFile_PreservesPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "script.sh")
	dst := filepath.Join(tmpDir, "script_copy.sh")

	_ = os.WriteFile(src, []byte("#!/bin/sh\necho ok\n"), 0755)
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	dstInfo, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if dstInfo.Mode().Perm() != srcInfo.Mode().Perm() {
		t.Errorf("expected permission %v, got %v", srcInfo.Mode().Perm(), dstInfo.Mode().Perm())
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
	// Deterministic sorting verification
	expectedSlice := []string{"internal/engine/agent.go", "pkg/client.go", "pkg/newfile.go"}
	for i, exp := range expectedSlice {
		if files[i] != exp {
			t.Errorf("expected files[%d] == %s, got %s (slice not sorted deterministically: %v)", i, exp, files[i], files)
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

func TestPreFlightSyntaxCheck(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Valid Go code
	validGo := filepath.Join(tmpDir, "valid.go")
	_ = os.WriteFile(validGo, []byte("package main\nfunc main() {}\n"), 0644)
	ok, msg := PreFlightSyntaxCheck(tmpDir, "valid.go")
	if !ok {
		t.Fatalf("expected valid Go code to pass syntax check, got: %s", msg)
	}

	// 2. Invalid Go code (unclosed bracket)
	badGo := filepath.Join(tmpDir, "bad.go")
	_ = os.WriteFile(badGo, []byte("package main\nfunc main() {\n"), 0644)
	badOK, badMsg := PreFlightSyntaxCheck(tmpDir, "bad.go")
	if badOK {
		t.Fatalf("expected malformed Go code to be rejected, but it passed!")
	}
	if !strings.Contains(badMsg, "Go Syntax Error") {
		t.Fatalf("expected Go Syntax Error message, got: %s", badMsg)
	}

	// 3. Valid Python code
	validPy := filepath.Join(tmpDir, "valid.py")
	_ = os.WriteFile(validPy, []byte("def add(a, b):\n    return a + b\n"), 0644)
	okPy, msgPy := PreFlightSyntaxCheck(tmpDir, "valid.py")
	if !okPy {
		t.Fatalf("expected valid Python code to pass syntax check, got: %s", msgPy)
	}

	// 4. Invalid Python code (syntax error)
	badPy := filepath.Join(tmpDir, "bad.py")
	_ = os.WriteFile(badPy, []byte("def add(a, b\n    return a +\n"), 0644)
	badPyOK, badPyMsg := PreFlightSyntaxCheck(tmpDir, "bad.py")
	if badPyOK {
		t.Fatalf("expected malformed Python code to be rejected, but it passed!")
	}
	if !strings.Contains(badPyMsg, "Python Syntax Error") {
		t.Fatalf("expected Python Syntax Error message, got: %s", badPyMsg)
	}

	// 5. Valid JSON payload
	validJSON := filepath.Join(tmpDir, "config.json")
	_ = os.WriteFile(validJSON, []byte(`{"enabled": true, "count": 42}`), 0644)
	okJSON, msgJSON := PreFlightSyntaxCheck(tmpDir, "config.json")
	if !okJSON {
		t.Fatalf("expected valid JSON to pass, got: %s", msgJSON)
	}

	// 6. Malformed JSON payload
	badJSON := filepath.Join(tmpDir, "bad.json")
	_ = os.WriteFile(badJSON, []byte(`{"enabled": true, trailing_comma: }`), 0644)
	badJSONOK, badJSONMsg := PreFlightSyntaxCheck(tmpDir, "bad.json")
	if badJSONOK {
		t.Fatalf("expected malformed JSON to be rejected")
	}
	if !strings.Contains(badJSONMsg, "JSON Syntax Error") {
		t.Fatalf("expected JSON Syntax Error message, got: %s", badJSONMsg)
	}

	// 7. Valid Rust code
	validRs := filepath.Join(tmpDir, "valid.rs")
	_ = os.WriteFile(validRs, []byte("pub fn add(a: i32, b: i32) -> i32 { a + b }\n"), 0644)
	okRs, msgRs := PreFlightSyntaxCheck(tmpDir, "valid.rs")
	if !okRs {
		t.Fatalf("expected valid Rust to pass, got: %s", msgRs)
	}

	// 8. Malformed Rust code (if rustc available)
	if _, err := exec.LookPath("rustc"); err == nil {
		badRs := filepath.Join(tmpDir, "bad.rs")
		_ = os.WriteFile(badRs, []byte("pub fn add(a: i32, b: i32 -> i32 {\n"), 0644)
		badRsOK, badRsMsg := PreFlightSyntaxCheck(tmpDir, "bad.rs")
		if badRsOK {
			t.Fatalf("expected malformed Rust to fail syntax check")
		}
		if !strings.Contains(badRsMsg, "Rust Syntax Error") {
			t.Fatalf("expected Rust Syntax Error, got: %s", badRsMsg)
		}
	}
}

func TestPreFlightSyntaxCheckFiles(t *testing.T) {
	tmpDir := t.TempDir()
	f1 := filepath.Join(tmpDir, "good.go")
	_ = os.WriteFile(f1, []byte("package main\n\nfunc A() {}\n"), 0644)

	f2 := filepath.Join(tmpDir, "bad.go")
	_ = os.WriteFile(f2, []byte("package main\n\nfunc B( {}\n"), 0644)

	ok1, _ := PreFlightSyntaxCheckFiles(tmpDir, []string{"good.go"})
	if !ok1 {
		t.Fatalf("expected good.go to pass")
	}

	okBoth, msg := PreFlightSyntaxCheckFiles(tmpDir, []string{"good.go", "bad.go"})
	if okBoth {
		t.Fatalf("expected bad.go in file list to fail")
	}
	if !strings.Contains(msg, "Go Syntax Error") {
		t.Fatalf("expected Go Syntax Error in msg, got: %s", msg)
	}
}

func TestParseTracebacks(t *testing.T) {
	// 1. Go test failure
	goOut := "=== RUN   TestSample\n--- FAIL: TestSample (0.00s)\n    sample_test.go:10: assert failed\nFAIL\n"
	goTraces := parseTracebacks(goOut, "")
	if len(goTraces) == 0 {
		t.Fatalf("expected Go trace extracted, got 0")
	}
	if !strings.Contains(goTraces[0], "--- FAIL: TestSample") {
		t.Errorf("expected Go trace content, got %q", goTraces[0])
	}

	// 2. Python pytest failure
	pyOut := "FAILED test_calc.py::test_add - assert 1 == 2\nE   assert 1 == 2\n"
	pyTraces := parseTracebacks(pyOut, "")
	if len(pyTraces) == 0 {
		t.Fatalf("expected Python trace extracted, got 0")
	}
	if !strings.Contains(pyTraces[0], "FAILED") {
		t.Errorf("expected Python trace content, got %q", pyTraces[0])
	}

	// 3. Rust cargo test panic
	rustOut := "test test_task ... FAILED\n\nfailures:\n---- test_task stdout ----\nthread 'test_task' panicked at 'assert failed', src/lib.rs:12:5\ntest result: FAILED. 0 passed; 1 failed\n"
	rustTraces := parseTracebacks(rustOut, "")
	if len(rustTraces) == 0 {
		t.Fatalf("expected Rust trace extracted, got 0")
	}
	if !strings.Contains(rustTraces[0], "FAILED") && !strings.Contains(rustTraces[0], "panicked at") {
		t.Errorf("expected Rust trace content, got %q", rustTraces[0])
	}

	// 4. Go runtime data race detector
	raceOut := "==================\nWARNING: DATA RACE\nWrite at 0x00c000100010 by goroutine 7:\n  main.mutate()\n==================\nFAIL\n"
	raceTraces := parseTracebacks(raceOut, "")
	if len(raceTraces) == 0 {
		t.Fatalf("expected Go data race trace extracted, got 0")
	}
	if !strings.Contains(raceTraces[0], "DATA RACE") {
		t.Errorf("expected DATA RACE content, got %q", raceTraces[0])
	}

	// 5. Node / Jest failure
	jestOut := "FAIL src/calc.test.ts\n  ● Calculator › adds\n    expect(received).toBe(expected)\n"
	jestTraces := parseTracebacks(jestOut, "")
	if len(jestTraces) == 0 {
		t.Fatalf("expected Jest trace extracted, got 0")
	}
	if !strings.Contains(jestTraces[0], "Calculator") {
		t.Errorf("expected Jest trace content, got %q", jestTraces[0])
	}

	// 6. Multiple failing Go tests extracted cleanly
	multiGo := "=== RUN   TestOne\n--- FAIL: TestOne (0.00s)\n    one_test.go:5: err 1\n=== RUN   TestTwo\n--- FAIL: TestTwo (0.00s)\n    two_test.go:8: err 2\nFAIL\n"
	multiTraces := parseTracebacks(multiGo, "")
	if len(multiTraces) != 2 {
		t.Fatalf("expected 2 distinct Go traces, got %d", len(multiTraces))
	}
	if !strings.Contains(multiTraces[0], "TestOne") || !strings.Contains(multiTraces[1], "TestTwo") {
		t.Errorf("expected separate TestOne and TestTwo, got %v", multiTraces)
	}
}

func TestCreateGitPRBranch_BypassesGlobalGPGSign(t *testing.T) {
	tmpDir := t.TempDir()

	cInit := exec.Command("git", "init")
	cInit.Dir = tmpDir
	if err := cInit.Run(); err != nil {
		t.Skip("git init unavailable")
	}

	// Force GPG signing in this repo with an invalid gpg binary to simulate
	// environments where commit.gpgsign is globally enabled without a gpg agent.
	c1 := exec.Command("git", "config", "user.name", "Tester")
	c1.Dir = tmpDir
	_ = c1.Run()
	c2 := exec.Command("git", "config", "user.email", "tester@test.com")
	c2.Dir = tmpDir
	_ = c2.Run()
	c3 := exec.Command("git", "config", "commit.gpgsign", "true")
	c3.Dir = tmpDir
	_ = c3.Run()
	c4 := exec.Command("git", "config", "gpg.program", "invalid-gpg-program-never-exists")
	c4.Dir = tmpDir
	_ = c4.Run()

	mgr := NewCheckpointManager(tmpDir)
	branchName := "fix/test-gpgsign-bypass"
	err := mgr.CreateGitPRBranchWithAudit(branchName, "fix(test): commit without gpg sign", "# Audit Report\n")
	if err != nil {
		t.Fatalf("CreateGitPRBranchWithAudit failed when commit.gpgsign=true: %v", err)
	}

	// Verify branch was created and head commit exists
	cHead := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cHead.Dir = tmpDir
	out, err := cHead.Output()
	if err != nil {
		t.Fatalf("failed reading HEAD: %v", err)
	}
	if strings.TrimSpace(string(out)) != branchName {
		t.Errorf("expected branch %s, got %s", branchName, strings.TrimSpace(string(out)))
	}
}
