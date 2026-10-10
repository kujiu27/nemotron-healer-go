package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateSafePathNestedGitRejected(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []string{"nested/.git/hooks/pre-commit", "vendor/.git/config", ".GIT/x"} {
		if _, err := ValidateSafePath(dir, p); err == nil {
			t.Fatalf("nested .git path must be rejected: %s", p)
		}
	}
}

func TestValidateSafePathSymlinkEscapeRejected(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "assets")); err != nil {
		t.Skip("symlinks unavailable")
	}
	if _, err := ValidateSafePath(dir, "assets/cron.d/x"); err == nil || !strings.Contains(err.Error(), "symlink escapes") {
		t.Fatalf("symlink escape must be rejected, got: %v", err)
	}
	// A plain in-workspace path on a symlinked TempDir (macOS /var) must pass.
	if _, err := ValidateSafePath(dir, "pkg/file.go"); err != nil {
		t.Fatalf("legit path on symlinked tmp root rejected: %v", err)
	}
}

func TestPersistRegressionTraversalBlocked(t *testing.T) {
	// Regression: LLM target_file with ../ used to write outside the workspace.
	dir := t.TempDir()
	// Assert the underlying gate (the falsifier routes its write through it).
	if _, err := ValidateSafePath(dir, filepath.Join("../../pwn", "x_nemotron_regression_test.go")); err == nil {
		t.Fatal("traversal regression path must be rejected by the gate")
	}
}

func TestSanitizeEnvironmentDefaultDeny(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@prod")
	t.Setenv("PATH", "/usr/bin")
	env := sanitizeEnvironment()
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "postgres://") {
		t.Fatal("DATABASE_URL (DSN-style secret) leaked into subprocess env")
	}
	if !strings.Contains(joined, "PATH=") {
		t.Fatal("PATH must survive the allowlist")
	}
}

func TestSanitizeEnvironmentRetainsMemoryContainment(t *testing.T) {
	t.Setenv("GOMEMLIMIT", "256MiB")
	t.Setenv("GOCACHE", "/tmp/gocache")
	t.Setenv("NODE_ENV", "test")
	t.Setenv("SECRET_TOKEN", "super-secret-123")

	env := sanitizeEnvironment()
	joined := strings.Join(env, "\n")

	if !strings.Contains(joined, "GOMEMLIMIT=256MiB") {
		t.Errorf("GOMEMLIMIT must survive allowlist for OOM containment, got: %s", joined)
	}
	if !strings.Contains(joined, "GOCACHE=/tmp/gocache") {
		t.Errorf("GOCACHE must survive allowlist, got: %s", joined)
	}
	if !strings.Contains(joined, "NODE_ENV=test") {
		t.Errorf("NODE_ENV must survive allowlist, got: %s", joined)
	}
	if strings.Contains(joined, "SECRET_TOKEN") {
		t.Errorf("SECRET_TOKEN must be stripped by default-deny, got: %s", joined)
	}
}

func TestCheckpointDirPrivate(t *testing.T) {
	dir := t.TempDir()
	cm := NewCheckpointManager(dir)
	cpID, err := cm.CreateCheckpoint()
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(os.TempDir(), "nemotron_checkpoints", cpID))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0077 != 0 {
		t.Fatalf("checkpoint dir must be 0700, got %v", fi.Mode().Perm())
	}
}
