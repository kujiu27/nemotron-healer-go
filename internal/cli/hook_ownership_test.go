package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newHookRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git", "hooks"), 0755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestInstallRefusesForeignPrePush(t *testing.T) {
	dir := newHookRepo(t)
	pre := filepath.Join(dir, ".git", "hooks", "pre-push")
	if err := os.WriteFile(pre, []byte("#!/bin/sh\n# husky-managed\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallGitHook(dir); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("foreign hook must be protected, got: %v", err)
	}
	got, _ := os.ReadFile(pre)
	if !strings.Contains(string(got), "husky-managed") {
		t.Fatal("foreign hook content was modified")
	}
}

func TestInstallOverwritesOwnHook(t *testing.T) {
	dir := newHookRepo(t)
	if _, err := InstallGitHook(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallGitHook(dir); err != nil {
		t.Fatalf("re-install over own hook must succeed: %v", err)
	}
}

func TestUninstallRemovesOnlyOwnHook(t *testing.T) {
	dir := newHookRepo(t)
	if _, err := InstallGitHook(dir); err != nil {
		t.Fatal(err)
	}
	if err := UninstallGitHook(dir); err != nil {
		t.Fatal(err)
	}
	pre := filepath.Join(dir, ".git", "hooks", "pre-push")
	if _, err := os.Stat(pre); !os.IsNotExist(err) {
		t.Fatal("own hook not removed")
	}
	if err := os.WriteFile(pre, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := UninstallGitHook(dir); err == nil || !strings.Contains(err.Error(), "refusing to remove") {
		t.Fatalf("foreign hook must be preserved, got: %v", err)
	}
}
